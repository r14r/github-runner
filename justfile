# GitHub Runner CLI — Go application. Run all recipes from repository root.
set shell := ["bash", "-euo", "pipefail", "-c"]
set working-directory := "."

default:
    @just --list

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

# Fully Go-based unit and integration tests. No Python or pytest needed.
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
      mkdir -p "$destination/env/github-runner-cli/runner/lib" "$destination/env/github-runner-cli/runner/docker"; \
      install -m 0755 dist/github-runner-cli "$destination/github-runner-cli"; \
      for f in ./*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/"; done; \
      for f in runner/*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/runner/"; done; \
      for f in runner/lib/*.sh; do install -m 0644 "$f" "$destination/env/github-runner-cli/runner/lib/"; done; \
      for f in runner/docker/*.sh; do install -m 0755 "$f" "$destination/env/github-runner-cli/runner/docker/"; done; \
      install -m 0644 runner/docker/Dockerfile "$destination/env/github-runner-cli/runner/docker/Dockerfile"; \
      test "$("$destination/github-runner-cli" version)" = "github-runner-cli $(cat VERSION)"

run *args:
    go run ./cmd/github-runner-cli {{args}}

runner-download repo:
    go run ./cmd/github-runner-cli runner download "{{repo}}"

runner-register repo:
    go run ./cmd/github-runner-cli runner register "{{repo}}"

runner-install-service repo:
    go run ./cmd/github-runner-cli runner install-service "{{repo}}"

runner-start repo:
    go run ./cmd/github-runner-cli start "{{repo}}"

runner-stop repo:
    go run ./cmd/github-runner-cli stop "{{repo}}"

runner-status repo:
    go run ./cmd/github-runner-cli status "{{repo}}"

runner-monitor repo:
    go run ./cmd/github-runner-cli monitor "{{repo}}"

runner-docker-doctor:
    go run ./cmd/github-runner-cli runner docker-doctor

runner-pr repo job workflow=".github/workflows/ci.yml":
    go run ./cmd/github-runner-cli runner setup "{{repo}}" --workflow "{{workflow}}" --job "{{job}}"

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
