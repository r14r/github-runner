# Go CLI migration status

Version: `0.3.0`

## Native Go components
- `cmd/github-runner-cli/main.go`: CLI and command parsing.
- `cmd/github-runner-cli/docker.go`: Docker/Colima diagnostic, image selection,
  downloads, repository registration, services, status, monitoring, and refresh.
- `cmd/github-runner-cli/setup.go`: GitHub workflow PR workflow using gh/git.
- `internal/workflow/patch.go`: conservative, validated workflow rewriting.
- All active project tests are Go tests under `*_test.go`.

## Installation and CI
```sh
just bootstrap
just ci
just build
just install
```

Installation copies `github-runner-cli` to `<INSTALL>/github-runner-cli`
and any required shell runtime compatibility files to
`<INSTALL>/env/github-runner-cli`.

## Temporary compatibility boundary

The official GitHub runner is a separate upstream executable with its own
entrypoint and `config.sh`/`run.sh`/`svc.sh` scripts. The self-hosted
container uses a minimal `entrypoint.sh`. The previous standalone host
services (macOS launchd / Ubuntu systemd) still use vetted legacy shell
scripts; they are only invoked under `github-runner-cli host ...`. For
repository-specific Docker runners, lifecycle and workflow setup are native Go.

Do not remove existing runner registrations or volumes to migrate them.
The Go CI no longer depends on Python/pytest. Existing host systemd/launchd
scripts require dedicated real-host validation before replacement.
