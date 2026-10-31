#!/usr/bin/env bash
# Shared helpers for per-repository GitHub Actions runners.
set -euo pipefail

RUNNER_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUNNER_INSTANCES="${RUNNER_INSTANCES:-$RUNNER_ROOT/../.local/instances}"
RUNNER_CHECKOUTS="${RUNNER_CHECKOUTS:-$RUNNER_ROOT/../.local/checkouts}"
RUNNER_CACHE="${RUNNER_CACHE:-$RUNNER_ROOT/../.local/cache}"
RUNNER_VERSION="2.338.1"
RUNNER_SHA256_OSX_X64="f06e329ffe90a2a4bda8b460a20b9e26dcb7525a093bf9b4f89125c72bc1f39e"
RUNNER_SHA256_LINUX_X64="08e9ccb3604d2462c24b5e33604dc30810296ca15cc65f23cffd4317eb144c37"
# Resolve image profile per repository. Explicit image override remains supported.
RUNNER_IMAGE_OVERRIDE="${RUNNER_IMAGE:-}"
RUNNER_PROFILE="${RUNNER_PROFILE:-auto}"
RUNNER_IMAGE=""


fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
info() { printf '==> %s\n' "$*" >&2; }
require() { command -v "$1" >/dev/null 2>&1 || fail "Missing tool: $1"; }

set_repo() {
  local raw="${1:-}"
  raw="${raw#https://github.com/}"
  raw="${raw#http://github.com/}"
  raw="${raw%.git}"
  raw="${raw%/}"
  [[ "$raw" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || fail "Repository must be OWNER/REPO (got: '$raw')"
  REPOSITORY="$raw"
  RUNNER_SLUG="$(printf '%s' "$raw" | tr '[:upper:]/' '[:lower:]-' | tr -cd 'a-z0-9._-')"
  RUNNER_INSTANCE="$RUNNER_INSTANCES/$RUNNER_SLUG"
  RUNNER_LABEL="repo-$RUNNER_SLUG"
  RUNNER_CONTAINER="github-runner-$RUNNER_SLUG"
  RUNNER_VOLUME="github-runner-$RUNNER_SLUG-state"
  case "$RUNNER_PROFILE" in
    auto)
      if [[ "$RUNNER_SLUG" == r14r-bookbuilder ]]; then
        RUNNER_PROFILE=bookbuilder
      else
        RUNNER_PROFILE=runner-base
      fi
      ;;
    generic) RUNNER_PROFILE=runner-base ;;
    runner-base|bookbuilder) ;;
    *) fail "RUNNER_PROFILE must be auto, generic, runner-base, or bookbuilder." ;;
  esac
  if [[ -n "$RUNNER_IMAGE_OVERRIDE" ]]; then
    RUNNER_IMAGE="$RUNNER_IMAGE_OVERRIDE"
  elif [[ "$RUNNER_PROFILE" == bookbuilder ]]; then
    RUNNER_IMAGE="github-actions-runner-local:$RUNNER_VERSION-bookbuilder"
  else
    RUNNER_IMAGE="github-actions-runner-local:$RUNNER_VERSION"
  fi

}

require_macos_supported() {
  [[ "$(uname -s)" == Darwin ]] || fail 'This package supports macOS Intel (Darwin x86_64) only.'
  [[ "$(uname -m)" == x86_64 ]] || fail 'This release bundle is for Intel Macs (x86_64) only.'
  require sw_vers
  local os_ver major minor
  os_ver="$(sw_vers -productVersion)"
  major="${os_ver%%.*}"
  minor="$(printf '%s' "$os_ver" | cut -d. -f2)"
  [[ "$major" =~ ^[0-9]+$ && "$minor" =~ ^[0-9]+$ ]] || fail "Cannot parse macOS version: $os_ver"
  if (( major < 13 || (major == 13 && minor < 5) )); then
    fail "macOS $os_ver is too old for runner $RUNNER_VERSION (Node.js 24 requires macOS >= 13.5). Use a newer Mac or supported Linux runner."
  fi
}

require_github_login() {
  require gh
  gh auth status >/dev/null 2>&1 || fail 'GitHub CLI not authenticated. Run: gh auth login'
}

check_public_repo() {
  local allow_public="${1:-false}" visibility
  visibility="$(gh repo view "$REPOSITORY" --json visibility --jq '.visibility')" || fail "Cannot read repository $REPOSITORY"
  if [[ "$visibility" == PUBLIC && "$allow_public" != true ]]; then
    fail "Repository $REPOSITORY is public. Self-hosted runners can execute untrusted PR code. Re-run with --allow-public only after securing workflow triggers and isolating the runner host."
  fi
}

# RUNNER_BACKEND=auto chooses Colima/Docker on Monterey and native macOS on supported Intel Macs.
# Set RUNNER_BACKEND=docker explicitly to use Docker on any supported host.
select_backend() {
  local requested="${RUNNER_BACKEND:-auto}" os_ver major minor
  if [[ "$requested" == auto ]]; then
    case "$(uname -s)" in
      Darwin)
        os_ver="$(sw_vers -productVersion)"
        major="${os_ver%%.*}"
        minor="$(printf '%s' "$os_ver" | cut -d. -f2)"
        if (( major < 13 || (major == 13 && minor < 5) )); then
          requested=docker
          info "macOS $os_ver cannot run Node.js 24: using a Linux x64 runner in Docker (Colima)."
        else
          requested=macos
        fi
        ;;
      *) requested=docker ;;
    esac
  fi
  [[ "$requested" == docker || "$requested" == macos ]] || fail 'RUNNER_BACKEND must be auto, docker, or macos.'
  RUNNER_SELECTED_BACKEND="$requested"
  if [[ "$requested" == docker ]]; then
    require docker
    docker info >/dev/null 2>&1 || fail 'Docker daemon unreachable. Start Colima with: colima start (then docker context use colima).'
    local architecture
    architecture="$(docker info --format '{{.Architecture}}')"
    [[ "$architecture" == x86_64 || "$architecture" == amd64 ]] || fail "Docker host is $architecture; this bundle is linux/amd64 only."
  else
    require_macos_supported
  fi
}

docker_state_exists() {
  docker volume inspect "$RUNNER_VOLUME" >/dev/null 2>&1 || return 1
  docker run --rm --platform linux/amd64 \
    --mount "type=volume,source=$RUNNER_VOLUME,target=/runner" \
    "$RUNNER_IMAGE" check >/dev/null 2>&1
}

docker_container_exists() {
  docker container inspect "$RUNNER_CONTAINER" >/dev/null 2>&1
}

docker_create_runner() {
  # Docker socket access is NOT the default: explicit opt-in is required.
  local extra=()
  if [[ "${RUNNER_DOCKER_SOCKET:-0}" == 1 ]]; then
    info 'SECURITY WARNING: allowing CI to control Colima Docker daemon.'
    extra+=(--mount "type=bind,source=/var/run/docker.sock,target=/var/run/docker.sock")
  elif [[ "${RUNNER_DOCKER_SOCKET:-0}" != 0 ]]; then
    fail 'RUNNER_DOCKER_SOCKET must be 0 or 1'
  fi
  docker create --platform linux/amd64 \
    --name "$RUNNER_CONTAINER" \
    --restart unless-stopped \
    --mount "type=volume,source=$RUNNER_VOLUME,target=/runner" \
    "${extra[@]}" \
    "$RUNNER_IMAGE" run >/dev/null
}
