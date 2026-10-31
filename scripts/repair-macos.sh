#!/usr/bin/env bash
# Explicit recovery from a stale/missing GitHub Actions runner registration.
# Does NOT remove _work, _diag or the installed runner binaries.
set -euo pipefail
umask 077

[[ "$(uname -s)" == "Darwin" ]] || { echo "just repair supports macOS only." >&2; exit 1; }
[[ "$(id -u)" -ne 0 ]] || { echo "Run as the runner account, not root." >&2; exit 1; }
REPO="${RUNNER_REPO:-r14r/BookBuilder-with-typst}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
RUNNER_DIR="${RUNNER_DIR:-$(dirname "$here")/.local/bookbuilder-actions-runner}"
fail() { printf 'Repair refused: %s\n' "$*" >&2; exit 1; }
log() { printf '[BookBuilder runner repair] %s\n' "$*"; }

[[ ! -L "$RUNNER_DIR" ]] || fail "Refusing runner symlink: $RUNNER_DIR"
[[ -f "$RUNNER_DIR/.runner" ]] || fail "No local .runner configuration. Run just install."
[[ -f "$RUNNER_DIR/config.sh" && -f "$RUNNER_DIR/svc.sh" ]] ||
  fail "Runner files incomplete at $RUNNER_DIR. Do not remove credentials manually."
command -v gh >/dev/null 2>&1 || fail "GitHub CLI missing: brew install gh"
command -v jq >/dev/null 2>&1 || fail "jq missing: brew install jq"
gh auth status >/dev/null || fail "Authenticate first: gh auth login --scopes repo"

runner_name="$(jq -r '.agentName // empty' "$RUNNER_DIR/.runner")"
[[ -n "$runner_name" ]] || fail "Local .runner has no agentName. Inspect before proceeding."

# Refuse to affect a valid, online runner, or any busy job on GitHub.
# Use the API, rather than trusting launchd's 'Started' output.
remote="$(gh api "repos/$REPO/actions/runners?per_page=100")" ||
  fail "Cannot query GitHub runner registrations. Check gh authentication/admin permission."
state="$(printf '%s' "$remote" | jq -r --arg name "$runner_name" \
  '[.runners[]? | select(.name == $name)] | first | .status // "missing"')"
busy="$(printf '%s' "$remote" | jq -r --arg name "$runner_name" \
  '[.runners[]? | select(.name == $name)] | first | .busy // false')"
[[ "$busy" != "true" ]] || fail "GitHub reports $runner_name busy. Wait for jobs to finish."
[[ "$state" != "online" ]] || fail "GitHub reports $runner_name ONLINE. Do not replace its registration. Inspect the latest _diag logs first."

log "GitHub runner $runner_name: $state; repairing local registration."
log "Stop running all CI jobs before proceeding; do not run this during an active build."
if [[ -f "$RUNNER_DIR/.service" ]]; then
  (cd "$RUNNER_DIR" && bash ./svc.sh stop) || log "Old launchd service was already stopped."
  (cd "$RUNNER_DIR" && bash ./svc.sh uninstall) ||
    fail "Could not uninstall launchd service. No registration data was changed."
  [[ ! -f "$RUNNER_DIR/.service" ]] ||
    fail "launchd .service marker still exists; registration data left unchanged."
fi

# The installed runner's supported --local flag only removes local registration
# and credentials; it does NOT issue a remote DELETE. Verified in actions/runner
# v2.338.1 Runner.cs and CommandSettings.cs. Preserve _work and _diag.
log "Removing only stale local runner config with ./config.sh remove --local"
(cd "$RUNNER_DIR" && bash ./config.sh remove --local) ||
  fail "Local cleanup failed. Do not manually delete the entire runner directory."
[[ ! -f "$RUNNER_DIR/.runner" ]] || fail "Old .runner remains after local cleanup."

token="$(gh api --method POST "repos/$REPO/actions/runners/registration-token" --jq .token)"
[[ -n "$token" ]] || fail "Could not obtain a GitHub registration token."
log "Registering fresh credentials for $runner_name (reusing installed binaries)."
if ! (cd "$RUNNER_DIR" && bash ./config.sh --unattended \
  --url "https://github.com/$REPO" --token "$token" \
  --name "$runner_name" --labels "${RUNNER_LABEL:-bookbuilder}" --work _work --replace); then
  unset token
  fail "GitHub registration failed. Local credentials have been reset; rerun just install or just repair after fixing access."
fi
unset token
(
  cd "$RUNNER_DIR"
  bash ./svc.sh install
  bash ./svc.sh start
  bash ./svc.sh status
)
log "Repair launched; verify Online using just status and GitHub Runner Settings."
