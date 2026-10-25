#!/usr/bin/env bash
# Platform-aware runner installer; run as a normal user (never with sudo).
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
case "$(uname -s)" in
  Darwin) exec bash "$here/install-macos.sh" ;;
  Linux) exec bash "$here/install-ubuntu.sh" ;;
  *) echo "Unsupported runner OS: $(uname -s)" >&2; exit 1 ;;
esac
