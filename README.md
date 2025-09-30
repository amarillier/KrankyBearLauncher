
# Kranky Bear Launcher ![image](https://github.com/user-attachments/assets/eb234c46-98bb-4da0-b418-431d0afecbb5)



## Features
* Scheduler / application launcher for testing application execution and termination at user specified times
* .json config file specifies applications, parameters, one or multiple start times, duration, recurrence
*   recurrence allows hourly, daily, weekly, or null meaning no recurrence
*   switches allow for multiple config files, default is launcher.json
* json syntax verification at start time only verifies the json config is valid
    testing for additional comma, missing comma, missing { or }, missing [ or ]
    NOTE: After launch, changes made to a config file will be detected by syntax
    checks are not performed
* dry run / simulation capability simulates execution without actually executing
* ~/ and $HOME/ syntax for Linux / MacOS are recognized and expanded, explicit paths to applications are supported
* logging to specified log file, default is launcher.log
    configurable log rotation at default 1Mb with log retention at default 3 log files
* optional http listener - status shows both currently launched and scheduled applications
    http listen port (default 8080) and auto browser refresh interval (default 5 seconds) are
    customizable via switches
* automated configuration change detection reloads launcher*.json changes.
    NOTE: reloading configurations due to any changes detected will terminate any
    currently launched applications so the new schedule applies immediately
* ability to force configuration reload with a SIGUSR1 signal for Mac, Linux only
* signal detection - SIGINT and SIGTERM for Mac, Windows, Linux terminates all running
    apps and the launcher when the launcher exits
* 

### See below for usage and syntax



## To-do / known problems
- None currently known
- See ReleaseNotes.txt for all recent changes and future plans



## License
See license.txt
This is 100% free for anyone to use or misuse any way you like with no warranty as
to suitability or anything else, other than it has no viruses when I compile and
commit to git. But you should always check and scan anything you download from the
internet for viruses anyway. Don't be reckless.

All KrankyBear icons, images, logos used are copyright (c) Allan Marillier, 2024, 2025 ...

## Usage / syntax
./KrankyBearLauncher -help | -?

All switches accept single or double - or -- syntax
Usage of ./KrankyBearLauncher:
  -checkupdate
        Check for application updates
  -checkupdateonly
        Check for application updates and exit
  -config string
        Path to configuration file (default "launcher.json")
  -debug
        Verbose / debug logging
  -dry-run
        Simulate launches without executing
  -http-port int
        Port for HTTP status server (default 80)
  -http-refresh int
        Refresh interval in seconds for HTTP status page (default 5)
  -list
        List scheduled applications and exit
  -log string
        Path to log file (default "launcher.log")
  -log-retention int
        Number of rotated logs to retain (default 3)
  -makeconfig
        Make Windows and non Windows sample configurations and exit
  -makesample
        Make Windows and non Windows sample configurations and exit
  -max-log-size int
        Maximum log file size in bytes before rotation (1Mb) (default 1048576)
  -reload-interval duration
        Interval to check for config changes (best never less than 60s) (default 1m0s)
  -sim
        sim alias for --dry-run
  -simulate
        simulate alias for --dry-run
  -status
        Start an http listener on http://localhost:80 (or specified port) to show currently running applications
  -verbose
        Verbose / debug logging


## Example use
### Run with default reload interval and log rotation
./KrankyBearLauncher -help

./KrankyBearLauncher --log=launcher.log --max-log-size=524288 --log-retention=3

### Run with 128Kb log size, retain 2 logs, reload every 5 minutes, using launcher-allan.json
./KrankyBearLauncher --log=launcher.log --max-log-size=131072 --log-retention=2 -reload-interval 300 --config launcher-allan.json

### Force reload manually
kill -SIGUSR1 <pid_of_launcher>

### Run with default reload interval
./KrankyBearLauncher --config=launcher.json --log=launcher.log

### Run with custom reload interval
./KrankyBearLauncher --reload-interval=30s

### Just list scheduled apps
./KrankyBearLauncher --list -launcher-allan.json

### Normal run with reload interval
./KrankyBearLauncher --config=launcher-allan.json --reload-interval=60s

### Run with 128Kb log size, retain 2 logs, reload every 5 minutes, using launcher-allan.json, web listener on custom port 8085 with browser http refresh interval eavery 10 seconds
./KrankyBearLauncher --log=launcher.log --max-log-size=131072 --log-retention=2 -reload-interval 300 --config launcher-allan.json --status --http -port 8085 -http-refresh 10


## Example configuration file config.json:
Note examples below show both Windows and non Windows paths as well as
absolute paths, as well as non Windows home directory expansion of 
~/, $HOME/ etc. as well as both single and multiple launch times per
application. This would generally not be used with both Windows and non
Windows path styles

NOTE: Google Chrome and Microsoft Edge both behave weirdly in many cases
since both are based on Chromium. They launch a process, which launches
another process and detaches. This is very difficult and unpredictable to
track (reliably). It is best to avoid launching either of them because
they may not show properly in application tracking, and may not be 
properly terminated when their run time expires. Use other applications.

[
  {
    "name": "notepad",
    "path": "C:\\Windows\\System32\\notepad.exe",
    "params": "",
    "launch_times": ["10:00"],
    "duration_minutes": 10,
    "recurrence": "daily",
    "comment": "Notepad sucks - why do this? NOTE: Double backslash to escape \ in path"
  },
    {
    "name": "DB Browser SQLite",
    "path": "c:\\Program Files\\DB Browser for SQLite\\DB Browser for SQLite.exe",
    "params": "",
    "launch_times": ["10:00"],
    "duration_minutes": 10,
    "recurrence": "daily",
    "comment": "DB Browser for SQLite NOTE: Double backslash to escape \ in path"
  },
  {
    "name": "timer",
    "path": "~/bin/timer",
    "params": "",
    "launch_times": ["09:25", "09:27", "09:29"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "comment here"
  },
  {
    "name": "clock",
    "path": "$HOME/bin/clock",
    "params": "",
    "launch_times": ["09:25"],
    "duration_minutes": 1,
    "recurrence": "weekly",
    "comment": "comment here"
  },
  {
    "name": "chrome",
    "path": "c:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
    "params": "--incognito" "https://www.tanium.com/",
    "launch_times": ["10:30", "11:00"],
    "duration_minutes": 15,
    "recurrence": "daily",
    "comment": "comment here"
  },
  {
    "name": "Inkscape",
    "path": "/Applications/Inkscape.app/Contents/MacOS/inkscape",
    "params": "",
    "launch_times": ["09:26", "09:27"],
    "duration_minutes": 1,
    "recurrence": "daily",
    "comment": "comment here"
  },
  {
    "name": "HourlyApp",
    "path": "/usr/bin/echo",
    "params": "Hello from hourly app",
    "launch_times": ["00:00"],
    "duration_minutes": 1,
    "recurrence": "hourly",
    "comment": "comment here"
  }
]


# "Now this is not the end. It is not even the beginning of the end. But it is, perhaps, the end of the beginning." Winston Churchill, November 10, 1942