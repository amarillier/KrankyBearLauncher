#!/bin/bash

# a simple wrapper script to launch Chrome in ncognito, new-window mode
# with all passed parameters - because Chrome does weird stuff and 
# does not launch properly from go exec.Command without losing track of PIDs
# NOTE: even this does not always work properly, sometimes Chrome just
# deta

#/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome "--incognito --new-window $*" &
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome "--incognito --new-window" &
CHROME_PID=$!
echo "chrome pid: $CHROME_PID"
sleep 2  # Give Chrome time to fork
pgrep -f $CHROME_PID
REAL_PID=$(pgrep -f "Google Chrome.*--incognito" | head -n 1)
echo $REAL_PID > /tmp/chrome_launcher.pid
echo "real pid: $REAL_PID"