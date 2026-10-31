# Multi-repository runner extension for `r14r/github-runner`

This directory extends the canonical runner manager; it does not replace any
BookBuilder installer, repair, monitoring, cleanup or safety code at the root.
Runtime state stays in the root `.local/` directory (not in Git).

## Target platform

Your macOS Monterey 12.7.6 Intel machine uses `RUNNER_BACKEND=auto` -> Linux
x64 runner in Docker/Colima. Native macOS requires 13.5+ and x64. The existing
root-level `install-macos.sh` is retained but cannot make Monterey supported.
The extension uses official runner **2.338.1** with SHA-256 verified release
assets (both Linux x64 and macOS x64).

## One repository

```bash
cd github-runner
colima status && docker context use colima
gh auth status
just runner-download r14r/website-builder
just runner-register r14r/website-builder
just runner-install-service r14r/website-builder
just runner-start r14r/website-builder
just runner-status r14r/website-builder

# Creates a PR, but never auto-merges it by default:
just runner-pr r14r/website-builder discover-layouts
```

Or invoke the scripts directly, with `--job` repeatable:

```bash
bash runner/03-setup.sh r14r/website-builder \
  --workflow .github/workflows/ci.yml --job discover-layouts
```

## Second repository

```bash
just runner-download r14r/Boersenfuehrerschein-VideoAnalyzer
just runner-register r14r/Boersenfuehrerschein-VideoAnalyzer
just runner-install-service r14r/Boersenfuehrerschein-VideoAnalyzer
just runner-start r14r/Boersenfuehrerschein-VideoAnalyzer
just runner-status r14r/Boersenfuehrerschein-VideoAnalyzer
just runner-pr r14r/Boersenfuehrerschein-VideoAnalyzer ci-gate
```

Before moving Docker build jobs to this runner, you must consciously enable
Docker daemon access by setting `RUNNER_DOCKER_SOCKET=1` when creating the
runner container (`runner-install-service` or `runner-start`). This is
**high-risk**: any workflow with control of the socket can control Colima
containers and volumes. The default is **no** Docker socket access.

The runner has a persistent workspace and is not an isolated clean execution
environment. Only run trusted code. Public repositories are blocked by default.
The new package does not silently delete/re-register existing runners.

## `just` commands

- `runner-download`, `runner-register`, `runner-install-service`
- `runner-start`, `runner-stop`, `runner-status`, `runner-monitor`
- `runner-pr REPO JOB`, `runner-ci`

For `runner-pr`, the default branch is read from GitHub, the repo is cloned
into `.local/checkouts/`, and only the selected scalar `runs-on` is changed.
Open a PR, check it out locally, and leave it open. Direct script option
`--merge` enables a checked, squash-and-delete merge as an explicit opt-in.
Dynamic/matrix/multiline `runs-on` is rejected and must be edited manually.

**Migration limits:** Docker-container jobs and some Docker Compose bind
mounts can fail with Docker-outside-of-Docker due to mismatched host/container
paths. `setup-node`, Playwright dependency installation, PHP/Java/Typst and
Docker builds require dedicated preflight and project profile packages.
A passing runner registration alone does not prove that application CI passes.

## Versioning

This is a proposed **0.1.2** extension of the canonical repository currently
at 0.1.1. Existing BookBuilder runner registrations, `.local/` contents,
launchd/systemd services and established recipes remain unchanged.
