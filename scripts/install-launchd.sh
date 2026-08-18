#!/usr/bin/env bash
# Registers autoship as a macOS LaunchAgent.
#
# autoship is a scheduled one-shot, not a daemon: it runs, decides, possibly
# works, and exits. That is why it costs nothing between ticks, and why
# supervision, restart and history are launchd's job rather than ours.
#
# A LaunchAgent (not a LaunchDaemon) is used deliberately: it only runs while
# the user is logged in, which is what the Windows S4U interactive-token
# principal buys autoship there too — Gradle, the Android SDK cache, and the
# Keychain-backed secrets store (dpapi_darwin.go) all need that session.
#
# launchd has no native "every N minutes within a daily window" trigger, so
# this script expands the window into one StartCalendarInterval entry per
# tick — the same shape as the Windows Task Scheduler repetition, just made
# explicit instead of relying on a single "repeat every" setting.
#
# Usage:
#   scripts/install-launchd.sh --exe /path/to/autoship --config /path/to/autoship.yaml [options]
#
# Options:
#   --exe PATH             Path to the autoship binary (required)
#   --config PATH          Path to autoship.yaml (required)
#   --name NAME             Task name suffix. Default: autoship
#   --interval MINUTES      Minutes between ticks. Default: 15
#   --start HH:MM            First run of the day. Default: 09:00
#   --window HOURS           Length of the working-hours window. Default: 10
#   --dry-run                 Register the task to run `autoship dry-run` instead of `run`.
#   --print-only               Print the plist without installing it.

set -euo pipefail

EXE=""
CONFIG=""
NAME="autoship"
INTERVAL=15
START="09:00"
WINDOW=10
VERB="run"
PRINT_ONLY=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --exe) EXE="$2"; shift 2 ;;
    --config) CONFIG="$2"; shift 2 ;;
    --name) NAME="$2"; shift 2 ;;
    --interval) INTERVAL="$2"; shift 2 ;;
    --start) START="$2"; shift 2 ;;
    --window) WINDOW="$2"; shift 2 ;;
    --dry-run) VERB="dry-run"; shift ;;
    --print-only) PRINT_ONLY=1; shift ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "$EXE" || -z "$CONFIG" ]]; then
  echo "usage: $0 --exe PATH --config PATH [--name NAME] [--interval MIN] [--start HH:MM] [--window HOURS] [--dry-run] [--print-only]" >&2
  exit 2
fi
if [[ ! -x "$EXE" ]]; then
  echo "autoship binary not found or not executable at $EXE" >&2
  exit 1
fi
if [[ ! -f "$CONFIG" ]]; then
  echo "autoship.yaml not found at $CONFIG" >&2
  exit 1
fi

EXE_ABS="$(cd "$(dirname "$EXE")" && pwd)/$(basename "$EXE")"
CONFIG_ABS="$(cd "$(dirname "$CONFIG")" && pwd)/$(basename "$CONFIG")"
WORKDIR="$(dirname "$CONFIG_ABS")"
LABEL="dev.autoship.${NAME}"
PLIST_DIR="$HOME/Library/LaunchAgents"
PLIST_PATH="${PLIST_DIR}/${LABEL}.plist"
LOG_DIR="$HOME/Library/Logs/autoship"

start_min=$(( 10#${START%%:*} * 60 + 10#${START##*:} ))
window_min=$(( WINDOW * 60 ))
end_min=$(( start_min + window_min ))

intervals=""
t=$start_min
while [[ $t -lt $end_min ]]; do
  h=$(( t / 60 ))
  m=$(( t % 60 ))
  intervals+="        <dict>
            <key>Hour</key><integer>${h}</integer>
            <key>Minute</key><integer>${m}</integer>
        </dict>
"
  t=$(( t + INTERVAL ))
done

PLIST_CONTENT=$(cat <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${LABEL}</string>
    <key>ProgramArguments</key>
    <array>
        <string>${EXE_ABS}</string>
        <string>${VERB}</string>
        <string>--config</string>
        <string>${CONFIG_ABS}</string>
        <string>--quiet</string>
    </array>
    <key>WorkingDirectory</key>
    <string>${WORKDIR}</string>
    <key>StartCalendarInterval</key>
    <array>
${intervals}    </array>
    <key>RunAtLoad</key>
    <false/>
    <key>StandardOutPath</key>
    <string>${LOG_DIR}/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>${LOG_DIR}/stderr.log</string>
</dict>
</plist>
PLIST
)

echo "Label:            $LABEL"
echo "Command:          $EXE_ABS $VERB --config \"$CONFIG_ABS\" --quiet"
echo "Working dir:      $WORKDIR"
echo "Schedule:         every ${INTERVAL} min from ${START} for ${WINDOW}h, daily ($(( (end_min - start_min) / INTERVAL )) ticks/day)"
echo "Runs as:          the logged-in user (LaunchAgent, not LaunchDaemon)"
echo "launchd logs:     ${LOG_DIR}/{stdout,stderr}.log"

if [[ $PRINT_ONLY -eq 1 ]]; then
  echo ""
  echo "--print-only: nothing was installed. Plist would be:"
  echo "$PLIST_CONTENT"
  exit 0
fi

mkdir -p "$PLIST_DIR" "$LOG_DIR"
printf '%s\n' "$PLIST_CONTENT" > "$PLIST_PATH"

launchctl unload "$PLIST_PATH" >/dev/null 2>&1 || true
launchctl load -w "$PLIST_PATH"

echo ""
echo "Installed and loaded: $PLIST_PATH"
echo "Inspect it with:      launchctl list | grep $LABEL"
echo "Run it once now with: launchctl start $LABEL"
echo "Remove it with:       launchctl unload \"$PLIST_PATH\" && rm \"$PLIST_PATH\""
