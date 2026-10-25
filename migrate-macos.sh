#!/usr/bin/env bash
# Move an existing macOS GitHub runner into this repository without re-registering it.
# Safely retire the old launchd service before changing the absolute runner path.
set -euo pipefail
umask 077

[[ "$(uname -s)" == "Darwin" ]] || { echo "Runner directory migration is macOS-only." >&2; exit 1; }
[[ "$(id -u)" -ne 0 ]] || { echo "Run migration as the runner account, never as root." >&2; exit 1; }

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
source_dir="${LEGACY_RUNNER_DIR:-$HOME/.local/share/bookbuilder-actions-runner}"
target_dir="${RUNNER_DIR:-$here/.local/bookbuilder-actions-runner}"

fail() { printf 'Migration error: %s\n' "$*" >&2; exit 1; }
log() { printf '[BookBuilder runner migration] %s\n' "$*"; }

[[ "$source_dir" != "$target_dir" ]] || { log "Old and new locations are identical; nothing to move."; exit 0; }
[[ ! -L "$source_dir" ]] || fail "Legacy runner path is a symlink; refuse to migrate automatically: $source_dir"
[[ ! -L "$target_dir" ]] || fail "Target runner path is a symlink; refuse to overwrite: $target_dir"

if [[ ! -e "$source_dir" ]]; then
  if [[ -d "$target_dir" ]]; then
    log "No legacy installation; runner already uses: $target_dir"
  else
    log "No legacy installation found; a fresh install will use: $target_dir"
  fi
  exit 0
fi
[[ -d "$source_dir" ]] || fail "Legacy runner path is not a directory: $source_dir"

# Refuse to merge two installations: runner credentials and service state are unique.
if [[ -e "$target_dir" ]]; then
  fail "Both legacy and target runner directories exist. Nothing was changed. Check $source_dir and $target_dir before retrying."
fi
if [[ ! -f "$source_dir/config.sh" && ! -f "$source_dir/.runner" ]]; then
  # Older installers created this directory before checking prerequisites.
  # An empty placeholder contains no registration and can be removed safely.
  if [[ -z "$(ls -A "$source_dir")" ]]; then
    rmdir "$source_dir"
    log "Removed empty placeholder at legacy location; install will provision the new path."
    exit 0
  fi
  fail "Legacy directory has unknown contents and no runner configuration. Inspect it manually: $source_dir"
fi

# Important: stopping the runner will interrupt any current workflow job.
if [[ -f "$source_dir/.service" ]]; then
  [[ -f "$source_dir/svc.sh" ]] || fail "A service is recorded but svc.sh is missing: $source_dir"
  log "Stopping old launchd service (do not migrate during an active CI job)"
  (cd "$source_dir" && bash ./svc.sh stop) || log "Old runner was already stopped"
  log "Uninstalling the old launchd service without removing GitHub registration"
  (cd "$source_dir" && bash ./svc.sh uninstall) ||
    fail "Could not uninstall old service. Runner directory has NOT been moved."
  [[ ! -f "$source_dir/.service" ]] ||
    fail "Old .service marker persists after uninstall. Runner directory has NOT been moved."
fi

log "Moving runner files (including .runner, credentials, _work, _diag) to: $target_dir"
mkdir -p "$(dirname "$target_dir")"
chmod 700 "$(dirname "$target_dir")"
mv "$source_dir" "$target_dir" ||
  fail "Move failed; check both paths and recover files before retrying."
chmod 700 "$target_dir"

# This is a new absolute path, so the old launchd plist must not be reused.
if [[ -f "$target_dir/.runner" ]]; then
  [[ -f "$target_dir/svc.sh" ]] || fail "Runner moved but svc.sh missing; repair installation at $target_dir"
  log "Reinstalling launchd service for new absolute path (existing registration retained)"
  (
    cd "$target_dir"
    bash ./svc.sh install
    bash ./svc.sh start
    bash ./svc.sh status
  ) || fail "Runner moved to $target_dir, but launchd setup failed. Rerun just install to repair it."
fi
log "Migration finished; GitHub runner identity preserved without a new registration token."
