#!/usr/bin/env bash
# Lifecycle and monitoring commands for an existing BookBuilder GitHub runner.
set -euo pipefail
REPO="${RUNNER_REPO:-r14r/BookBuilder-with-typst}"
OS="$(uname -s)"
case "$OS" in
  Darwin)
    here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
    RUNNER_DIR="${RUNNER_DIR:-$here/.local/bookbuilder-actions-runner}"
    ;;
  Linux) RUNNER_DIR="${RUNNER_DIR:-/opt/bookbuilder-actions-runner}" ;;
  *) echo "Unsupported runner OS: $OS" >&2; exit 1 ;;
esac

service() {
  local action="$1"
  if [[ ! -f "$RUNNER_DIR/svc.sh" ]]; then
    printf 'Runner service not installed at %s. Run just install first.\n' "$RUNNER_DIR" >&2
    return 1
  fi
  if [[ "$OS" == "Darwin" ]]; then
    (cd "$RUNNER_DIR" && bash ./svc.sh "$action")
  else
    # Only service administration uses sudo; CI still runs as a restricted user.
    sudo bash -c 'cd "$1" && bash ./svc.sh "$2"' _ "$RUNNER_DIR" "$action"
  fi
}

github_status() {
  if [[ ! -f "$RUNNER_DIR/.runner" ]]; then
    echo "GitHub registration: NOT configured locally (run just install)"
    return
  fi
  if ! command -v gh >/dev/null 2>&1 || ! command -v jq >/dev/null 2>&1; then
    echo "GitHub registration: status unavailable (need gh and jq)"
    return
  fi
  local name response state busy
  name="$(jq -r '.agentName // empty' "$RUNNER_DIR/.runner")"
  if [[ -z "$name" ]]; then
    echo "GitHub registration: invalid local .runner identity"
    return
  fi
  if ! response="$(gh api "repos/$REPO/actions/runners?per_page=100" 2>/dev/null)"; then
    echo "GitHub registration: API query failed; check gh auth and runner admin access"
    return
  fi
  state="$(printf '%s' "$response" | jq -r --arg name "$name" \
    '[.runners[]? | select(.name == $name)] | first | .status // "missing"')"
  busy="$(printf '%s' "$response" | jq -r --arg name "$name" \
    '[.runners[]? | select(.name == $name)] | first | .busy // false')"
  echo "GitHub runner: $name | status=$state | busy=$busy"
  if [[ "$state" == "missing" || "$state" == "offline" ]]; then
    echo "  If diagnostics say registration deleted, run: just repair"
  fi
}

diagnostics() {
  local diag="$RUNNER_DIR/_diag"
  if [[ ! -d "$diag" ]]; then echo "No runner diagnostics directory yet"; return; fi
  local f latest=""
  for f in "$diag"/Runner_*.log; do
    [[ -f "$f" ]] || continue
    if [[ -z "$latest" || "$f" > "$latest" ]]; then latest="$f"; fi
  done
  if [[ -n "$latest" ]]; then
    echo "Log: $latest"
    tail -n 8 "$latest"
    if [[ "$(tail -n 120 "$latest")" == *"The runner registration has been deleted from the server"* ]]; then
      echo "WARNING: Runner rejected by GitHub (stale registration). Run just repair after ensuring no job is running."
    fi
  else
    echo "No runner diagnostic logs yet"
  fi
}

monitor() {
  local interval="${1:-10}"
  if [[ ! "$interval" =~ ^[0-9]+$ ]] || (( 10#$interval < 2 )); then
    echo "Monitor interval must be an integer >= 2 seconds" >&2
    exit 2
  fi
  [[ -f "$RUNNER_DIR/svc.sh" ]] || {
    echo "Runner not installed at $RUNNER_DIR. Run just install first." >&2
    exit 1
  }
  echo "Monitoring every ${interval}s (Ctrl+C to stop)"
  while true; do
    if [[ -t 1 ]]; then printf '\033[2J\033[H'; fi
    printf '\nGitHub Runner | %s | %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$RUNNER_DIR"
    echo "--- Service ---"
    service status || echo "Service status unavailable"
    github_status
    echo "--- Recent GitHub Actions ---"
    if command -v gh >/dev/null 2>&1; then
      gh run list --repo "$REPO" --limit 8 || echo "GitHub runs unavailable (check gh auth status)"
    else
      echo "GitHub CLI unavailable"
    fi
    echo "--- Last runner diagnostics ---"
    diagnostics
    [[ "${RUNNER_MONITOR_ONCE:-0}" == "1" ]] && break
    sleep "$interval"
  done
}

case "${1:-}" in
  start|stop) service "$1" ;;
  status) service status; github_status ;;
  monitor) monitor "${2:-10}" ;;
  *) echo "Usage: bash control.sh {start|stop|status|monitor [seconds]}" >&2; exit 2 ;;
esac
