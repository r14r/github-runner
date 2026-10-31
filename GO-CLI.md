# github-runner-cli (Go) — migration stage

Binary: `github-runner-cli`. Repository: `r14r/github-runner`.

## Build and install

```sh
just bootstrap
just ci
just build
just install                  # requires permission to write /usr/local/bin
INSTALL_DIR="$HOME/.local/bin" just install  # user-scoped install
```

Installation layout:

```text
<INSTALL>/github-runner-cli
<INSTALL>/env/github-runner-cli/
  runner/
    01-download.sh
    02-configure.sh
    03-setup.sh
    04-run.sh
    lib/
    docker/
  install.sh
  control.sh
  ...
```

The Go binary dispatches the existing host/runner operations to packaged, audited compatibility scripts. This is an **intermediate migration** and is not a full rewrite of the Bash/Python internals into Go. The Docker image's entrypoint must remain a shell script inside the container. No existing Docker volume, runner registration or host service is deleted by the installation itself.

## Running a repository-specific runner

```sh
export RUNNER_BACKEND=docker
github-runner-cli runner docker-doctor
github-runner-cli runner download r14r/BookBuilder
# For private repositories:
github-runner-cli runner register r14r/BookBuilder
# For public repositories, explicitly acknowledge the risk:
github-runner-cli runner register r14r/BookBuilder --allow-public
github-runner-cli runner install-service r14r/BookBuilder
github-runner-cli runner start r14r/BookBuilder
github-runner-cli runner status r14r/BookBuilder
github-runner-cli runner monitor r14r/BookBuilder
```

## Additional commands

```sh
github-runner-cli runner stop r14r/BookBuilder
github-runner-cli runner setup r14r/BookBuilder --workflow .github/workflows/ci.yml --job build
github-runner-cli host doctor
github-runner-cli host status
github-runner-cli host actions-cancel
```

The `host` command family operates the legacy standalone runner, not a repository-scoped Docker runner. Existing host scripts resolve state relative to their installation location; inspect and migrate host-local state before switching existing host-managed installations to the new binary. On container runners, Docker volumes are named per repository and are retained. `RUNNER_DOCKER_SOCKET=1` is a privileged opt-in and is not enabled by default.

## Verification boundaries

The Linux CI covers Go source, command dispatch and existing Python/Bash regression checks. Real macOS launchd, Ubuntu systemd, GitHub registration and Colima container execution must be tested on their respective hosts before considering the migration complete.
