# github-runner-cli

Go CLI to manage self-hosted GitHub Actions runners on Docker/Colima.
Version: **0.4.0**. The repository is `r14r/github-runner`.

## Command help

Every command provides detailed help without contacting Docker or GitHub and
without requiring any repository flags:

```sh
ghr help                        # Overview of all commands
ghr help status                 # Purpose, parameters, values, examples
ghr status --help               # Identical result; works without --repo
ghr install -h
ghr help uninstall
ghr list-installed --help
ghr runner --help               # Advanced runner subcommands
ghr runner setup --help
ghr help host monitor
```

`ghr` is an optional shell alias for `github-runner-cli`. All examples
also work with the full executable name. Help is available even if CLI
runtime scripts have not been installed.

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

## Workflow requirements and Docker toolchain

Analyze all workflow files without executing project code:

```sh
github-runner-cli requirements --repo r14r/BookBuilder
github-runner-cli requirements --repo r14r/BookBuilder --output toolchain.yaml
github-runner-cli build --repo r14r/BookBuilder
github-runner-cli doctor --repo r14r/BookBuilder
```

The YAML manifest lists the repository revision, workflow files, `uses:`
actions, detected tool versions, selected runner image profile, and warnings.
Tools pinned by the BookBuilder runner profile are listed with that provenance.
This static Go analyzer does **not** execute workflow code or infer every
possible shell dependency. Unknown/dynamic requirements are identified rather
than silently guessed.

`build` verifies detected requirements against an existing known image
profile (`runner-base` or `bookbuilder`) and builds/verifies the image;
this is not an arbitrary tool/package installer. Official setup actions
such as `ruby/setup-ruby` can install runtimes when a job executes.

**On a new runner, registration is still required before starting:**

```sh
github-runner-cli runner register --repo r14r/BookBuilder --allow-public
github-runner-cli start --repo r14r/BookBuilder
```

For an already registered runner, `start` suffices. `requirements`,
`build` and `doctor` do not register or start a runner.

## CI and formatting

`just ci` now calls `fmt-check`, not `fmt`, and does not modify tracked
Go source files. Run `just fmt` explicitly when formatting is desired.

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
