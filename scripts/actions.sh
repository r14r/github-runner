#!/usr/bin/env bash
# Repository-wide GitHub Actions cleanup. Never runs automatically during install.
set -euo pipefail
umask 077

REPO="${RUNNER_REPO:-r14r/BookBuilder-with-typst}"
ACTION="${1:-}"
case "$ACTION" in
  cancel|delete|clear) ;;
  *) echo "Usage: bash actions.sh {cancel|delete|clear}" >&2; exit 2 ;;
esac

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
command -v gh >/dev/null 2>&1 || fail "GitHub CLI not found (brew install gh)"
gh auth status >/dev/null || fail "Authenticate using gh auth login --scopes repo"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Capture all available pages before mutating runs (to avoid page shifting).
snapshot() {
  local target="$1"
  gh api --paginate "repos/$REPO/actions/runs?per_page=100" \
    --jq '.workflow_runs[] | [.id, .status] | @tsv' > "$target" ||
    fail "Unable to list Actions runs. Nothing has been changed by this snapshot operation."
  awk -F '\t' 'NF != 2 || $1 !~ /^[0-9]+$/ { bad=1 } END { exit bad }' "$target" ||
    fail "Invalid workflow-run response; refusing to act."
}

confirm_delete() {
  local count="$1" choice
  printf 'About to permanently delete %s workflow runs in %s (ALL branches/workflows).\n' "$count" "$REPO"
  echo "Logs and artifacts of these runs will also be removed."
  printf "Type DELETE %s to confirm: " "$REPO"
  IFS= read -r choice || fail "Confirmation not received; nothing deleted."
  [[ "$choice" == "DELETE $REPO" ]] || fail "Confirmation did not match; nothing deleted."
}

cancel_list() {
  local active="$1" id state failed=0 submitted=0
  while IFS=$'\t' read -r id state; do
    [[ -n "$id" ]] || continue
    printf 'Cancel %s (%s)\n' "$id" "$state"
    if gh run cancel "$id" --repo "$REPO"; then
      submitted=$((submitted + 1))
    else
      printf 'WARNING: could not cancel %s (may already be completed).\n' "$id" >&2
      failed=$((failed + 1))
    fi
  done < "$active"
  echo "Cancellation requested: $submitted; failed: $failed"
  (( failed == 0 ))
}

delete_ids() {
  local ids="$1" id failed=0 deleted=0
  while IFS= read -r id; do
    [[ -n "$id" ]] || continue
    printf 'Delete %s\n' "$id"
    if gh run delete "$id" --repo "$REPO"; then
      deleted=$((deleted + 1))
    else
      printf 'WARNING: could not delete %s.\n' "$id" >&2
      failed=$((failed + 1))
    fi
  done < "$ids"
  echo "Deleted: $deleted; failed: $failed"
  (( failed == 0 ))
}

await_completion() {
  local originals="$1" current="$2"
  local polls="${ACTIONS_MAX_POLLS:-60}" seconds="${ACTIONS_POLL_SECONDS:-5}"
  [[ "$polls" =~ ^[1-9][0-9]*$ ]] || fail "ACTIONS_MAX_POLLS must be a positive integer"
  [[ "$seconds" =~ ^[0-9]+$ ]] || fail "ACTIONS_POLL_SECONDS must be a nonnegative integer"
  local i remaining
  for (( i=1; i<=polls; i++ )); do
    snapshot "$current"
    remaining="$(awk -F '\t' 'NR==FNR { original[$1]=1; next } ($1 in original) && $2 != "completed" { n++ } END { print n+0 }' "$originals" "$current")"
    if [[ "$remaining" == "0" ]]; then return 0; fi
    echo "Still waiting for $remaining original runs to complete ($i/$polls checks)."
    if (( i < polls )); then sleep "$seconds"; fi
  done
  echo "Some cancellations are still pending; no runs were deleted. Retry after they complete." >&2
  return 1
}

snapshot "$work/original.tsv"
count="$(wc -l < "$work/original.tsv" | tr -d '[:space:]')"
echo "Found $count workflow run(s) in $REPO."

case "$ACTION" in
  cancel)
    awk -F '\t' '$2 != "completed"' "$work/original.tsv" > "$work/active.tsv"
    if [[ ! -s "$work/active.tsv" ]]; then echo "No active/queued runs to cancel."; exit 0; fi
    cancel_list "$work/active.tsv"
    echo "Cancellation is asynchronous; use just actions-clear later to delete completed runs."
    ;;
  delete)
    awk -F '\t' '$2 == "completed" { print $1 }' "$work/original.tsv" > "$work/deletable.txt"
    count="$(wc -l < "$work/deletable.txt" | tr -d '[:space:]')"
    if [[ "$count" == "0" ]]; then echo "No completed runs to delete."; exit 0; fi
    confirm_delete "$count"
    delete_ids "$work/deletable.txt"
    ;;
  clear)
    if [[ "$count" == "0" ]]; then echo "Nothing to clear."; exit 0; fi
    # Confirm before either canceling or deleting anything.
    confirm_delete "$count"
    awk -F '\t' '$2 != "completed"' "$work/original.tsv" > "$work/active.tsv"
    if [[ -s "$work/active.tsv" ]]; then
      cancel_list "$work/active.tsv" ||
        fail "Cancellation failed for one or more runs. No deletion attempted."
      await_completion "$work/active.tsv" "$work/current.tsv" ||
        fail "Cancellation not complete. No deletion attempted."
    else
      snapshot "$work/current.tsv"
    fi
    # Only delete runs from the initial snapshot, never newly-created runs.
    awk -F '\t' 'NR==FNR { original[$1]=1; next } ($1 in original) && $2 == "completed" { print $1 }' \
      "$work/original.tsv" "$work/current.tsv" > "$work/deletable.txt"
    if [[ ! -s "$work/deletable.txt" ]]; then echo "No original runs remain to delete."; exit 0; fi
    delete_ids "$work/deletable.txt"
    ;;
esac
