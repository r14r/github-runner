#!/usr/bin/env bash
# Install all BookBuilder workflow dependencies and a repository-scoped macOS runner.
# LaunchAgent executes as the current non-privileged macOS user after GUI login.
set -euo pipefail

REPO="${RUNNER_REPO:-r14r/BookBuilder-with-typst}"
RUNNER_VERSION="2.338.1"
TYPST_VERSION="0.15.1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
RUNNER_DIR="${RUNNER_DIR:-$(dirname "$SCRIPT_DIR")/.local/bookbuilder-actions-runner}"
RUNNER_NAME="${RUNNER_NAME:-$(hostname -s)-bookbuilder-macos}"
LOCAL_BIN="$HOME/.local/bin"

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
msg() { printf '[BookBuilder macOS] %s\n' "$*"; }

[[ "$(uname -s)" == "Darwin" ]] || fail "This installer is for macOS. Use install-ubuntu.sh on Ubuntu."
[[ "$(id -u)" -ne 0 ]] || fail "Do not run this installer with sudo/root."
case "$(uname -m)" in
  arm64)
    RUNNER_ARCH="arm64"
    RUNNER_SHA256="52c3fb801b931fbd454979bae5ec3443ea3da2c4a1b4c44fa546fb630d61a0a9"
    TYPST_ARCH="aarch64-apple-darwin"
    TYPST_SHA256="48f62ed034aa3a7978309579ac6ca00045e2ef0da73114e8af27cfd8e74dc05a"
    ;;
  x86_64)
    RUNNER_ARCH="x64"
    RUNNER_SHA256="f06e329ffe90a2a4bda8b460a20b9e26dcb7525a093bf9b4f89125c72bc1f39e"
    TYPST_ARCH="x86_64-apple-darwin"
    TYPST_SHA256="7f9fdd9584866245de9a79e0add8f9236fae6f40a8a45e2c4771ccc14db4e0fa"
    ;;
  *) fail "Unsupported Mac CPU: $(uname -m). Use Apple Silicon or Intel x64." ;;
esac

# Migrate before creating the new installation directory.
bash "$SCRIPT_DIR/migrate-macos.sh"

command -v brew >/dev/null || fail "Install Homebrew first: https://brew.sh/"
msg "Installing required Homebrew formulae"
brew install python@3.12 php@8.2 openjdk@21 git gh jq just ffmpeg fontconfig xz

brew_prefix="$(brew --prefix)"
python_prefix="$(brew --prefix python@3.12)"
php_prefix="$(brew --prefix php@8.2)"
java_prefix="$(brew --prefix openjdk@21)"
export JAVA_HOME="$java_prefix/libexec/openjdk.jdk/Contents/Home"
export PATH="$LOCAL_BIN:$python_prefix/libexec/bin:$python_prefix/bin:$php_prefix/bin:$java_prefix/bin:$brew_prefix/bin:$PATH"
mkdir -p "$LOCAL_BIN" "$RUNNER_DIR"

msg "Installing pinned Typst ${TYPST_VERSION} if needed"
if ! command -v typst >/dev/null || [[ ! "$(typst --version)" =~ ^typst[[:space:]]0[.]15[.]1([[:space:]]|$) ]]; then
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  asset="typst-${TYPST_ARCH}.tar.xz"
  curl --fail --location --retry 3 \
    "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/${asset}" \
    --output "$tmp/$asset"
  printf '%s  %s\n' "$TYPST_SHA256" "$tmp/$asset" | shasum -a 256 -c -
  tar -xJf "$tmp/$asset" -C "$tmp"
  install -m 0755 "$tmp/typst-${TYPST_ARCH}/typst" "$LOCAL_BIN/typst"
  rm -rf "$tmp"
  trap - EXIT
fi

msg "Running read-only macOS host doctor"
bash "$(dirname "$0")/doctor-macos.sh"

gh auth status >/dev/null || fail "Authenticate GitHub CLI: gh auth login --scopes repo"
if [[ ! -f "$RUNNER_DIR/config.sh" ]]; then
  msg "Installing GitHub Actions runner ${RUNNER_VERSION} for ${RUNNER_ARCH}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  asset="actions-runner-osx-${RUNNER_ARCH}-${RUNNER_VERSION}.tar.gz"
  curl --fail --location --retry 3 \
    "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/${asset}" \
    --output "$tmp/$asset"
  printf '%s  %s\n' "$RUNNER_SHA256" "$tmp/$asset" | shasum -a 256 -c -
  tar -xzf "$tmp/$asset" -C "$RUNNER_DIR"
  rm -rf "$tmp"
  trap - EXIT
fi

if [[ ! -f "$RUNNER_DIR/.runner" ]]; then
  msg "Fetching one-time registration token for ${REPO}"
  token="$(gh api --method POST "repos/${REPO}/actions/runners/registration-token" --jq .token)"
  [[ -n "$token" ]] || fail "Missing registration token; GitHub repo admin access required"
  (
    cd "$RUNNER_DIR"
    ./config.sh --unattended --url "https://github.com/${REPO}" \
      --token "$token" --name "$RUNNER_NAME" --labels "${RUNNER_LABEL:-bookbuilder}" \
      --work _work
  )
  unset token
else
  msg "Existing runner registration preserved"
fi

# launchd agents do not read interactive shell startup files.
# Runner.Listener reads this .env file at startup. Preserve non-managed keys.
msg "Setting runner PATH and JAVA_HOME for launchd"
envfile="$RUNNER_DIR/.env"
tmpenv="$(mktemp)"
trap 'rm -f "$tmpenv"' EXIT
if [[ -f "$envfile" ]]; then
  grep -v -E '^(PATH|JAVA_HOME)=' "$envfile" > "$tmpenv" || true
fi
printf 'PATH=%s\nJAVA_HOME=%s\n' "$PATH" "$JAVA_HOME" >> "$tmpenv"
cat "$tmpenv" > "$envfile"
chmod 600 "$envfile"
rm -f "$tmpenv"
trap - EXIT

(
  cd "$RUNNER_DIR"
  if [[ ! -f .service ]]; then
    ./svc.sh install
  fi
  ./svc.sh stop || true
  ./svc.sh start
  ./svc.sh status
)
msg "Verify the runner is Online: https://github.com/${REPO}/settings/actions/runners"
msg "macOS launchd service starts when this account is logged into the graphical session."
