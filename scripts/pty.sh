#!/usr/bin/env bash
# Drives a TUI under a real tmux pty and captures what it rendered, so a
# subprocess pipe (which the program would see as a non-tty and never render
# to) never enters the picture. See ../docs/dogfooding.md for usage.
set -euo pipefail

SESSION="${JOJO_PTY_SESSION:-jojo-pty}"
READY_TIMEOUT_S="${JOJO_PTY_READY_TIMEOUT_S:-5}"

usage() {
	cat <<'EOF' >&2
Usage:
  pty.sh start <cols> <rows> -- <cmd> [args...]
  pty.sh keys <key> [key...]
  pty.sh capture [--plain]
  pty.sh stop
EOF
	exit 1
}

wait_for_output() {
	local waited=0
	local step="0.1"
	while [ "$(tmux capture-pane -p -t "$SESSION" | tr -d '[:space:]')" = "" ]; do
		sleep "$step"
		waited=$(awk -v w="$waited" -v s="$step" 'BEGIN { print w + s }')
		if awk -v w="$waited" -v t="$READY_TIMEOUT_S" 'BEGIN { exit !(w >= t) }'; then
			return 0
		fi
	done
}

cmd_start() {
	[ "$#" -ge 4 ] || usage
	local cols="$1" rows="$2"
	shift 2
	[ "$1" = "--" ] || usage
	shift

	tmux new-session -d -s "$SESSION" -x "$cols" -y "$rows" "$@"
	wait_for_output
}

cmd_keys() {
	[ "$#" -ge 1 ] || usage
	tmux send-keys -t "$SESSION" "$@"
}

cmd_capture() {
	if [ "${1:-}" = "--plain" ]; then
		tmux capture-pane -p -t "$SESSION"
	else
		tmux capture-pane -p -e -t "$SESSION"
	fi
}

cmd_stop() {
	tmux kill-session -t "$SESSION"
}

[ "$#" -ge 1 ] || usage
subcommand="$1"
shift

case "$subcommand" in
start) cmd_start "$@" ;;
keys) cmd_keys "$@" ;;
capture) cmd_capture "$@" ;;
stop) cmd_stop "$@" ;;
*) usage ;;
esac
