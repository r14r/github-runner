#!/usr/bin/env bash
# Read-only host preflight; works independently of any registered runner.
set -euo pipefail
errors=0

ok() { echo "OK: $*"; }
bad() { echo "FAIL: $*" >&2; errors=$((errors + 1)); }
check() { if command -v "$1" >/dev/null 2>&1; then ok "$1"; else bad "$1 missing"; fi; }

[[ "$(uname -s)" == "Linux" && "$(uname -m)" == "x86_64" ]] && ok "Linux x64" || bad "Linux x64 required"
if [[ -r /etc/os-release ]]; then
  # shellcheck source=/dev/null
  source /etc/os-release
  [[ "$ID" == "ubuntu" && "$VERSION_ID" == "24.04" ]] && ok "Ubuntu 24.04" || bad "Ubuntu 24.04 required"
else
  bad "/etc/os-release missing"
fi
for tool in git curl unzip zip jq tar xz just ffmpeg fc-list; do check "$tool"; done
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
  [[ "$(typst --version)" =~ ^typst[[:space:]]0[.]15[.]1([[:space:]]|$) ]] && ok "Typst 0.15.1" || bad "Typst 0.15.1 required"
else
  bad "typst missing"
fi
if (( errors )); then
  echo "Preflight failed: ${errors} missing or incompatible requirements." >&2
  exit 1
fi
echo "All BookBuilder runner host requirements passed."
