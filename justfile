# GitHub Runner CLI — Go application. Run all recipes from repository root.
set shell := ["bash", "-euo", "pipefail", "-c"]
set working-directory := "."

default:
    @just --list

# Fast-forward repository updates, run required checks, and rebuild the CLI.
update:
    git pull --ff-only
    just bootstrap
    just ci
    just build

# Prepare Go modules without registering a runner or touching host state.
bootstrap:
    @command -v go >/dev/null 2>&1 || { echo "Go toolchain required" >&2; exit 1; }
    @go version
    go mod download

fmt:
    gofmt -w ./cmd ./internal

fmt-check:
    @files="$(gofmt -l ./cmd ./internal)"; test -z "$files" || { printf 'Unformatted Go files:\n%s\n' "$files" >&2; exit 1; }

lint: fmt-check
    go vet ./...

test:
    go test -count=1 ./...

# Go-only unit and integration tests.
ci: bootstrap fmt lint test
    go test -race ./...

build: ci
    mkdir -p dist
    go build -trimpath -ldflags "-X main.version=$(cat VERSION)" -o dist/github-runner-cli ./cmd/github-runner-cli
    ./dist/github-runner-cli version

# Binary + only the remaining host/container compatibility assets.
# INSTALL_DIR defaults to /usr/local/bin. Never touch runner registrations.
install: build
    @destination="${INSTALL_DIR:-/usr/local/bin}"; \
      mkdir -p "$destination/env/github-runner-cli/scripts" "$destination/env/github-runner-cli/runner/lib" "$destination/env/github-runner-cli/runner/docker"; \
      install -m 0755 dist/github-runner-cli "$destination/github-runner-cli"; \
      for f in scripts/*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/scripts/"; done; \
      for f in runner/*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/runner/"; done; \
      for f in runner/lib/*.sh; do install -m 0644 "$f" "$destination/env/github-runner-cli/runner/lib/"; done; \
      for f in runner/docker/*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/runner/docker/"; done; \
      install -m 0644 runner/docker/Dockerfile "$destination/env/github-runner-cli/runner/docker/Dockerfile"; \
      test "$("$destination/github-runner-cli" version)" = "github-runner-cli $(cat VERSION)"

run *args:
    go run ./cmd/github-runner-cli {{args}}

runner-download repository:
    go run ./cmd/github-runner-cli runner download --repo "{{repository}}"

runner-register repository:
    go run ./cmd/github-runner-cli runner register --repo "{{repository}}"

runner-install-service repository:
    go run ./cmd/github-runner-cli runner install-service --repo "{{repository}}"

runner-start repository:
    go run ./cmd/github-runner-cli start --repo "{{repository}}"

runner-stop repository:
    go run ./cmd/github-runner-cli stop --repo "{{repository}}"

runner-status repository:
    go run ./cmd/github-runner-cli status --repo "{{repository}}"

runner-monitor repository:
    go run ./cmd/github-runner-cli monitor --repo "{{repository}}"

runner-docker-doctor:
    go run ./cmd/github-runner-cli runner docker-doctor

runner-pr repository job workflow=".github/workflows/ci.yml":
    go run ./cmd/github-runner-cli runner setup --repo "{{repository}}" --workflow "{{workflow}}" --job "{{job}}"

list-installed *args:
    go run ./cmd/github-runner-cli list-installed {{args}}

# Uninstall is intentionally not a just recipe: use installed binary with flags.
# Legacy host scripts remain available only for existing registrations.
doctor:
    go run ./cmd/github-runner-cli host doctor

migrate:
    go run ./cmd/github-runner-cli host migrate

repair:
    go run ./cmd/github-runner-cli host repair

start:
    @echo 'Use: github-runner-cli start OWNER/REPO' >&2
    @exit 2

stop:
    @echo 'Use: github-runner-cli stop OWNER/REPO' >&2
    @exit 2

status:
    @echo 'Use: github-runner-cli status OWNER/REPO' >&2
    @exit 2

monitor:
    @echo 'Use: github-runner-cli monitor OWNER/REPO' >&2
    @exit 2

clean:
    rm -rf dist
