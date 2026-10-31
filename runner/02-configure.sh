#!/usr/bin/env bash
# Register one repository-specific runner using a fresh GitHub API registration token.
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

usage() {
  echo "Usage: $0 OWNER/REPO [--allow-public] [--name RUNNER_NAME] [--if-needed]"
  exit 2
}
[[ $# -ge 1 ]] || usage
set_repo "$1"; shift
allow_public=false
if_needed=false
name="$(hostname -s | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9._-')-$RUNNER_SLUG"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --allow-public) allow_public=true; shift ;;
    --if-needed) if_needed=true; shift ;;
    --name) [[ $# -ge 2 ]] || usage; name="$2"; shift 2 ;;
    *) usage ;;
  esac
done
[[ "$name" =~ ^[A-Za-z0-9_.-]+$ ]] || fail "Invalid runner name '$name'"
select_backend
require_github_login
check_public_repo "$allow_public"
if [[ "$RUNNER_SELECTED_BACKEND" == docker ]]; then
  docker image inspect "$RUNNER_IMAGE" >/dev/null 2>&1 || fail 'Run ./01-download.sh first (Docker runner image missing).'
  docker volume inspect "$RUNNER_VOLUME" >/dev/null 2>&1 || fail 'Run ./01-download.sh first (state volume missing).'
  if docker_state_exists; then
    if [[ "$if_needed" == true ]]; then
      info "Runner already configured in volume $RUNNER_VOLUME; preserving registration."
      exit 0
    fi
    fail "Runner already configured in volume $RUNNER_VOLUME"
  fi
else
  [[ -f "$RUNNER_INSTANCE/config.sh" ]] || fail "Run ./01-download.sh '$REPOSITORY' first."
  if [[ -f "$RUNNER_INSTANCE/.runner" ]]; then
    if [[ "$if_needed" == true ]]; then
      info "Runner already configured at $RUNNER_INSTANCE; preserving registration."
      exit 0
    fi
    fail "Runner already configured at $RUNNER_INSTANCE."
  fi
fi

info "Requesting a fresh registration token for $REPOSITORY via GitHub API"
# Registration tokens are short-lived. Never store one in the project or in git.
token="$(gh api --method POST "repos/$REPOSITORY/actions/runners/registration-token" --jq '.token')" || fail 'Cannot obtain a registration token (repository Administration permission is required).'
[[ -n "$token" && "$token" != null ]] || fail 'GitHub returned an empty registration token.'

info "Registering runner $name with label $RUNNER_LABEL"
if [[ "$RUNNER_SELECTED_BACKEND" == docker ]]; then
  # No token in Docker environment, command-line arguments, or persistent files.
  printf '%s\n' "$token" | docker run --rm --interactive --platform linux/amd64 \
    --mount "type=volume,source=$RUNNER_VOLUME,target=/runner" \
    --env "RUNNER_REPOSITORY=$REPOSITORY" \
    --env "RUNNER_NAME=$name" \
    --env "RUNNER_LABEL=$RUNNER_LABEL" \
    "$RUNNER_IMAGE" configure
  info "Registered Linux runner. Workflow label: [self-hosted, Linux, X64, $RUNNER_LABEL]"
else
  (
    cd "$RUNNER_INSTANCE"
    ./config.sh --unattended \
      --url "https://github.com/$REPOSITORY" \
      --token "$token" \
      --name "$name" \
      --labels "$RUNNER_LABEL" \
      --work _work
  )
  info "Registered macOS runner. Workflow label: [self-hosted, macOS, X64, $RUNNER_LABEL]"
fi
unset token
info "Start it with: ./04-run.sh $REPOSITORY start, or ./04-run.sh $REPOSITORY foreground"
