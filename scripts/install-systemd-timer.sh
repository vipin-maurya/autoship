#!/usr/bin/env bash
# Registers autoship as a systemd --user timer on Linux.
#
# autoship is a scheduled one-shot, not a daemon: it runs, decides, possibly
# works, and exits. That is why it costs nothing between ticks, and why
# supervision, restart and history are systemd's job rather than ours.
#
# This installs a *user* unit, not a system one, deliberately: it only runs
# in a session for the invoking user, which is what the Windows S4U
# interactive-token principal buys autoship there too — Gradle, the Android
# SDK cache, and the Secret Service-backed secrets store (dpapi_linux.go, via
# secret-tool) all need that user session and its D-Bus bus to be available.
# On a headless box, enable lingering so the user unit runs without an active
# login (`loginctl enable-linger $USER`) — see the printed notes.
#
# Like the Windows Task Scheduler trigger, this expands the window into one
# OnCalendar= line per tick rather than relying on a single repeating rule,
# so the exact tick times are visible in the unit file.
#
# Usage:
#   scripts/install-systemd-timer.sh --exe /path/to/autoship --config /path/to/autoship.yaml [options]
#
# Options:
#   --exe PATH             Path to the autoship binary (required)
#   --config PATH          Path to autoship.yaml (required)
#   --name NAME             Unit name. Default: autoship
#   --interval MINUTES      Minutes between ticks. Default: 15
#   --start HH:MM            First run of the day. Default: 09:00
#   --window HOURS           Length of the working-hours window. Default: 10
#   --dry-run                 Register the unit to run `autoship dry-run` instead of `run`.
#   --print-only               Print the unit files without installing them.

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
UNIT_DIR="$HOME/.config/systemd/user"
SERVICE_PATH="${UNIT_DIR}/${NAME}.service"
TIMER_PATH="${UNIT_DIR}/${NAME}.timer"

start_min=$(( 10#${START%%:*} * 60 + 10#${START##*:} ))
window_min=$(( WINDOW * 60 ))
end_min=$(( start_min + window_min ))

calendars=""
t=$start_min
while [[ $t -lt $end_min ]]; do
  h=$(( t / 60 ))
  m=$(( t % 60 ))
  printf -v hh "%02d" "$h"
  printf -v mm "%02d" "$m"
  calendars+="OnCalendar=*-*-* ${hh}:${mm}:00
"
  t=$(( t + INTERVAL ))
done

SERVICE_CONTENT=$(cat <<UNIT
[Unit]
Description=autoship: watch $(basename "$WORKDIR") and ship closed-testing releases

[Service]
Type=oneshot
WorkingDirectory=${WORKDIR}
ExecStart=${EXE_ABS} ${VERB} --config ${CONFIG_ABS} --quiet
UNIT
)

TIMER_CONTENT=$(cat <<UNIT
[Unit]
Description=autoship schedule: every ${INTERVAL} min from ${START} for ${WINDOW}h, daily

[Timer]
${calendars}Persistent=false
AccuracySec=1min

[Install]
WantedBy=timers.target
UNIT
)

echo "Unit name:        ${NAME}.service / ${NAME}.timer"
echo "Command:          $EXE_ABS $VERB --config \"$CONFIG_ABS\" --quiet"
echo "Working dir:      $WORKDIR"
echo "Schedule:         every ${INTERVAL} min from ${START} for ${WINDOW}h, daily ($(( (end_min - start_min) / INTERVAL )) ticks/day)"
echo "Runs as:          your user session (systemd --user, not a system unit)"

if [[ $PRINT_ONLY -eq 1 ]]; then
  echo ""
  echo "--print-only: nothing was installed. Units would be:"
  echo "--- ${NAME}.service ---"
  echo "$SERVICE_CONTENT"
  echo "--- ${NAME}.timer ---"
  echo "$TIMER_CONTENT"
  exit 0
fi

mkdir -p "$UNIT_DIR"
printf '%s\n' "$SERVICE_CONTENT" > "$SERVICE_PATH"
printf '%s\n' "$TIMER_CONTENT" > "$TIMER_PATH"

systemctl --user daemon-reload
systemctl --user enable --now "${NAME}.timer"

echo ""
echo "Installed and started: $TIMER_PATH"
echo "Inspect it with:       systemctl --user list-timers ${NAME}.timer"
echo "Run it once now with:  systemctl --user start ${NAME}.service"
echo "Remove it with:        systemctl --user disable --now ${NAME}.timer && rm \"$SERVICE_PATH\" \"$TIMER_PATH\""
echo ""
echo "On a headless server, the user session (and this timer) normally stops"
echo "when you log out. Keep it running unattended with:"
echo "  sudo loginctl enable-linger \$USER"
