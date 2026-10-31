#!/usr/bin/env bash
# Non-destructive Docker/Colima diagnostics before building a runner image.
set -euo pipefail

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || fail 'Docker CLI missing.'
docker info >/dev/null 2>&1 || fail 'Docker daemon unavailable. Start Colima and select its Docker context.'
arch="$(docker info --format '{{.Architecture}}')"
[[ "$arch" == x86_64 || "$arch" == amd64 ]] || fail "Expected x86_64 Docker daemon, found: $arch"
printf 'Docker context: %s\n' "$(docker context show)"
printf 'Docker storage:\n'
docker system df || fail 'Cannot inspect Docker storage.'
if command -v colima >/dev/null 2>&1 && [[ "$(docker context show)" == colima ]]; then
  printf 'Colima VM filesystems:\n'
  colima ssh -- df -h || printf 'WARNING: Colima filesystem details unavailable.\n' >&2
fi
# Inspect the writable container filesystem without changing retained volumes.
free_kb="$(docker run --rm --platform linux/amd64 --entrypoint sh ubuntu:24.04 -c "df -Pk / | awk 'NR==2 {print \$4}'")" \
  || fail 'Cannot inspect container writable storage.'
[[ "$free_kb" =~ ^[0-9]+$ ]] || fail "Unexpected container disk availability: $free_kb"
if (( free_kb < 2097152 )); then
  fail "Docker container filesystem has less than 2 GiB free ($((free_kb / 1024)) MiB). Free BuildKit cache or increase Colima disk size; do not remove volumes."
fi
printf 'Container writable storage free: %s MiB\n' "$((free_kb / 1024))"
printf 'Checking signed Ubuntu 24.04 package indexes...\n'
if ! docker run --rm --platform linux/amd64 ubuntu:24.04 sh -ec 'apt-get update'; then
  cat >&2 <<'MESSAGE'
ERROR: Ubuntu apt-get update fails in an unmodified container.
Inspect 'colima ssh -- df -h' and 'docker system df -v'.
If disk space is sufficient, try 'docker builder prune -f', then rerun.
Also check proxy/network interception and VM time. Never disable APT signature checks.
No GitHub runner volumes or registrations were modified.
MESSAGE
  exit 1
fi
printf 'Docker/Colima preflight passed.\n'
