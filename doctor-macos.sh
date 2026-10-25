#!/usr/bin/env bash
# Read-only prerequisites check for a macOS BookBuilder runner.
set -euo pipefail
errors=0
ok() { echo "OK: $*"; }
bad() { echo "FAIL: $*" >&2; errors=$((errors + 1)); }
check() { if command -v "$1" >/dev/null 2>&1; then ok "$1"; else bad "$1 missing"; fi; }

[[ "$(uname -s)" == "Darwin" ]] && ok "macOS" || bad "macOS required"
case "$(uname -m)" in
  arm64|x86_64) ok "Architecture $(uname -m)" ;;
  *) bad "Unsupported macOS architecture: $(uname -m)" ;;
esac
# Prefer the pinned Homebrew runtimes when invoked directly from a shell:
# Homebrew php@8.2 is keg-only, and ~/.local/bin stores the pinned Typst.
if command -v brew >/dev/null 2>&1; then
  brew_prefix="$(brew --prefix)"
  python_prefix="$(brew --prefix python@3.12)"
  php_prefix="$(brew --prefix php@8.2)"
  java_prefix="$(brew --prefix openjdk@21)"
  export JAVA_HOME="$java_prefix/libexec/openjdk.jdk/Contents/Home"
  export PATH="$HOME/.local/bin:$python_prefix/libexec/bin:$python_prefix/bin:$php_prefix/bin:$java_prefix/bin:$brew_prefix/bin:$PATH"
fi

for tool in git curl unzip zip jq tar xz just ffmpeg fc-list gh brew; do check "$tool"; done
if command -v python3.12 >/dev/null 2>&1; then
  python3.12 -c 'import venv, ensurepip, ssl; print("OK: Python 3.12 venv, pip and SSL")' || bad "Python 3.12 modules"
else
  bad "python3.12 missing"
fi
if command -v php >/dev/null 2>&1; then
  [[ "$(php -r 'echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;')" == "8.2" ]] && ok "PHP 8.2" || bad "PHP 8.2 required"
  for ext in curl zip mbstring; do
    php -r 'exit(extension_loaded($argv[1]) ? 0 : 1);' "$ext" && ok "PHP extension ${ext}" || bad "PHP extension ${ext} missing"
  done
else
  bad "php missing"
fi
if command -v java >/dev/null 2>&1; then
  java -version 2>&1 | head -n 1 | grep -Eq 'version "21\.|openjdk 21\.' && ok "Java 21" || bad "Java 21 required"
else
  bad "java missing"
fi
if command -v typst >/dev/null 2>&1; then
  typst_version="$(typst --version 2>&1)" || true
  [[ "$typst_version" =~ ^typst[[:space:]]0[.]15[.]1([[:space:]]|$) ]] && ok "Typst 0.15.1 ($typst_version)" || bad "Typst 0.15.1 required; found '$typst_version' at $(command -v typst)"
else
  bad "typst missing"
fi
if (( errors > 0 )); then
  echo "macOS BookBuilder runner preflight: ${errors} missing or incompatible prerequisites." >&2
  exit 1
fi
echo "macOS BookBuilder runner preflight passed."
