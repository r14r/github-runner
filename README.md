# github-runner-cli

Go-based runner manager for **arbitrary GitHub repositories**. It provides
repository-scoped, Docker/Colima-backed GitHub Actions runners without
application-specific profiles, hardcoded repository names or workflow
dependencies on the manager repository.

Current version: **0.5.1**.

## Install

```sh
just bootstrap
just ci
just build
just install
# Or: INSTALL_DIR="$HOME/.local/bin" just install
ghr help
```

`just install` installs the Go binary and Docker runtime assets only. It
does **not** remove runner containers or registrations.

## Analyze and prepare any repository

```sh
ghr requirements --repo OWNER/REPO
ghr requirements --repo OWNER/REPO --output toolchain.yaml
ghr build --repo OWNER/REPO
ghr doctor --repo OWNER/REPO
```

The analyzer performs a temporary sparse checkout of
`.github/workflows/*.yml|*.yaml`, identifies recognized `uses:` language
setup actions and referenced command-line tools, then reports provenance,
versions and unresolved requirements. It never executes target workflow code.

The generic builder creates a **repository-isolated image tag** using a
trusted Ubuntu 24.04 Linux x64 base. A small, fixed allowlist supports
`ffmpeg`, `fontconfig`, Java 21, the Ubuntu-packaged PHP CLI, and pinned
releases of Just 1.58.0 and Typst 0.15.1. The base already provides Python
3.12 and Docker CLI. Tools provided by recognized workflow setup actions
(e.g. `actions/setup-java`, `actions/setup-python`,
`ruby/setup-ruby`) are reported as **WORKFLOW ACTION** and are not
misrepresented as preinstalled in the runner image. Unsupported or dynamic
requirements fail closed with a clear diagnostic; use the relevant setup
action or a custom image rather than running arbitrary shell commands.

`ghr doctor` prints readable numbered stages: repository and revision,
workflow requirement counts, selected image, and one line per tool with
`OK`, `WORKFLOW ACTION` or `FAIL`. Git progress is hidden on success but
retained in failure details.

## Register and operate

```sh
export RUNNER_BACKEND=docker
ghr runner docker-doctor

# Register only once; public repositories require explicit opt-in:
ghr runner register --repo OWNER/REPO [--allow-public]
ghr start --repo OWNER/REPO
ghr status --repo OWNER/REPO
ghr monitor --repo OWNER/REPO
ghr stop --repo OWNER/REPO
```

Alternatively, `ghr install --repo OWNER/REPO [--allow-public]` builds,
registers and creates a stopped container in one operation.

```sh
ghr list-installed
ghr list-installed --repo OWNER/REPO
ghr uninstall --repo OWNER/REPO --dry-run
ghr uninstall --repo OWNER/REPO [--delete-data]
ghr uninstall --all --dry-run
ghr uninstall --all --yes [--delete-data]
ghr self-uninstall --yes
```

Each registration uses dedicated Docker container/volume names derived from
the supplied `OWNER/REPO` slug. A persistent volume survives normal image
rebuilds and container restarts. Uninstall checks exact registration identity
and refuses busy runners. `--delete-data` is irreversible; images are not
pruned automatically.

## Explicit configuration

- `--repo` or `--repository OWNER/REPO`: required for repository-scoped
  operations; no repository is selected implicitly.
- `RUNNER_PROFILE=auto|generic|toolchain|runner-base`: defaults to
  `toolchain`, or opt into the minimal `runner-base`.
- `RUNNER_IMAGE`: optional explicit image tag; useful when migrating an
  existing registration, but keep image compatibility in mind.
- `RUNNER_DOCKER_SOCKET=1`: **privileged** Docker daemon mount; disabled by
  default.
- `RUNNER_BACKEND=docker`: the supported backend on Linux/macOS with Docker
  (including Colima).

For a runner installed with an older application-specific image, the
registration volume is **not** deleted by this refactor. Its new default
image tag differs; rebuild and explicitly refresh a **stopped, idle** runner
before restarting, or set `RUNNER_IMAGE` to the compatible existing image.

`ghr refresh --repo OWNER/REPO` automatically stops, replaces and restarts a **running, idle** runner, preserving its GitHub registration and Docker volume. If the container was already stopped, it remains stopped after refresh. A busy runner, a GitHub API error or an unverifiable runner identity causes refresh to abort without stopping anything. A Docker socket mount requires explicit `RUNNER_DOCKER_SOCKET=1` before any stop. On failure, the CLI reports the recovery action; it never deletes registration data.

## Workflow routing

Workflow routing is an application-repository choice. The Go command
`ghr runner setup --repo OWNER/REPO` opens a reviewable PR for explicit
job-level labels and never edits another repository silently.

For repositories that want hosted by default and self-hosted by explicit
selection, place the following expression **in their own workflows**:

```yaml
runs-on: ${{ fromJSON(vars.RUNNER_LABELS_JSON || '"ubuntu-24.04"') }}
```

This repository does not contain application workflow templates or fetch
application-owned runner diagnostics.

## Development

```sh
just update           # git pull --ff-only, bootstrap, CI, build
just fmt              # explicit formatting change
just ci               # Go tests and vet; does not format files
just build
just install
```

`just ci` is read-only with respect to Go source formatting. GitHub Actions
runs the Go test suite, a CLI installation smoke test and a generic Docker
image test on Ubuntu 24.04.

**Legacy native host installers are retired in v0.5.0.** The repository now
supports repository-agnostic Docker runners only; it does not automatically
stop or migrate any previously installed host launchd/systemd service.
