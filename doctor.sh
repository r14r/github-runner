#!/usr/bin/env bash
# Platform-aware read-only host prerequisite check for self-hosted workflows.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
case "$(uname -s)" in
  Linux) exec bash "$root/doctor-linux.sh" ;;
  Darwin) exec bash "$root/doctor-macos.sh" ;;
  *) echo "Unsupported self-hosted runner OS: $(uname -s)" >&2; exit 1 ;;
esac
