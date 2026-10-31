# github-runner-cli

Go CLI to manage self-hosted GitHub Actions runners on Docker/Colima.
Version: **0.3.1**. The repository is `r14r/github-runner`.

## Build and install

```sh
just bootstrap
just ci
just build
just install

# Optional unprivileged install:
INSTALL_DIR="$HOME/.local/bin" just install
```

`just install` installs the binary and runtime assets only; it does not
register or stop any runner.

## Setup, start and monitor one repository

```sh
export RUNNER_BACKEND=docker
docker context use colima
github-runner-cli runner docker-doctor
github-runner-cli install --repo r14r/BookBuilder --allow-public
github-runner-cli start --repo r14r/BookBuilder
github-runner-cli status --repo r14r/BookBuilder
github-runner-cli monitor --repository r14r/BookBuilder
github-runner-cli stop --repo r14r/BookBuilder
```

`--repo` and `--repository` are interchangeable. Repository flags can
appear before or after other options, including `--repo=OWNER/REPO`.
A positional `OWNER/REPO` is **no longer accepted**.

## Show installed runners

```sh
# All runners installed in the current Docker/Colima context:
github-runner-cli list-installed

# Only one repo: show local runner plus all its GitHub runner registrations:
github-runner-cli list-installed --repo r14r/BookBuilder
github-runner-cli list-installed --repository r14r/github-runner-test
```

Without a repository flag, `list-installed` enumerates **local Docker
containers and volumes** managed by this CLI (not every GitHub runner across
all organizations). With `--repo` it additionally queries the repository
GitHub API, including runners hosted on other machines.

## Uninstall runner(s)

```sh
# Unregister the matching GitHub runner and remove its Docker container.
# Preserves local volume, registration/workspace files and other images.
github-runner-cli uninstall --repo r14r/BookBuilder --dry-run
# Once reviewed, perform the actual removal:
github-runner-cli uninstall --repo r14r/BookBuilder

# Permanently remove its registration/workspace Docker volume too:
github-runner-cli uninstall --repo r14r/BookBuilder --delete-data

# Remove all locally managed runners; explicit confirmation required:
github-runner-cli uninstall --all --yes
github-runner-cli uninstall --all --yes --delete-data
```

Uninstallation checks the exact local GitHub identity and the remote
`busy` flag before modifying runner resources. It **refuses** to interrupt
busy or unidentifiable registrations. Bulk removal preflights all managed
runners before any change; it never deletes unrelated Docker volumes or
images. Without `--delete-data`, the local registration files remain,
even though the GitHub registration has been deleted. To reinstall such
a runner, delete its stale data explicitly first.

```sh
# Remove only the CLI application and <INSTALL>/env/github-runner-cli:
github-runner-cli self-uninstall --yes
```

`self-uninstall` never stops or deregisters runners and never removes
containers or Docker volumes. It refuses to run from a checkout or `go run`.

## Refresh a runner image

Check `busy=false` first, then:

```sh
github-runner-cli stop --repo r14r/BookBuilder
github-runner-cli runner download --repo r14r/BookBuilder
github-runner-cli refresh --repo r14r/BookBuilder
github-runner-cli start --repo r14r/BookBuilder
```

The BookBuilder-specific image contains PHP 8.2, Java 21, just, ffmpeg,
fontconfig and Typst 0.15.1.

## Setup workflow routing

```sh
github-runner-cli runner setup --repo r14r/BookBuilder \
  --workflow .github/workflows/ci.yml --job python --allow-public
```

This validates the target workflow and opens a PR without automatically
merging. Dynamic `runs-on` and cross-OS migrations are refused unless
explicitly reviewed and enabled.

## Installed files and compatibility

```text
<INSTALL>/
├── github-runner-cli
└── env/github-runner-cli/
    ├── runner/docker/Dockerfile
    ├── runner/docker/entrypoint.sh
    ├── runner/*.sh      # compatibility for older native host installations
    └── scripts/*.sh     # legacy macOS/Ubuntu service-management scripts
```

Docker runner management and workflow patching are written in Go.
Older macOS launchd and Ubuntu systemd scripts are retained only as
compatibility assets and are not affected by Docker uninstall operations.
The CLI development/CI workflow has no Python or pytest dependency.

### Security

Never run untrusted public pull-request code on runners with host privileges.
`RUNNER_DOCKER_SOCKET=1` provides privileged Docker daemon access and
remains an explicit opt-in.
