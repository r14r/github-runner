#!/usr/bin/env bash
# Provision a dedicated Ubuntu 24.04 x64 VM and register its repository runner.
# Run from an authenticated administrator account, NOT as root.
set -euo pipefail

REPO="${RUNNER_REPO:-r14r/BookBuilder-with-typst}"
RUNNER_USER="${RUNNER_USER:-bookbuilder-runner}"
RUNNER_DIR="${RUNNER_DIR:-/opt/bookbuilder-actions-runner}"
RUNNER_NAME="${RUNNER_NAME:-$(hostname -s)-bookbuilder}"
RUNNER_VERSION="2.336.0"
RUNNER_SHA256="04cf0be1aff4c3ec3554466c39124ca250e3effd8873bb7e8d68535aa9505d5d"
TYPST_VERSION="0.15.1"
TYPST_SHA256="a6d077d0a95eed5a2eba715b2dae06be954f624ccbf85758a03f389ded33118c"

fail() { echo "ERROR: $*" >&2; exit 1; }
msg() { printf '[bookbuilder runner] %s\n' "$*"; }

[[ "$(id -u)" -ne 0 ]] || fail "Run as a normal account with sudo and gh authentication, not as root."
[[ "$(uname -m)" == "x86_64" ]] || fail "Only x86_64 is supported."
[[ -r /etc/os-release ]] || fail "Missing /etc/os-release."
# shellcheck source=/dev/null
source /etc/os-release
[[ "$ID" == "ubuntu" && "$VERSION_ID" == "24.04" ]] || fail "Use a dedicated Ubuntu 24.04 LTS VM."
command -v sudo >/dev/null || fail "sudo is required for host provisioning."
sudo -v

msg "Installing operating-system tools and render dependencies"
sudo apt-get update
sudo apt-get install -y --no-install-recommends software-properties-common ca-certificates curl   git gh jq unzip zip xz-utils tar build-essential pkg-config   python3.12 python3.12-venv python3-pip python3-dev   openjdk-21-jdk-headless ffmpeg fontconfig fonts-dejavu-core fonts-liberation   fonts-texgyre just libicu74 libssl3t64 libkrb5-3 zlib1g

# Ubuntu 24.04 only provides PHP 8.3 in its standard archive.
# Opt in to the well-known third-party Ondrej PHP PPA to install PHP 8.2 once.
if ! dpkg-query -W -f='${Status}' php8.2-cli 2>/dev/null | grep -qx "install ok installed"; then
  msg "Enabling the Ondrej PHP PPA for PHP 8.2 (third-party apt source)"
  sudo add-apt-repository -y ppa:ondrej/php
  sudo apt-get update
fi
sudo apt-get install -y --no-install-recommends php8.2-cli php8.2-curl php8.2-zip php8.2-mbstring
sudo update-alternatives --set php /usr/bin/php8.2

if ! command -v typst >/dev/null || [[ ! "$(typst --version)" =~ ^typst[[:space:]]0[.]15[.]1([[:space:]]|$) ]]; then
  msg "Installing Typst ${TYPST_VERSION} with pinned SHA-256"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  asset="typst-x86_64-unknown-linux-musl.tar.xz"
  curl --fail --location --retry 3 \
    "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/${asset}" -o "$tmp/$asset"
  printf '%s  %s\n' "$TYPST_SHA256" "$tmp/$asset" | sha256sum -c -
  tar -xJf "$tmp/$asset" -C "$tmp"
  sudo install -m 0755 "$tmp/typst-x86_64-unknown-linux-musl/typst" /usr/local/bin/typst
  rm -rf "$tmp"
  trap - EXIT
fi

msg "Verifying the host tools before registering the runner"
bash "$(dirname "$0")/doctor.sh"

command -v gh >/dev/null || fail "Install GitHub CLI and authenticate before registration."
gh auth status >/dev/null || fail "Run 'gh auth login --scopes repo' with repository admin permission."
if ! id "$RUNNER_USER" >/dev/null 2>&1; then
  sudo useradd --system --create-home --home-dir /home/"$RUNNER_USER" --shell /bin/bash "$RUNNER_USER"
fi
sudo install -d -o "$RUNNER_USER" -g "$RUNNER_USER" -m 0750 "$RUNNER_DIR"

if ! sudo test -f "$RUNNER_DIR/config.sh"; then
  msg "Downloading GitHub Actions runner ${RUNNER_VERSION} (SHA-256 pinned)"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  asset="actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz"
  curl --fail --location --retry 3 \
    "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/${asset}" -o "$tmp/$asset"
  printf '%s  %s\n' "$RUNNER_SHA256" "$tmp/$asset" | sha256sum -c -
  sudo tar -xzf "$tmp/$asset" -C "$RUNNER_DIR"
  sudo chown -R "$RUNNER_USER:$RUNNER_USER" "$RUNNER_DIR"
  rm -rf "$tmp"
  trap - EXIT
fi

if ! sudo test -f "$RUNNER_DIR/.runner"; then
  msg "Obtaining a short-lived registration token via the authenticated gh CLI"
  token="$(gh api --method POST "repos/${REPO}/actions/runners/registration-token" --jq '.token')"
  [[ -n "$token" ]] || fail "GitHub registration token not available (repository admin access required)."
  sudo -u "$RUNNER_USER" bash -c 'cd "$1" && ./config.sh --unattended --url "$2" --token "$3" --name "$4" --labels "$5" --work _work' \
    _ "$RUNNER_DIR" "https://github.com/${REPO}" "$token" "$RUNNER_NAME" "${RUNNER_LABEL:-bookbuilder}"
  unset token
else
  msg "Runner is already registered; retaining its existing identity"
fi

if ! sudo test -f "$RUNNER_DIR/.service"; then
  msg "Installing systemd service as non-privileged user"
  sudo bash -c 'cd "$1" && ./svc.sh install "$2"' _ "$RUNNER_DIR" "$RUNNER_USER"
fi
# Avoid apt/needrestart terminating the runner mid-job.
if [[ -d /etc/needrestart/conf.d ]]; then
  echo '$nrconf{override_rc}{qr(^actions\.runner\..+\.service$)} = 0;' |
    sudo tee /etc/needrestart/conf.d/bookbuilder_actions_runner.conf >/dev/null
fi
sudo bash -c 'cd "$1" && ./svc.sh start && ./svc.sh status' _ "$RUNNER_DIR"

msg "Registration completed. Verify the runner is Online at:"
msg "https://github.com/${REPO}/settings/actions/runners"
msg "Do not add sudo privileges or Docker socket access to ${RUNNER_USER}."
