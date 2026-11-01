# Go CLI v0.4.0

Main entry point: `cmd/github-runner-cli/main.go`.
Repository-specific operations: `cmd/github-runner-cli/docker.go`.
Workflow patcher: `internal/workflow/patch.go`.

## Built-in command help

```sh
ghr help
ghr help status
ghr status --help
ghr help runner setup
ghr runner setup --help
ghr help host doctor
```

Each help page documents behavior, syntax, parameters, accepted values,
side-effect/safety notes and complete examples. Help is resolved before
repository validation and before checking runtime assets.

## Workflow requirements and image verification

```sh
github-runner-cli requirements --repo r14r/BookBuilder
github-runner-cli requirements --repo r14r/BookBuilder --output toolchain.yaml
github-runner-cli build --repo r14r/BookBuilder
github-runner-cli doctor --repo r14r/BookBuilder
# First registration only:
github-runner-cli runner register --repo r14r/BookBuilder --allow-public
github-runner-cli start --repo r14r/BookBuilder
```

`requirements` parses GitHub Actions workflow files without executing code,
and reports recognized tools, version pins and provenance in YAML.
It also records explicitly pinned requirements from the selected runner
image profile; unresolved dependencies are reported.
`build` validates profile support, builds and tests the runner image.
Unsupported/dynamic dependencies fail closed rather than being installed
arbitrarily. `doctor` checks supported binaries and pinned versions in
the image without touching runner registration. `start` requires an
existing registration.

`just ci` and GitHub Actions check formatting without rewriting sources.
Use `just fmt` to opt in to formatting changes.

## Required CLI flags

For all repository-scoped commands use `--repo OWNER/REPO` or the equivalent
`--repository OWNER/REPO`, **not** a positional repository argument.

```sh
github-runner-cli install --repo r14r/BookBuilder --allow-public
github-runner-cli start --repo r14r/BookBuilder
github-runner-cli status --repository r14r/BookBuilder
github-runner-cli list-installed
github-runner-cli list-installed --repo r14r/BookBuilder
github-runner-cli uninstall --repo r14r/BookBuilder --dry-run
github-runner-cli uninstall --repo r14r/BookBuilder
github-runner-cli uninstall --all --yes --delete-data
github-runner-cli self-uninstall --yes
```

## Safety invariants

- `list-installed` without repo lists the current Docker daemon's installed
  runner containers/volumes, **not** every registered runner on GitHub.
- `list-installed --repo` also shows registrations from the GitHub API.
- Uninstallation correlates remote runner ID/name with local registration;
  detects `busy` and refuses to interrupt jobs.
- No bulk deletion without `--all --yes`.
- Docker volumes are preserved unless `--delete-data` is supplied.
- No Docker image pruning or deletion as part of runner uninstall.
- `self-uninstall` only removes the installed CLI binary and its adjacent
  `env/github-runner-cli` directory; running runners remain untouched.
- Legacy macOS/Ubuntu host-managed service scripts are not uninstalled
  via Docker runner commands; do not confuse them with Docker containers.

## Build and validation

```sh
just bootstrap
just ci
just build
just install
```

Only Go tooling is required for the project CI; installation includes the
Docker entrypoint script and legacy host scripts as runtime assets. See
`README.md` for a complete command reference.
