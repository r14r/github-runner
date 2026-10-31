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

The five top-level commands are the recommended interface. Use the Docker
backend on macOS Monterey/Colima. `install` downloads/builds the runner,
registers it when not already registered, and creates its service container;
`start` is intentionally separate. Existing volumes and registrations are
retained on repeated installation.

```sh
export RUNNER_BACKEND=docker

# New private repository:
github-runner-cli install r14r/example-repo

# New public repository (explicit security acknowledgement):
github-runner-cli install r14r/BookBuilder --allow-public

github-runner-cli start r14r/BookBuilder
github-runner-cli status r14r/BookBuilder
github-runner-cli monitor r14r/BookBuilder   # Ctrl+C ends monitoring
github-runner-cli stop r14r/BookBuilder
```

The underlying explicit, stepwise commands are still supported:

```sh
github-runner-cli runner docker-doctor
github-runner-cli runner download r14r/BookBuilder
github-runner-cli runner register r14r/BookBuilder --allow-public
github-runner-cli runner install-service r14r/BookBuilder
github-runner-cli runner start r14r/BookBuilder
```

**Safety:** Public repositories require `--allow-public`; do not schedule
untrusted pull request workflows on a privileged self-hosted runner.
`RUNNER_DOCKER_SOCKET=1` grants powerful Docker host access and remains opt-in.

## BookBuilder Docker toolchain

The `r14r/BookBuilder` repository automatically selects the `bookbuilder`
Docker image profile (`github-actions-runner-local:2.338.1-bookbuilder`).
Other repositories retain the generic `2.338.1` image by default.
Set `RUNNER_PROFILE=bookbuilder` explicitly to use this toolchain for
another repository.

The BookBuilder profile includes ffmpeg, fontconfig/fc-list, just, Java 21,
PHP 8.2 with curl/zip/mbstring extensions, and Typst 0.15.1. The standalone
just and typst binaries are checked against pinned official GitHub release
SHA-256 checksums; PHP 8.2 comes from the signed Ubuntu ondrej/php PPA.
Image build fails if the requirements are not met. After building, the
download command runs the repository's `doctor-linux.sh` in the new image.

### Update an existing BookBuilder container

**Do not stop a busy runner.** Verify the runner is idle first, because
stopping a running job interrupts that job. Changes to the image do not affect
existing containers until they are explicitly replaced.

```sh
export RUNNER_BACKEND=docker
github-runner-cli status r14r/BookBuilder
# When status shows busy=false and no jobs are running:
github-runner-cli stop r14r/BookBuilder
github-runner-cli runner download r14r/BookBuilder
github-runner-cli refresh r14r/BookBuilder
github-runner-cli start r14r/BookBuilder
github-runner-cli status r14r/BookBuilder
docker logs -f github-runner-r14r-bookbuilder
```

The `refresh` operation refuses to replace a running container and
removes **only** the stopped container, never its named registration volume.
If the existing container has a Docker socket mount, explicitly pass
`RUNNER_DOCKER_SOCKET=1` when refreshing to preserve that privileged access;
do this only if that trust boundary is acceptable. No workflow files in
BookBuilder are changed by this process.

To verify the tools manually from the source checkout:

```sh
docker run --rm -i --platform linux/amd64 --entrypoint bash \
  github-actions-runner-local:2.338.1-bookbuilder -s < doctor-linux.sh
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
