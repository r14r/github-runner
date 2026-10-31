# github-runner-cli

Go CLI for managing repository-specific GitHub Actions self-hosted runners on
Docker/Colima. Source of truth: `r14r/github-runner`. Version is read from
root `VERSION` at build time.

## Build

Go >=1.23 and `just` are required. **Python, pytest, and pip are no longer
project development dependencies.**

```sh
just bootstrap
just ci
just build
just install
```

The default binary installation directory is `/usr/local/bin`. To install
without administrator privileges:

```sh
INSTALL_DIR="$HOME/.local/bin" just install
```

No service registration, runner credentials or Docker volumes are touched by
`just install`: it only installs the CLI and runtime assets.

## Install a repository-specific runner

```sh
export RUNNER_BACKEND=docker
docker context use colima
github-runner-cli runner docker-doctor

# Public repositories require explicit acknowledgement:
github-runner-cli install r14r/BookBuilder --allow-public
github-runner-cli start r14r/BookBuilder
github-runner-cli status r14r/BookBuilder
github-runner-cli monitor r14r/BookBuilder
```

The `install` command builds the image, checks the required toolchain,
registers the repository-specific runner only if missing, and creates the
container. It does not start a job or modify target repository workflows.
On `r14r/BookBuilder`, the selected Docker image contains the BookBuilder
requirements (PHP 8.2, Java 21, just, ffmpeg, fontconfig, Typst 0.15.1).

To stop:

```sh
github-runner-cli stop r14r/BookBuilder
```

To upgrade an existing container: verify GitHub reports `busy=false`,
stop it, build the updated image, and explicitly refresh it:

```sh
github-runner-cli stop r14r/BookBuilder
github-runner-cli runner download r14r/BookBuilder
github-runner-cli refresh r14r/BookBuilder
github-runner-cli start r14r/BookBuilder
```

The registration volume `github-runner-r14r-bookbuilder-state` is preserved.
Do not stop a runner while it is executing a job.

## Change workflow routing

```sh
github-runner-cli runner setup r14r/BookBuilder \
  --workflow .github/workflows/ci.yml --job python --allow-public
```

This uses a native Go workflow patcher and opens a PR. It validates all
selected workflows before writing, rejects dynamic `runs-on`, symlinks, and
OS migrations without `--allow-cross-os`. It does not merge automatically.

## Installation files

```text
<INSTALL>/
├── github-runner-cli
└── env/github-runner-cli/
    ├── runner/docker/Dockerfile
    ├── runner/docker/entrypoint.sh
    ├── runner/*.sh          # compatibility only
    └── *.sh                 # legacy macOS/Ubuntu host utilities
```

The Go CLI directly implements Docker provisioning, registration, lifecycle,
monitoring and GitHub workflow patching. The official runner's container
entrypoint remains a shell script. Older macOS launchd/Ubuntu systemd host
installers and recovery tools remain as packaged compatibility assets and
have **not yet** been converted to native Go. Never remove active service or
registration files during migration.

The CI pipeline uses Go tests, `go vet`, `gofmt`, race tests, and a separate
Docker build+BookBuilder doctor. No Python or pytest invocation is required.

### Security

Public GitHub repositories can run untrusted pull-request code. Never give
untrusted workflows the host's Docker socket or credentials.
`RUNNER_DOCKER_SOCKET=1` explicitly grants privileged Docker access.
