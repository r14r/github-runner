#!/usr/bin/env bash
# Launch the runner in the terminal or manage its launchd service on macOS.
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

[[ $# -ge 1 && $# -le 2 ]] || fail "Usage: $0 OWNER/REPO [foreground|install|start|stop|status|monitor|uninstall]"
set_repo "$1"
action="${2:-foreground}"
select_backend

github_status() {
  local name="${1:-}" rows line
  [[ -n "$name" ]] || { info 'GitHub registration name unavailable'; return 0; }
  if ! command -v gh >/dev/null 2>&1 || ! gh auth status >/dev/null 2>&1; then
    info 'GitHub online/busy status unavailable (gh authentication needed).'
    return 0
  fi
  if ! rows="$(gh api --paginate "repos/$REPOSITORY/actions/runners?per_page=100" \
      --jq '.runners[] | [.name, .status, (.busy|tostring)] | @tsv' 2>/dev/null)"; then
    info 'GitHub runner API unavailable (Administration: Read permission needed).'
    return 0
  fi
  local found=false label state busy
  while IFS=$'\t' read -r label state busy; do
    if [[ "$label" == "$name" ]]; then
      info "GitHub status: $state; busy=$busy; name=$name"
      found=true
      break
    fi
  done <<< "$rows"
  [[ "$found" == true ]] || info "GitHub status: runner '$name' not found. Check registration."
}

if [[ "$RUNNER_SELECTED_BACKEND" == docker ]]; then
  docker_state_exists || fail "Runner not registered in $RUNNER_VOLUME. Run 02-configure.sh first."
  case "$action" in
    install)
      if docker_container_exists; then
        info "Runner container $RUNNER_CONTAINER already exists."
      else
        docker_create_runner
        info "Installed container $RUNNER_CONTAINER (Docker socket access is opt-in)."
      fi
      ;;
    start)
      if ! docker_container_exists; then docker_create_runner; fi
      docker start "$RUNNER_CONTAINER"
      ;;
    stop)
      docker stop "$RUNNER_CONTAINER"
      ;;
    status)
      if docker_container_exists; then
        docker container inspect --format '{{.Name}}: {{.State.Status}} (restart: {{.HostConfig.RestartPolicy.Name}})' "$RUNNER_CONTAINER"
      else
        info "Docker container not installed: $RUNNER_CONTAINER"
      fi
      runner_name="$(docker run --rm --platform linux/amd64 \
        --mount "type=volume,source=$RUNNER_VOLUME,target=/runner" \
        "$RUNNER_IMAGE" identity 2>/dev/null)" || runner_name=''
      github_status "$runner_name"
      ;;
    monitor)
      while true; do
        "$RUNNER_ROOT/04-run.sh" "$REPOSITORY" status
        gh run list --repo "$REPOSITORY" --limit 5 2>/dev/null || true
        docker logs --tail 8 "$RUNNER_CONTAINER" 2>/dev/null || true
        sleep "${RUNNER_MONITOR_SECONDS:-10}"
      done
      ;;
    uninstall)
      if docker_container_exists; then docker rm -f "$RUNNER_CONTAINER"; fi
      info "Volume $RUNNER_VOLUME and registration retained."
      ;;
    foreground)
      if ! docker_container_exists; then docker_create_runner; fi
      if [[ "$(docker container inspect --format '{{.State.Running}}' "$RUNNER_CONTAINER")" == true ]]; then
        fail 'Runner is already running. Use docker logs -f to follow its output.'
      fi
      docker start --attach "$RUNNER_CONTAINER"
      ;;
    *) fail "Unsupported action: $action" ;;
  esac
  exit 0
fi
[[ -f "$RUNNER_INSTANCE/.runner" ]] || fail "Runner is not configured at $RUNNER_INSTANCE. Run 02-configure.sh first."

case "$action" in
  foreground)
    info "Running $REPOSITORY in the foreground; use Ctrl+C to stop."
    cd "$RUNNER_INSTANCE"
    exec ./run.sh
    ;;
  install|start|stop|status|uninstall)
    [[ -x "$RUNNER_INSTANCE/svc.sh" ]] || fail 'Runner service script svc.sh is missing.'
    cd "$RUNNER_INSTANCE"
    case "$action" in
      install) ./svc.sh install ;;
      start) ./svc.sh start ;;
      stop) ./svc.sh stop ;;
      status)
        ./svc.sh status
        require jq
        github_status "$(jq -r '.agentName // empty' .runner)"
        ;;
      uninstall) ./svc.sh uninstall ;;
    esac
    ;;
  monitor)
    while true; do
      "$RUNNER_ROOT/04-run.sh" "$REPOSITORY" status
      gh run list --repo "$REPOSITORY" --limit 5 2>/dev/null || true
      sleep "${RUNNER_MONITOR_SECONDS:-10}"
    done
    ;;
  *) fail "Unsupported action: $action" ;;
esac
