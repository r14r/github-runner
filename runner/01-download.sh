#!/usr/bin/env bash
# Download and verify the official macOS Intel GitHub Actions runner package.
set -euo pipefail
source "$(cd "$(dirname "$0")" && pwd)/lib/common.sh"

[[ $# -eq 1 ]] || fail "Usage: $0 OWNER/REPO"
set_repo "$1"
select_backend
if [[ "$RUNNER_SELECTED_BACKEND" == docker ]]; then
  bash "$RUNNER_ROOT/docker/doctor.sh"
  info "Building Linux x64 GitHub Actions runner $RUNNER_VERSION for $REPOSITORY (profile: $RUNNER_PROFILE)"
  docker build --platform linux/amd64 \
    --target "$RUNNER_PROFILE" \
    --build-arg "RUNNER_VERSION=$RUNNER_VERSION" \
    --build-arg "RUNNER_SHA256_LINUX_X64=$RUNNER_SHA256_LINUX_X64" \
    --tag "$RUNNER_IMAGE" \
    --file "$RUNNER_ROOT/docker/Dockerfile" "$RUNNER_ROOT/docker"
  if [[ "$RUNNER_PROFILE" == bookbuilder ]]; then
    info "Validating BookBuilder dependencies inside the new image"
    docker run --rm --interactive --platform linux/amd64 \
      --entrypoint bash "$RUNNER_IMAGE" -s < "$RUNNER_ROOT/../doctor-linux.sh"
  fi
  docker volume create "$RUNNER_VOLUME" >/dev/null
  info "Docker runner image ready: $RUNNER_IMAGE (volume: $RUNNER_VOLUME)"
  exit 0
fi
require curl
require shasum
require tar

if [[ -e "$RUNNER_INSTANCE/config.sh" ]]; then
  info "Runner binaries already installed at $RUNNER_INSTANCE (not overwriting)."
  exit 0
fi
mkdir -p "$RUNNER_INSTANCE" "$RUNNER_CACHE"
archive="actions-runner-osx-x64-$RUNNER_VERSION.tar.gz"
package="$RUNNER_CACHE/$archive"
url="https://github.com/actions/runner/releases/download/v$RUNNER_VERSION/$archive"

if [[ ! -f "$package" ]]; then
  info "Downloading $url"
  curl --fail --location --retry 3 --silent --show-error --output "$package" "$url"
fi
info 'Checking SHA-256 checksum'
if ! (cd "$RUNNER_CACHE" && printf '%s  %s\n' "$RUNNER_SHA256_OSX_X64" "$archive" | shasum -a 256 -c -); then
  rm -f "$package"
  fail 'Checksum mismatch. Cached download has been removed.'
fi
info "Installing $RUNNER_VERSION into $RUNNER_INSTANCE"
tar -xzf "$package" -C "$RUNNER_INSTANCE"
[[ -f "$RUNNER_INSTANCE/config.sh" && -f "$RUNNER_INSTANCE/run.sh" ]] || fail 'Runner archive is incomplete.'
info 'Download complete.'
