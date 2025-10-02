package main

import (
	"bytes"
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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	checkUpdate      bool
	checkUpdateOnly  bool
	makeSampleConfig bool
	configPath       string
	logPath          string
	dryRun           bool
	listOnly         bool
	showStatus       bool
	maxLogSize       int64
	logRetention     int
	reloadInterval   time.Duration
	verboseLog       bool
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
	appVersion = "0.2.0" // see FyneApp.toml
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

// find child PIDs of a given parent PID (Unix only, uses pgrep)
func findChildPIDs(parentPID int) ([]int, error) {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var pids []int
	for _, line := range lines {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

func makeSampleConfigs() error {
	// Windows sample config
	winSample := `[
  {
    "name": "notepad",
    "path": "C:\\Windows\\System32\\notepad.exe",
    "params": "launchertest.txt",
    "launch_times": ["08:56", "08:58"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "Notepad sucks - why do this? NOTE: Double backslash to escape \\ in path. NOTE2: use parameter trick to make notepad prompt to open a new file, allowing for detection, where a simple launch would spawn and detach a new process that is hard to track"
  },
  {
    "name": "notepad++",
    "path": "c:\\Program Files\\Notepad++\\notepad++.exe",
    "params": "",
    "launch_times": ["09:45", "09:47"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "Notepad sucks, use Notepad++ NOTE: Double backslash to escape \\ in path"
  },
  {
    "name": "DB Browser SQLite",
    "path": "c:\\Program Files\\DB Browser for SQLite\\DB Browser for SQLite.exe",
    "params": "",
    "launch_times": ["10:00"],
    "duration_minutes": 2,
    "recurrence": "daily",
    "comment": "DB Browser for SQLite NOTE: Double backslash to escape \\ in path"
  },
  {
    "name": "chrome",
    "path": "c:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
    "params": "--new-window --incognito https://www.google.com/",
    "launch_times": ["08:44", "08:46"],
    "duration_minutes": 2,
    "recurrence": "daily",
    "comment": "NOT RECOMMENDED: Unreliable detection - Google Chrome in incognito mode, use --new-window to force a new window"
  }
]`
	winPath := "launcher_windows_sample.json"
	if err := os.WriteFile(winPath, []byte(winSample), 0644); err != nil {
		return fmt.Errorf("failed to write Windows sample config: %w", err)
	}
	log.Printf("Windows sample config written to %s", winPath)

	// non Windows sample config
	nonWinSample := `[
  {
    "name": "timer",
    "path": "~/bin/timer",
    "params": "",
    "launch_times": ["10:52", "10:54", "10:56"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "A simple timer app I wrote"
  },
  {
    "name": "Inkscape",
    "path": "/Applications/Inkscape.app/Contents/MacOS/inkscape",
    "params": "",
    "launch_times": ["10:53", "10:56"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "Vector graphics editor"
  },
  {
    "name": "DB Browser for SQLite",
    "path": "/Applications/DB Browser for SQLite.app/Contents/MacOS/DB Browser for SQLite",
    "params": "",
    "launch_times": ["10:53", "10:56"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "DB Browser - SQLite"
  },
  {
    "name": "HourlyApp",
    "path": "/usr/bin/echo",
    "params": "Hello from hourly app",
    "launch_times": ["00:00"],
    "duration_minutes": 1,
    "recurrence": "hourly",
    "comment": "An app that runs every hour"
  },
  {
    "name": "Google Chrome via shell script",
    "path": "./chrome_launch.sh",
    "params": "--incognito --new-window https://www.google.com/",
    "launch_times": ["10:31", "10:33"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "NOT RECOMMENDED: Unreliable detection - Google Chrome in incognito mode, use --new-window to force a new window"
  },
  {
    "name": "Google Chrome",
    "path": "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    "params": "--new-window --new-window --incognito https://www.google.com/",
    "launch_times": ["10:45", "10:47"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "NOT RECOMMENDED: Unreliable detection - Google Chrome in incognito mode, use --new-window to force a new window"
  },
  {
    "name": "Microsoft Edge",
    "path": "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
    "params": "https://www.google.com/",
    "launch_times": ["11:25", "11:27", "11:29"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "NOT RECOMMENDED: Unreliable detection - Microsoft Edge in use -new-window -inprivate to force a new window"
  }
]
`

	nonWinPath := "launcher_nonwindows_sample.json"
	if err := os.WriteFile(nonWinPath, []byte(nonWinSample), 0644); err != nil {
		return fmt.Errorf("failed to write non Windows sample config: %w", err)
	}
	log.Printf("Non Windows sample config written to %s", nonWinPath)
	return nil
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

// read PID from a file - MacOS Chrome weirdness workaround attempt, unused
func readPIDFromFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID format: %w", err)
	}
	return pid, nil
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
	uc := updatechecker.New(repoOwner, repo, repoName, repodl, 0, false)
	uc.CheckForUpdate(appVersion)
	updtmsg := uc.Message
	return updtmsg, uc.UpdateAvailable
}

// validate .json config file and exit out ASAP with error help if possible
func validateConfigFindErrorLocation(data []byte, offset int64) (line, col int) {
	line = 1
	col = 1
	for i := int64(0); i < offset && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return
}

func ValidateConfig(jsonData []byte) ([]AppConfig, error) {
	var configs []AppConfig

	// Run regex-based pre-checks
	if err := validateJsonSyntaxHints(jsonData); err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(jsonData))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&configs); err != nil {
		if syntaxErr, ok := err.(*json.SyntaxError); ok {
			line, col := validateConfigFindErrorLocation(jsonData, syntaxErr.Offset)
			return nil, fmt.Errorf("syntax error at line %d, column %d: %v", line, col, err)
		}
		return nil, fmt.Errorf("JSON decoding error: %v", err)
	}

	for i, cfg := range configs {
		if cfg.Name == "" {
			return nil, fmt.Errorf("entry %d: 'name' is required", i)
		}
		if cfg.Path == "" {
			return nil, fmt.Errorf("entry %d: 'path' is required", i)
		}
		if len(cfg.LaunchTimes) == 0 {
			return nil, fmt.Errorf("entry %d: 'launch_times' must have at least one time", i)
		}
		for _, t := range cfg.LaunchTimes {
			if _, err := time.Parse("15:04", t); err != nil {
				return nil, fmt.Errorf("entry %d: invalid time format '%s' (expected HH:MM)", i, t)
			}
		}
		if cfg.DurationMinutes <= 0 {
			return nil, fmt.Errorf("entry %d: 'duration_minutes' must be positive", i)
		}
		if cfg.Recurrence != "daily" && cfg.Recurrence != "hourly" {
			return nil, fmt.Errorf("entry %d: 'recurrence' must be 'daily' or 'hourly'", i)
		}
	}

	return configs, nil
}

func validateJsonSyntaxHints(data []byte) error {
	text := string(data)

	// Trailing comma before closing object or array
	trailingCommaObj := regexp.MustCompile(`,\s*}`)
	trailingCommaArr := regexp.MustCompile(`,\s*]`)
	if trailingCommaObj.MatchString(text) {
		return fmt.Errorf("possible trailing comma before closing '}'")
	}
	if trailingCommaArr.MatchString(text) {
		return fmt.Errorf("possible trailing comma before closing ']'")
	}

	// Missing closing bracket or brace (very basic heuristic)
	openBraces := bytes.Count(data, []byte("{"))
	closeBraces := bytes.Count(data, []byte("}"))
	openBrackets := bytes.Count(data, []byte("["))
	closeBrackets := bytes.Count(data, []byte("]"))

	if openBraces != closeBraces {
		return fmt.Errorf("mismatched number of '{' and '}'")
	}
	if openBrackets != closeBrackets {
		return fmt.Errorf("mismatched number of '[' and ']'")

	}

	return nil
}

// ====== Scheduling ======
func scheduleApp(app AppConfig, dryRun bool) {
	// first clear any existing timers for this app
	runningMu.Lock()
	for _, t := range scheduledTimers[app.Name] {
		t.Stop()
	}
	scheduledTimers[app.Name] = nil
	runningMu.Unlock()

	// Schedule each launch time
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
			if strings.Contains(strings.ToLower(app.Name), "edge") || strings.Contains(strings.ToLower(app.Name), "chrome") {
				log.Printf("Note: Google Chrome and Microsoft Edge may fork and detach on macOS. Tracking may be (IS) unreliable.")

				// Suppress verbose updater messages from Chrome and Edge
				cmd.Stdout = nil
				cmd.Stderr = nil

			}

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

			/*
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
			*/

			runningMu.Lock()
			cur, ok := runningApps[app.Name]
			runningMu.Unlock()

			if ok && cur.PID == cmd.Process.Pid {
				if err := killProcessTree(cmd.Process.Pid); err != nil {
					// Fallback to direct kill if tree-kill fails
					if err2 := cmd.Process.Kill(); err2 != nil {
						log.Printf("Failed to terminate %s (PID %d): %v (fallback: %v)",
							app.Name, cmd.Process.Pid, err, err2)
					} else {
						log.Printf("%s terminated after %d minutes (fallback kill)",
							app.Name, app.DurationMinutes)
					}
				} else {
					log.Printf("%s terminated after %d minutes (tree kill)",
						app.Name, app.DurationMinutes)
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
		if err := killProcessTree(cmd.Process.Pid); err != nil {
			// Fallback: best-effort single process kill
			_ = cmd.Process.Kill()
		}
	}
	runningProcs = make(map[string]*exec.Cmd)
	runningApps = make(map[string]RunningApp)
	runningMu.Unlock()

	/*
		runningMu.Lock()
		for _, cmd := range runningProcs {
			_ = cmd.Process.Kill()
		}
		// Reset maps (safe; waiter will no-op if entries are gone)
		runningProcs = make(map[string]*exec.Cmd)
		runningApps = make(map[string]RunningApp)
		runningMu.Unlock()
	*/

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
	// http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
		fmt.Fprintf(w, `<li>Port: %d, Refresh interval: %d</li>`, port, refreshInterval)
		if verboseLog {
			fmt.Fprintf(w, `<li>Current time: %s</li>`, time.Now().Format(time.RFC1123))
		}

		fmt.Fprintf(w, `<br><br><hr>`)
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
			fmt.Fprintf(w, `<br>&emsp;Full config: Name=%s, Path=%s, Params=%s, LaunchTimes=%v, DurationMinutes=%d, Recurrence=%s, Comment=%s</br>`,
				app.Name, app.Path, app.Params, app.LaunchTimes, app.DurationMinutes, app.Recurrence, app.Comment)
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

	flag.BoolVar(&checkUpdate, "checkupdate", false, "Check for application updates")
	flag.BoolVar(&checkUpdateOnly, "checkupdateonly", false, "Check for application updates and exit")
	flag.BoolVar(&makeSampleConfig, "makeconfig", false, "Make Windows and non Windows sample configurations and exit")
	flag.BoolVar(&makeSampleConfig, "makesample", false, "Make Windows and non Windows sample configurations and exit")
	flag.StringVar(&configPath, "config", "launcher.json", "Path to configuration file")
	flag.StringVar(&logPath, "log", "launcher.log", "Path to log file")
	flag.BoolVar(&dryRun, "dry-run", false, "Simulate launches without executing")
	flag.BoolVar(&dryRun, "sim", false, "sim alias for --dry-run")
	flag.BoolVar(&dryRun, "simulate", false, "simulate alias for --dry-run")
	flag.BoolVar(&listOnly, "list", false, "List scheduled applications and exit")
	flag.BoolVar(&showStatus, "status", false, "Start an http listener on http://localhost:80 (or specified port) to show currently running applications")
	flag.Int64Var(&maxLogSize, "max-log-size", 1024*1024, "Maximum log file size in bytes before rotation (1Mb)")
	flag.IntVar(&logRetention, "log-retention", 3, "Number of rotated logs to retain")
	flag.DurationVar(&reloadInterval, "reload-interval", 60*time.Second, "Interval to check for config changes (best never less than 60s)")
	flag.IntVar(&httpPort, "http-port", 80, "Port for HTTP status server")
	flag.IntVar(&httpRefreshInterval, "http-refresh", 5, "Refresh interval in seconds for HTTP status page")
	flag.BoolVar(&verboseLog, "verbose", false, "Verbose / debug logging")
	flag.BoolVar(&verboseLog, "debug", false, "Verbose / debug logging")
	flag.Parse()

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

	// check update first
	if checkUpdate || checkUpdateOnly {
		// Check for updates and exit
		fmt.Println("Checking for updates...")
		log.Println("Checking for updates...")
		updtmsg, updateAvail := updateChecker("amarillier", "KrankyBearLauncher", "Kranky Bear Launcher", "https://github.com/amarillier/KrankyBearClock/releases/latest")
		fmt.Println(updtmsg)
		log.Println(updtmsg)
		if updateAvail {
			fmt.Println("Update available:", updtmsg)
			fmt.Println("Please visit the GitHub releases page to download the latest version.")
			log.Println("Update available:", updtmsg)
			log.Println("Please visit the GitHub releases page to download the latest version.")
		}
		if checkUpdateOnly {
			os.Exit(0)
		}
	}

	// validate json config file first
	jsonData, err := os.ReadFile(expandPath(configPath))
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	configs, err := ValidateConfig(jsonData)
	if err != nil {
		log.Fatalf("Config validation failed: %v", err)
	}
	log.Println("Config is valid. Loaded", len(configs), "apps.")

	// check for request to make sample config files
	if makeSampleConfig {
		if err := makeSampleConfigs(); err != nil {
			fmt.Println("Failed to create sample configs:", err)
			log.Println("Failed to create sample configs:", err)
			os.Exit(1)
		}
		fmt.Println("Sample configuration files created.")
		log.Println("Sample configuration files created.")
		os.Exit(0)
	}

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
	/*go func() {
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
	*/

	// Monitor running processes for external termination
	go func() {
		for {
			time.Sleep(10 * time.Second)
			runningMu.Lock()
			for name, app := range runningApps {
				alive, err := isProcessRunning(app.PID)
				if err != nil {
					log.Printf("Liveness check error for %s (PID %d): %v", name, app.PID, err)
					continue
				}
				if !alive {
					log.Printf("Detected exit of %s (PID %d) via liveness check", name, app.PID)
					delete(runningApps, name)
					delete(runningProcs, name)
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
