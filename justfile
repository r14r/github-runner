# Build and install github-runner-cli (Go). Root-relative recipes.
set shell := ["bash", "-euo", "pipefail", "-c"]
set working-directory := "."

default:
    @just --list

# Check installed toolchain without provisioning a runner.
bootstrap:
    @command -v go >/dev/null || { echo "Go >= 1.23 required" >&2; exit 1; }
    @command -v bash >/dev/null || { echo "bash required" >&2; exit 1; }
    @go version
    go mod download

# Validate Go code and preserve existing Bash/Python regression tests.
ci: bootstrap
    @test -z "$(gofmt -l ./cmd)" || { gofmt -l ./cmd; exit 1; }
    go vet ./...
    go test ./...
    @for f in ./*.sh runner/*.sh runner/lib/*.sh runner/docker/*.sh; do bash -n "$f"; done
    python3 -m unittest discover -s tests -p test_runner_extension.py
    @if python3 -m pytest --version >/dev/null 2>&1; then python3 -m pytest -q tests; else echo "pytest required for full CI" >&2; exit 1; fi

# Portable application binary; scripts are installed separately as runtime assets.
build: ci
    mkdir -p dist
    go build -trimpath -o dist/github-runner-cli ./cmd/github-runner-cli
    ./dist/github-runner-cli version

# Install binary and runtime scripts; INSTALL_DIR defaults to /usr/local/bin.
# Example: INSTALL_DIR="$HOME/.local/bin" just install
install: build
    @destination="${INSTALL_DIR:-/usr/local/bin}"; \
      mkdir -p "$destination/env/github-runner-cli"; \
      install -m 0755 dist/github-runner-cli "$destination/github-runner-cli"; \
      for f in ./*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/"; done; \
      mkdir -p "$destination/env/github-runner-cli/runner/lib" "$destination/env/github-runner-cli/runner/docker"; \
      for f in runner/*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/runner/"; done; \
      for f in runner/lib/*.sh runner/lib/*.py; do install -m 0644 "$f" "$destination/env/github-runner-cli/runner/lib/"; done; \
      for f in runner/docker/*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/runner/docker/"; done; \
      install -m 0644 runner/docker/Dockerfile "$destination/env/github-runner-cli/runner/docker/Dockerfile"; \
      "$destination/github-runner-cli" version

clean:
    rm -rf dist

run *args:
    go run ./cmd/github-runner-cli {{args}}

# Compatibility recipes forward to the Go CLI.
runner-download repo:
    go run ./cmd/github-runner-cli runner download "{{repo}}"

runner-register repo:
    go run ./cmd/github-runner-cli runner register "{{repo}}"

runner-install-service repo:
    go run ./cmd/github-runner-cli runner install-service "{{repo}}"

runner-start repo:
    go run ./cmd/github-runner-cli runner start "{{repo}}"

runner-stop repo:
    go run ./cmd/github-runner-cli runner stop "{{repo}}"

runner-status repo:
    go run ./cmd/github-runner-cli runner status "{{repo}}"

runner-monitor repo:
    go run ./cmd/github-runner-cli runner monitor "{{repo}}"

runner-docker-doctor:
    go run ./cmd/github-runner-cli runner docker-doctor

runner-pr repo job workflow=".github/workflows/ci.yml":
    go run ./cmd/github-runner-cli runner setup "{{repo}}" --workflow "{{workflow}}" --job "{{job}}"
