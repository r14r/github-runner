# Standalone GitHub Actions runner manager.
set shell := ["bash", "-euo", "pipefail", "-c"]
set working-directory := "."

default:
    @just --list

# Create a local test environment only when missing; never register a runner.
bootstrap:
    @command -v python3 >/dev/null || { echo "python3 required" >&2; exit 1; }
    @if [ ! -x .venv/bin/python ]; then python3 -m venv .venv; fi
    @if ! .venv/bin/python -m pytest --version >/dev/null 2>&1; then .venv/bin/python -m pip install pytest; fi

# Non-destructive validation of repository configuration.
setup:
    @grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' VERSION
    @test -f SOURCE.md && test -f update-cli.yaml
    @for f in ./*.sh; do bash -n "$f"; done

fmt-check:
    @for f in ./*.sh; do bash -n "$f"; done

lint: fmt-check
precommit: test

test: bootstrap fmt-check
    @test -x .venv/bin/python || { echo "Run just bootstrap first" >&2; exit 1; }
    .venv/bin/python -m pytest -q tests

# The archive contains only explicitly listed source files, not local credentials.
build: test runner-ci setup
    @version="$(cat VERSION)"; mkdir -p dist; tar -czf "dist/github-runner-${version}.tar.gz" --exclude="__pycache__" --exclude="*.pyc" ./*.sh VERSION README.md SOURCE.md MACOS.md UBUNTU.md AGENTS.md CHANGELOG.md RUNNER-EXTENSION.md runner
    @just verify-build

verify-build:
    @version="$(cat VERSION)"; archive="dist/github-runner-${version}.tar.gz"; test -s "$archive" && tar -tzf "$archive" >/dev/null

ci: build

# Provisioning is opt-in and must never be triggered by bootstrap or CI.
install:
    bash ./install.sh

migrate:
    bash ./migrate-macos.sh

repair:
    bash ./repair-macos.sh

doctor:
    bash ./doctor.sh

run:
    bash ./control.sh start

start: run

stop:
    bash ./control.sh stop

status:
    bash ./control.sh status

monitor interval="10":
    bash ./control.sh monitor "{{interval}}"

actions-cancel:
    bash ./actions.sh cancel

actions-delete:
    bash ./actions.sh delete

actions-clear:
    bash ./actions.sh clear

cleanup days="30":
    bash ./cleanup.sh "{{days}}"

cleanup-preview days="30":
    RUNNER_CLEANUP_DRY_RUN=1 bash ./cleanup.sh "{{days}}"

migrate-from-bookbuilder source:
    LEGACY_RUNNER_DIR="{{source}}" bash ./migrate-macos.sh

clean:
    rm -rf dist

# Repository-scoped Docker/Colima runners.
runner-docker-doctor:
    bash runner/docker/doctor.sh

runner-download repo:
    bash runner/01-download.sh "{{repo}}"

runner-register repo:
    bash runner/02-configure.sh "{{repo}}"

runner-install-service repo:
    bash runner/04-run.sh "{{repo}}" install

runner-start repo:
    bash runner/04-run.sh "{{repo}}" start

runner-stop repo:
    bash runner/04-run.sh "{{repo}}" stop

runner-status repo:
    bash runner/04-run.sh "{{repo}}" status

runner-monitor repo:
    bash runner/04-run.sh "{{repo}}" monitor

runner-pr repo job workflow=".github/workflows/ci.yml":
    bash runner/03-setup.sh "{{repo}}" --workflow "{{workflow}}" --job "{{job}}"

runner-ci:
    @for f in runner/*.sh runner/lib/*.sh runner/docker/*.sh; do bash -n "$f"; done
    python3 -m unittest discover -s tests -p test_runner_extension.py
