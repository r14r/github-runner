#!/usr/bin/env bash
# Open a PR that switches selected jobs to a repository-specific runner, then merge after checks pass.
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

usage() {
  cat <<'HELP'
Usage: ./03-setup.sh OWNER/REPO --workflow .github/workflows/ci.yml [options]

Options:
  --workflow PATH      Workflow to update (repeatable). If omitted, auto-select only
                       when there is exactly one .yml/.yaml workflow.
  --job JOB_ID         Migrate only this workflow job (repeatable).
  --allow-cross-os     Also replace Linux/Windows runner targets (review job commands).
  --allow-public       Explicitly acknowledge risks of a public self-hosted runner.
  --merge              Explicitly merge ONLY after passing required CI checks.
  --no-merge           Open a PR only (default).
  --base NAME          Base branch (default: repository default branch).
HELP
  exit 2
}
[[ $# -ge 1 ]] || usage
set_repo "$1"; shift
allow_public=false
allow_cross_os=false
auto_merge=false
base=''
workflows=()
jobs=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --workflow) [[ $# -ge 2 ]] || usage; workflows+=("$2"); shift 2 ;;
    --job) [[ $# -ge 2 ]] || usage; jobs+=("$2"); shift 2 ;;
    --allow-cross-os) allow_cross_os=true; shift ;;
    --allow-public) allow_public=true; shift ;;
    --merge) auto_merge=true; shift ;;
    --no-merge) auto_merge=false; shift ;;
    --base) [[ $# -ge 2 ]] || usage; base="$2"; shift 2 ;;
    *) usage ;;
  esac
done
select_backend
require_github_login
require git
require python3
check_public_repo "$allow_public"
if [[ "$RUNNER_SELECTED_BACKEND" == docker ]]; then
  docker_state_exists || fail "Runner not registered in $RUNNER_VOLUME. Run 02-configure.sh first."
  target_os=Linux
else
  [[ -f "$RUNNER_INSTANCE/.runner" ]] || fail "Runner not registered: $RUNNER_INSTANCE. Configure it first."
  target_os=macOS
fi

if [[ -z "$base" ]]; then
  base="$(gh repo view "$REPOSITORY" --json defaultBranchRef --jq '.defaultBranchRef.name')"
fi
[[ -n "$base" ]] || fail 'Could not resolve repository default branch.'
mkdir -p "$RUNNER_CHECKOUTS"
checkout="$(mktemp -d "$RUNNER_CHECKOUTS/$RUNNER_SLUG-XXXXXXXX")"
info "Cloning $REPOSITORY into $checkout"
gh repo clone "$REPOSITORY" "$checkout" -- --quiet
cd "$checkout"
git fetch --quiet origin "$base"
git switch --quiet "$base" 2>/dev/null || git switch --quiet -c "$base" "origin/$base"
git pull --ff-only --quiet origin "$base"

if [[ ${#workflows[@]} -eq 0 ]]; then
  shopt -s nullglob
  workflows=(.github/workflows/*.yml .github/workflows/*.yaml)
  shopt -u nullglob
  [[ ${#workflows[@]} -eq 1 ]] || {
    printf 'Detected %s workflows:\n' "${#workflows[@]}" >&2
    printf '  %s\n' "${workflows[@]}" >&2
    fail 'Specify each workflow to migrate using --workflow PATH.'
  }
fi
for workflow in "${workflows[@]}"; do
  [[ "$workflow" == .github/workflows/* && "$workflow" != *..* && ! -L "$workflow" ]] || fail "Workflow must be under .github/workflows/: '$workflow'"
  [[ -f "$workflow" ]] || fail "Missing workflow: '$workflow'"
done

branch="chore/self-hosted-runner-$(date -u +%Y%m%d%H%M%S)-$$"
git switch --quiet -c "$branch"
patch_args=(--label "$RUNNER_LABEL" --os "$target_os")
if [[ "$allow_cross_os" == true ]]; then patch_args+=(--allow-cross-os); fi
for job in "${jobs[@]}"; do patch_args+=(--job "$job"); done
python3 "$RUNNER_ROOT/lib/patch_workflow.py" "${patch_args[@]}" "${workflows[@]}"

git add -- "${workflows[@]}"
if git diff --cached --quiet; then
  info 'No changes needed; workflows already select this runner.'
  exit 0
fi
git -c user.name="$(gh api user --jq '.login')" -c user.email='github-actions-runner-setup@users.noreply.github.com' \
  commit --quiet -m "ci: use self-hosted runner $RUNNER_LABEL"
info "Pushing branch $branch"
git push --quiet --set-upstream origin "$branch"
pr_url="$(gh pr create --repo "$REPOSITORY" --base "$base" --head "$branch" \
  --title "ci: use self-hosted runner $RUNNER_LABEL" \
  --body "Route the selected GitHub Actions jobs to the repository-specific $target_os x64 self-hosted runner (label: \`$RUNNER_LABEL\`). Review OS-dependent steps before merging.")" || fail 'PR creation failed (the pushed branch was retained).'
info "Pull request: $pr_url"

# Explicitly check out the newly created PR in the local clone.
gh pr checkout "$pr_url" --repo "$REPOSITORY"
info "PR branch checked out in: $checkout"
if [[ "$auto_merge" == false ]]; then
  info 'PR created; review it and merge manually (opt in with --merge).'
  exit 0
fi
info 'Waiting for checks to appear; the runner must already be online.'
checks=0
for attempt in {1..12}; do
  checks="$(gh pr view "$pr_url" --repo "$REPOSITORY" --json statusCheckRollup --jq '.statusCheckRollup | length')" || fail 'Unable to inspect PR checks.'
  [[ "$checks" =~ ^[0-9]+$ ]] || fail "Invalid check count: $checks"
  (( checks > 0 )) && break
  sleep 5
done
if (( checks == 0 )); then
  fail "No checks appeared for $pr_url. Leaving PR OPEN; ensure workflows have pull_request triggers or merge manually."
fi
info 'Waiting for CI checks to complete successfully (Ctrl+C to stop).'
if ! gh pr checks "$pr_url" --repo "$REPOSITORY" --watch --fail-fast; then
  fail "CI checks did not pass. Leaving PR OPEN: $pr_url"
fi
info 'Checks passed; merging PR without bypassing required reviews or branch protection.'
gh pr merge "$pr_url" --repo "$REPOSITORY" --squash --delete-branch
info "Merged $pr_url"
