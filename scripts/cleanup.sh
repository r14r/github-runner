#!/usr/bin/env bash
# Safe runner housekeeping: delete only old, regular diagnostic .log files.
# Never change .runner, .credentials, _work, tool caches or runner services.
set -euo pipefail

DAYS="${1:-30}"
DRY_RUN="${RUNNER_CLEANUP_DRY_RUN:-0}"

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
[[ "$DAYS" =~ ^[1-9][0-9]*$ ]] || fail "Retention must be a positive integer number of days."
(( ${#DAYS} <= 4 )) || fail "Retention must be between 1 and 3650 days."
(( 10#$DAYS <= 3650 )) || fail "Retention must be between 1 and 3650 days."
[[ "$DRY_RUN" == "0" || "$DRY_RUN" == "1" ]] ||
  fail "RUNNER_CLEANUP_DRY_RUN must be 0 (delete) or 1 (preview)."

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
case "$(uname -s)" in
  Darwin) RUNNER_DIR="${RUNNER_DIR:-$here/.local/bookbuilder-actions-runner}" ;;
  Linux) RUNNER_DIR="${RUNNER_DIR:-/opt/bookbuilder-actions-runner}" ;;
  *) fail "Unsupported platform; macOS and Linux only." ;;
esac
[[ ! -L "$RUNNER_DIR" ]] || fail "Runner directory is a symlink: $RUNNER_DIR"
[[ -d "$RUNNER_DIR" ]] || fail "Runner missing at $RUNNER_DIR; run just install first."
diag="$RUNNER_DIR/_diag"
[[ ! -L "$diag" ]] || fail "Diagnostic directory is a symlink: $diag"

sizes() {
  echo "--- Runner storage ---"
  local name path
  for name in _diag _work _tool; do
    path="$RUNNER_DIR/$name"
    if [[ -d "$path" ]]; then
      du -sh "$path" || fail "Cannot measure disk usage: $path"
    fi
  done
  du -sh "$RUNNER_DIR" || fail "Cannot measure disk usage: $RUNNER_DIR"
}
echo "Runner: $RUNNER_DIR"
echo "Keeping diagnostic logs less than $DAYS days old."
[[ "$DRY_RUN" == "1" ]] && echo "DRY RUN: no files will be deleted."

if [[ ! -d "$diag" ]]; then
  echo "No _diag directory exists. Nothing to delete."
  sizes
  exit 0
fi
[[ -r "$diag" && -x "$diag" ]] || fail "Cannot read $diag."
[[ "$DRY_RUN" == "1" || -w "$diag" ]] || fail "Cannot modify $diag."

sizes
# BSD and GNU find round -mtime to full 24-hour periods:
# -mtime +29 matches files at least 30 complete days old.
age=$((10#$DAYS - 1))
candidates="$(mktemp)"
trap 'rm -f "$candidates"' EXIT
find "$diag" -maxdepth 1 -type f -name '*.log' -mtime "+$age" -print > "$candidates"
count="$(wc -l < "$candidates" | tr -d '[:space:]')"
printf 'Expired regular *.log files: %s\n' "$count"
if [[ "$count" == "0" ]]; then echo "Nothing to remove."; exit 0; fi
cat "$candidates"
if [[ "$DRY_RUN" == "1" ]]; then
  echo "Preview complete. To clean: just cleanup $DAYS"
else
  # Re-check age/type at deletion time and do not traverse nested directories,
  # symlinked logs or paths outside the runner's own _diag directory.
  find "$diag" -maxdepth 1 -type f -name '*.log' -mtime "+$age" -delete
  echo "Old diagnostic logs deleted."
  sizes
fi
