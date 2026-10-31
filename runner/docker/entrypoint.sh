#!/usr/bin/env bash
set -euo pipefail

# The named Docker volume persists runner registration and workspaces across restarts.
if [[ ! -x /runner/config.sh ]]; then
  cp -a /opt/actions-runner/. /runner/
  chown -R runner:runner /runner
fi

# Docker socket GID differs between Docker engines; match it dynamically.
if [[ -S /var/run/docker.sock ]]; then
  docker_gid="$(stat -c '%g' /var/run/docker.sock)"
  group_name="$(getent group "$docker_gid" | cut -d: -f1 || true)"
  if [[ -z "$group_name" ]]; then
    group_name=hostdocker
    groupadd --gid "$docker_gid" "$group_name"
  fi
  usermod -aG "$group_name" runner
fi
cd /runner

case "${1:-run}" in
  configure)
    [[ ! -e .runner ]] || { echo 'Runner is already registered.' >&2; exit 1; }
    # Registration token is supplied on stdin; never put it in Docker environment or volume.
    IFS= read -r token || [[ -n "${token:-}" ]]
    [[ -n "${token:-}" ]] || { echo 'No registration token provided.' >&2; exit 1; }
    exec gosu runner ./config.sh --unattended \
      --url "https://github.com/${RUNNER_REPOSITORY:?}" \
      --token "$token" \
      --name "${RUNNER_NAME:?}" \
      --labels "${RUNNER_LABEL:?}" \
      --work _work
    ;;
  check)
    [[ -e .runner ]]
    ;;
  identity)
    [[ -f .runner ]] || exit 1
    jq -r .agentName .runner
    ;;
  run)
    [[ -f .runner ]] || { echo 'Runner is not configured. Run 02-configure.sh.' >&2; exit 1; }
    exec gosu runner ./run.sh
    ;;
  *) echo "Unsupported entrypoint command: $1" >&2; exit 2 ;;
esac
