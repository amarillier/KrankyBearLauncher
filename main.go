package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	updatechecker "github.com/amarillier/go-update-checker"
	"github.com/fsnotify/fsnotify"
)

// ====== Types ======
type AppConfig struct {
	Name            string   `json:"name"`
	Path            string   `json:"path"`
	Params          string   `json:"params"`
	LaunchTimes     []string `json:"launch_times"` // e.g., ["14:30", "16:05"]
	DurationMinutes int      `json:"duration_minutes"`
	Recurrence      string   `json:"recurrence"`        // "", "hourly", "daily", "weekly"
	Comment         string   `json:"comment,omitempty"` // optional comment field
}

type RunningApp struct {
	Config    AppConfig
	Cmd       *exec.Cmd
	StartTime time.Time
	PID       int
}

// ====== Globals ======
var (
	checkUpdate    bool
	configPath     string
	logPath        string
	dryRun         bool
	listOnly       bool
	showStatus     bool
	maxLogSize     int64
	logRetention   int
	reloadInterval time.Duration
	verboseLog     bool
	// Maps of currently running processes and their commands
	// Use ONE mutex to protect BOTH maps to avoid race conditions.
	runningProcs = make(map[string]*exec.Cmd)
	runningApps  = make(map[string]RunningApp)
	runningMu    sync.Mutex
	// Map of scheduled timers for rescheduling on config reload
	scheduledTimers = make(map[string][]*time.Timer)

	lastConfigHash string
)

const (
	appName    = "Kranky Bear Launcher"
	appVersion = "0.1.0" // see FyneApp.toml
	appAuthor  = "Allan Marillier"
)

var appCopyright = "Copyright (c) Allan Marillier, 2025-" + strconv.Itoa(time.Now().Year())

// ====== Utilities ======
// remove old rotated logs beyond retention count
func cleanupOldLogs() {
	if logRetention <= 0 {
		return
	}
	base := filepath.Base(logPath)
	dir := filepath.Dir(logPath)
	files, _ := filepath.Glob(filepath.Join(dir, base+".*"))
	sort.Strings(files)
	if len(files) > logRetention {
		for _, f := range files[:len(files)-logRetention] {
			_ = os.Remove(f)
		}
	}
}

// expand path to absolute, expanding ~ and $HOME
func expandPath(p string) string {
	// Expand ~/, ~\  (all OS)
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	// Expand $HOME (Unix) – os.ExpandEnv below also handles $VAR
	if strings.Contains(p, "$HOME") {
		if home, err := os.UserHomeDir(); err == nil {
			p = strings.ReplaceAll(p, "$HOME", home)
		}
	}
	// Leave %VAR%/$VAR to os.ExpandEnv at call sites where needed.
	return p
}

// list scheduled applications and their details
func listApps(apps []AppConfig) {
	fmt.Println("Scheduled Applications:")
	for _, app := range apps {
		fmt.Printf("- %s at %v (%s, %d min, recurrence: %s)\n",
			app.Name, app.LaunchTimes, app.Path, app.DurationMinutes, app.Recurrence)
	}
}

// load and parse config file, returning hash of content for change detection
func loadConfig() ([]AppConfig, string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	hash := fmt.Sprintf("%x", sum[:])

	var apps []AppConfig
	if err := json.Unmarshal(data, &apps); err != nil {
		log.Printf("Error parsing JSON config: %v", err)
		log.Printf("Raw config content:\n%s", string(data))
		return nil, "", err
	}
	return apps, hash, nil
}

// convert app.Params (string) into []string, respecting "quoted args".
// It also expands environment variables ($VAR and %VAR% on Windows).
func paramsFor(app AppConfig) []string {
	if strings.TrimSpace(app.Params) == "" {
		return nil
	}
	return splitArgsRespectingQuotes(os.ExpandEnv(app.Params))
}

// rotate log file if it exceeds max size
func rotateLog() {
	if logPath == "" || maxLogSize <= 0 {
		return
	}
	fi, err := os.Stat(logPath)
	if err == nil && fi.Size() >= maxLogSize {
		timestamp := time.Now().Format("20060102_150405")
		rotated := fmt.Sprintf("%s.%s", logPath, timestamp)
		_ = os.Rename(logPath, rotated)
		cleanupOldLogs()
	}
}

// Very small quote-aware splitter (handles spaces and double quotes).
func splitArgsRespectingQuotes(s string) []string {
	var args []string
	var cur strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				cur.WriteByte(ch)
			} else if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// check github for application updates
func updateChecker(repoOwner string, repo string, repoName string, repodl string) (string, bool) {
	// uc := updatechecker.New("amarillier", "KrankyBearTimer", "Kranky Bear Timer", "", 1, false)
	uc := updatechecker.New(repoOwner, repo, repoName, repodl, 0, false)
	uc.CheckForUpdate(appVersion)
	// uc.PrintMessage()
	updtmsg := uc.Message
	return updtmsg, uc.UpdateAvailable
}

// ====== Scheduling ======
func scheduleApp(app AppConfig, dryRun bool) {
	for _, launchTimeStr := range app.LaunchTimes {
		layout := "15:04"
		now := time.Now()

		lt, err := time.Parse(layout, launchTimeStr)
		if err != nil {
			log.Printf("Invalid time format for %s: %v", app.Name, err)
			continue
		}

		launch := time.Date(now.Year(), now.Month(), now.Day(), lt.Hour(), lt.Minute(), 0, 0, now.Location())
		if launch.Before(now) {
			launch = launch.Add(24 * time.Hour)
		}
		delay := time.Until(launch)

		log.Printf("Scheduled %s to launch at %s", app.Name, launch.Format(time.RFC1123))

		timer := time.AfterFunc(delay, func() {
			if dryRun {
				log.Printf("[Dry Run] Would launch %s at %s", app.Name, launch.Format(time.RFC1123))
				return
			}

			log.Printf("Launching %s", app.Name)
			args := paramsFor(app)
			cmd := buildCmd(app, args) // resolved by cmd_windows.go or cmd_unix.go

			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			// A bit more logging, helpful for Windows debugging
			if verboseLog {
				if cmd.Dir != "" {
					log.Printf("Command args: %q | cwd: %q | exe: %q", args, cmd.Dir, cmd.Path)
				} else {
					log.Printf("Command args: %q | exe: %q", args, cmd.Path)
				}
			}

			if err := cmd.Start(); err != nil {
				log.Printf("Failed to launch %s: %v", app.Name, err)
				return
			}

			runningMu.Lock()
			runningProcs[app.Name] = cmd
			runningApps[app.Name] = RunningApp{
				Config:    app,
				Cmd:       cmd,
				StartTime: time.Now(),
				PID:       cmd.Process.Pid,
			}
			runningMu.Unlock()

			log.Printf("%s started with PID %d", app.Name, cmd.Process.Pid)

			go func(appName string, cmd *exec.Cmd, pid int) {
				err := cmd.Wait()
				runningMu.Lock()
				defer runningMu.Unlock()
				cur, ok := runningApps[appName]
				if ok && cur.PID == pid {
					delete(runningProcs, appName)
					delete(runningApps, appName)
					if err != nil {
						log.Printf("%s (PID %d) exited with error: %v", appName, pid, err)
					} else {
						log.Printf("%s (PID %d) exited normally", appName, pid)
					}
				}
			}(app.Name, cmd, cmd.Process.Pid)

			time.Sleep(time.Duration(app.DurationMinutes) * time.Minute)

			runningMu.Lock()
			cur, ok := runningApps[app.Name]
			runningMu.Unlock()
			if ok && cur.PID == cmd.Process.Pid {
				if err := cmd.Process.Kill(); err != nil {
					log.Printf("Failed to terminate %s: %v", app.Name, err)
				} else {
					log.Printf("%s terminated after %d minutes", app.Name, app.DurationMinutes)
				}
			}

			switch strings.ToLower(strings.TrimSpace(app.Recurrence)) {
			case "daily":
				scheduleApp(app, dryRun)
			case "weekly":
				time.AfterFunc(7*24*time.Hour, func() { scheduleApp(app, dryRun) })
			case "hourly":
				time.AfterFunc(1*time.Hour, func() { scheduleApp(app, dryRun) })
			}
		})

		runningMu.Lock()
		scheduledTimers[app.Name] = append(scheduledTimers[app.Name], timer)
		runningMu.Unlock()
	}
}

// ====== Reload / Reschedule ======
func reloadAndReschedule() {
	apps, hash, err := loadConfig()
	if err != nil {
		log.Printf("Failed to reload config: %v", err)
		return
	}
	if hash == lastConfigHash {
		return
	}
	lastConfigHash = hash
	log.Printf("Reloaded configuration")

	// Cancel all scheduled timers
	runningMu.Lock()
	for _, timers := range scheduledTimers {
		for _, t := range timers {
			t.Stop()
		}
	}
	scheduledTimers = make(map[string][]*time.Timer)
	runningMu.Unlock()

	// Kill all currently running processes. Waiters will reap/cleanup.
	runningMu.Lock()
	for _, cmd := range runningProcs {
		_ = cmd.Process.Kill()
	}
	// Reset maps (safe; waiter will no-op if entries are gone)
	runningProcs = make(map[string]*exec.Cmd)
	runningApps = make(map[string]RunningApp)
	runningMu.Unlock()

	// Reschedule all apps
	for _, app := range apps {
		if len(app.LaunchTimes) > 0 {
			log.Printf("Scheduling %s using multiple launch times", app.Name)
		}
		scheduleApp(app, dryRun)
	}
}

// ====== HTTP Status Server ======
func startHTTPStatusServer(port int, refreshInterval int) {
	// Simple HTTP server to show running and scheduled apps
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")

		// Snapshot running apps without holding the lock during I/O
		runningMu.Lock()
		snapshot := make([]RunningApp, 0, len(runningApps))
		for _, ra := range runningApps {
			snapshot = append(snapshot, ra)
		}
		runningMu.Unlock()

		fmt.Fprintf(w, `<html><head><meta http-equiv="refresh" content="%d"><title>Launcher Running Applications</title></head><body>`, refreshInterval)
		fmt.Fprintln(w, `<h2>Launcher: Running Applications</h2><ul>`)

		if len(snapshot) == 0 {
			fmt.Fprintln(w, `<li>No applications are currently running.</li>`)
		} else {
			for _, app := range snapshot {
				cmdline := strings.TrimSpace(app.Config.Path + " " + app.Config.Params)
				fmt.Fprintf(w, `<li><strong>%s</strong> - (PID: %d) - Command: %s - Started at %s</li>`,
					app.Config.Name, app.PID, cmdline, app.StartTime.Format("15:04:05"))
				fmt.Fprintf(w, `&emsp;Full config: Name=%s, Path=%s, Params=%s, LaunchTimes=%v, DurationMinutes=%d, Recurrence=%s</br>`,
					app.Config.Name, app.Config.Path, app.Config.Params, app.Config.LaunchTimes, app.Config.DurationMinutes, app.Config.Recurrence)
			}
		}

		fmt.Fprintln(w, `<br><hr><br><h2>Launcher: Scheduled Applications</h2><ul>`)

		apps, _, err := loadConfig()
		if err != nil {
			fmt.Fprintf(w, `<br>Error loading config: %v</br>`, err)
			fmt.Fprintln(w, `</body></html>`)
			return
		}
		for _, app := range apps {
			fmt.Fprintf(w, `<li><strong>%s</strong></li>`, app.Name)
			fmt.Fprintf(w, `&emsp;- Path: %s</li>`, app.Path)
			if app.Params != "" {
				fmt.Fprintf(w, `&emsp;- Params: %s</li>`, app.Params)
			}
			fmt.Fprintf(w, `&emsp;- Launch Times: %s</li>`, strings.Join(app.LaunchTimes, ", "))
			fmt.Fprintf(w, `&emsp;- Duration: %d minutes</li>`, app.DurationMinutes)
			if app.Recurrence != "" {
				fmt.Fprintf(w, `&emsp;- Recurrence: %s</li>`, app.Recurrence)
			}
			fmt.Fprintf(w, `<br>&emsp;Full config: Name=%s, Path=%s, Params=%s, LaunchTimes=%v, DurationMinutes=%d, Recurrence=%s</br>`,
				app.Name, app.Path, app.Params, app.LaunchTimes, app.DurationMinutes, app.Recurrence)
		}
		fmt.Fprintln(w, `</ul></body></html>`)
	})

	go func() {
		_ = http.ListenAndServe(fmt.Sprintf(":%d", port), nil)

	}()
}

// ====== Main ======
func main() {
	var httpPort int
	var httpRefreshInterval int

	flag.BoolVar(&checkUpdate, "checkupdate", true, "Check for application updates")
	flag.StringVar(&configPath, "config", "launcher.json", "Path to configuration file")
	flag.StringVar(&logPath, "log", "launcher.log", "Path to log file")
	flag.BoolVar(&dryRun, "dry-run", false, "Simulate launches without executing")
	flag.BoolVar(&dryRun, "sim", false, "sim alias for --dry-run")
	flag.BoolVar(&dryRun, "simulate", false, "simulate alias for --dry-run")
	flag.BoolVar(&listOnly, "list", false, "List scheduled applications and exit")
	flag.BoolVar(&showStatus, "status", false, "Start an http listener on http://localhost:8080 (or specified port) to show currently running applications")
	flag.Int64Var(&maxLogSize, "max-log-size", 1024*1024, "Maximum log file size in bytes before rotation (1Mb)")
	flag.IntVar(&logRetention, "log-retention", 3, "Number of rotated logs to retain")
	flag.DurationVar(&reloadInterval, "reload-interval", 60*time.Second, "Interval to check for config changes (best never less than 60s)")
	flag.IntVar(&httpPort, "http-port", 8080, "Port for HTTP status server")
	flag.IntVar(&httpRefreshInterval, "http-refresh", 5, "Refresh interval in seconds for HTTP status page")
	flag.BoolVar(&verboseLog, "verbose", false, "Verbose / debug logging")
	flag.Parse()

	// check update first
	if checkUpdate {
		updtmsg, available := updateChecker("amarillier", "KrankyBearLauncher", "Kranky Bear Launcher", "https://github.com/amarillier/KrankyBearClock/releases/latest")
		if updtmsg == "" {
			// open a window to show the update message
			// no need to test for updt window open at first start
			fmt.Println(updtmsg, available)
		}
		return
	}

	if listOnly {
		apps, _, err := loadConfig()
		if err != nil {
			fmt.Println("Failed to load config:", err)
			return
		}
		listApps(apps)
		return
	}

	rotateLog()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Failed to open log file:", err)
		return
	}
	defer logFile.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	if showStatus {
		startHTTPStatusServer(httpPort, httpRefreshInterval)
		log.Printf("HTTP status server running at http://localhost:%d/status", httpPort)
		log.Println("HTTP refresh interval:", httpRefreshInterval, "seconds")
	}

	apps, hash, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	lastConfigHash = hash

	// Setup file watcher for config changes
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	for _, app := range apps {
		scheduleApp(app, dryRun)
	}

	if err := watcher.Add(configPath); err != nil {
		log.Fatal(err)
	}

	// FSNotify reloader
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Println("Config file modified, reloading...")
					reloadAndReschedule()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("fsnotify error:", err)
			}
		}
	}()
	log.Println("Watching config file. Modify", configPath, "to trigger reload.")

	// Signals & shutdown handling
	setupSignalHandlers() // implemented per OS as separate signals_*.go

	// Monitor running processes for external termination
	go func() {
		for {
			time.Sleep(10 * time.Second) // check every 10 seconds
			runningMu.Lock()
			for name, app := range runningApps {
				if app.Cmd.ProcessState != nil && app.Cmd.ProcessState.Exited() {
					log.Printf("Detected external termination of %s (PID %d)", name, app.PID)
					delete(runningApps, name)
					delete(runningProcs, name)
				} else {
					// Try sending signal 0 to check if process is alive
					err := app.Cmd.Process.Signal(syscall.Signal(0))
					if err != nil {
						log.Printf("Process %s (PID %d) is no longer running: %v", name, app.PID, err)
						delete(runningApps, name)
						delete(runningProcs, name)
					}
				}
			}
			runningMu.Unlock()
		}
	}()

	// Periodic config reload ticker in its own goroutine
	ticker := time.NewTicker(reloadInterval)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			reloadAndReschedule()
		}
	}()

	// Block forever; shutdown occurs via signal goroutine's os.Exit
	select {}
}

// "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942
