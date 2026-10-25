# Standalone GitHub Actions runner commands. cd github-runner && just <command>
set shell := ["bash", "-euo", "pipefail", "-c"]

default:
    @just --list

# Install tools and register/start the runner (reuses existing registration).
install:
    bash ./install.sh

# Move an existing macOS runner to github-runner/.local (preserve GitHub registration).
migrate:
    bash ./migrate-macos.sh

# Explicitly repair stale/deleted GitHub registration on macOS.
# Safe refusal if an existing runner with this name is online/busy.
repair:
    bash ./repair-macos.sh

# Validate runtime and tool requirements without changing the host.
doctor:
    bash ./doctor.sh

# Start an already registered runner.
start:
    bash ./control.sh start

# Stop runner service; keep registration.
stop:
    bash ./control.sh stop

# Report runner service status.
status:
    bash ./control.sh status

# Refresh service status, GitHub workflow runs and diagnostics every 10 seconds.
# Ctrl+C stops monitoring only. Example: just monitor 30
monitor interval="10":
    bash ./control.sh monitor "{{interval}}"

# Cancel all queued and running workflow runs in the repository (all branches).
actions-cancel:
    bash ./actions.sh cancel

# Delete completed runs, logs and artifacts; requires typing DELETE <repository>.
actions-delete:
    bash ./actions.sh delete

# Confirm once, cancel active runs, wait for completion, then delete the original runs.
# Does not touch new runs created during cleanup.
actions-clear:
    bash ./actions.sh clear

# Remove only _diag/*.log files older than 30 complete days; show disk usage.
# Runner registrations, _work and tool caches are never deleted.
cleanup days="30":
    bash ./cleanup.sh "{{days}}"

# Preview exactly which old runner logs would be removed, without deleting.
cleanup-preview days="30":
    RUNNER_CLEANUP_DRY_RUN=1 bash ./cleanup.sh "{{days}}"

# Explicitly relocate a live macOS runner from its old BookBuilder checkout.
# Only when no workflow job is running. Provide the FULL original runtime path.
migrate-from-bookbuilder source:
    LEGACY_RUNNER_DIR="{{source}}" bash ./migrate-macos.sh

# Script syntax and mocked regression suite (python3 + pytest required).
test:
    @for f in ./*.sh; do bash -n "$f"; done
    python3 -m pytest -q tests

# No host provisioning or GitHub registration needed.
ci: test
