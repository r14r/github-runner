# github-runner-cli

Go-based runner manager for **arbitrary GitHub repositories**. It provides
repository-scoped, Docker/Colima-backed GitHub Actions runners without
application-specific profiles, hardcoded repository names or workflow
dependencies on the manager repository.

Current version: **0.5.5**.

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

**`ghr doctor --repo OWNER/REPO` is an end-to-end readiness check** with six stages:
(1) local Docker container installed/running and image matches,
(2) workflow toolchain verified in the image or attributed to setup actions,
(3) local runner registration and persistent volume,
(4) exact GitHub runner online, idle and correctly labeled,
(5) all `.github/workflows/*.yml` and `*.yaml` jobs configured with
`fromJSON(vars.RUNNER_LABELS_JSON ...)`,
(6) the repository variable exists and contains JSON labels
`["self-hosted","Linux","X64","repo-OWNER-REPO"]`.
It prints **exactly six aligned three-column rows**, followed by a result
row: two-digit step number, fixed-width `OK`/`FAIL` badge with green/red
background on color-capable terminals, and a concise description. There
are no introductory banners, duplicate Toolchain Doctor output or
repeated multi-line errors. Redirected output and `NO_COLOR=1` use plain
text, retaining alignment. The doctor continues through independent failures
and exits nonzero on any failed check.

Runner identity files with an optional leading UTF-8 BOM are read safely
without modifying the registration volume. Doctor is read-only and never modifies
registrations, containers, workflows or GitHub variables.

`ghr build --repo OWNER/REPO` continues to use the previous **image-only**
toolchain verification; a registered/running runner is not required for builds.

## Default repository through RUNNER_REPO

Set `RUNNER_REPO` to avoid repeating `--repo` for repository-scoped commands:

```sh
export RUNNER_REPO=r14r/BookBuilder
ghr wizard
ghr status
ghr doctor
ghr list
# An explicit --repo always wins:
ghr status --repo r14r/github-runner-test
```

The environment variable is validated as an `OWNER/REPO` value. When both are absent, repository-required commands fail; the wizard prompts for a repository.

## Guided setup and runner inventory

```sh
ghr wizard                         # interactive setup; prompts for a repository
ghr assistant --repo OWNER/REPO    # alias for wizard
ghr list                           # all locally installed runners and their container states
ghr list --repo OWNER/REPO         # one local runner plus GitHub registrations
```

The wizard checks Docker, image, persistent volume, registration, service container and running state. Completed steps are skipped. Each modification requires confirmation, and workflow routing is optional and creates a reviewable pull request. Re-running the wizard does not delete an existing registration or volume.

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

### Live log monitoring

```sh
# Behaves like tail -f: show 10 old log lines once, then only new ones.
ghr monitor --repo OWNER/REPO

# Show only new log lines, without existing history.
RUNNER_MONITOR_TAIL=0 ghr monitor --repo OWNER/REPO
```

Monitor executes `docker logs --follow --tail N` for the selected
runner container. It never prints the GitHub Actions run table or runner
status and never reprints older log lines. `Ctrl+C` stops only the
monitor process, not the runner. Use `ghr status` or `gh run list`
separately when needed.

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

## GitHub Actions workflow run management

Use the `actions` subcommands to list, cancel, or delete workflow runs in an
**explicitly selected repository**. These commands use the authenticated
GitHub CLI and do not require Docker or installed runner runtime assets.

```sh
# List all runs or select one GitHub Actions state/conclusion.
ghr actions list --repo OWNER/REPO
ghr actions list --repo OWNER/REPO --state failure
ghr actions list --repo OWNER/REPO --state in_progress

# Dry-run is the default for cancel and delete:
ghr actions cancel --repo OWNER/REPO --state in_progress
ghr actions delete --repo OWNER/REPO --state failure
ghr actions delete --repo OWNER/REPO --state all --with-cancel

# Explicitly request destructive changes:
ghr actions cancel --repo OWNER/REPO --state in_progress --yes
ghr actions delete --repo OWNER/REPO --state failure --yes
ghr actions delete --repo OWNER/REPO --state all --with-cancel --yes

# Full documentation with all 15 possible filter values:
ghr actions help
ghr actions delete --help
ghr actions delete help
```

`--yes` **requires an explicitly selected `--state`**, including
`--state all` if you intend to modify every eligible run. `delete`
only removes completed runs by default; active runs are skipped. Use
`--with-cancel` to cancel matching active runs, wait for GitHub to report
completion, then delete them. Run deletion is **irreversible**.
Bulk selection is performed **server-side** with GitHub's REST `status=STATE`
filter. If more than 1,000 matching runs exist (GitHub's search cap),
the CLI divides the search into creation-time windows, retrieves and
deduplicates every matching run, and only then begins deletions.
Each run is rechecked before a destructive API request.

GitHub GraphQL exposes read-only `WorkflowRun` objects but **does not
offer a `deleteWorkflowRun` or batch cancellation mutation**.
Deleting workflow runs therefore still requires the documented REST
`DELETE /repos/OWNER/REPO/actions/runs/RUN_ID` endpoint per run.
The CLI provides the requested bulk semantics even though GitHub itself
does not offer a single bulk-deletion mutation. If GitHub cannot fully
enumerate results, it fails before deleting any run. Invalid
permissions, API errors and cancellation timeouts return a nonzero exit status.

For the detailed state and conclusion descriptions, run
`ghr actions --help`. Supported values:
`all`, `success`, `failure`, `cancelled`, `timed_out`,
`action_required`, `neutral`, `skipped`, `stale`,
`completed`, `queued`, `in_progress`, `waiting`,
`pending`, and `requested`.

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


The Go CLI source and tests live in `cmd/cli/`; `extractions/setup-just` is recognized as a workflow-managed tool.

### Next verification and PR review

Run `just next` to see local validation steps, the current Git branch and working-tree state, and **all open GitHub pull requests** with their CI/merge status. It prints the appropriate review/checkout/merge commands without executing them.

Run `just next --run` to run `git diff --check`, then `just build` (which already runs the full `just ci` pipeline), and retrieve GitHub checks for every open PR. It does **not** automatically check out or merge other branches. For the PR belonging to the current branch, an eligible merge is offered **only after explicit confirmation** and only when CI checks pass, the worktree is clean, and GitHub reports it mergeable. Drafts and outstanding review requirements block this merge.

Requires Go, `gh` authenticated to GitHub, and the normal local build toolchain.


### Local-first runner monitoring

`ghr monitor --repo OWNER/REPO` follows the local runner's Docker log and
diagnostic files in `/runner/_diag`. It does **not** call the GitHub API.
The diagnostic output is best-effort and cannot guarantee complete step
stdout/stderr or authoritative GitHub step conclusions.

Use `ghr monitor --repo OWNER/REPO --github` only when GitHub's step
statuses are required; this opt-in legacy mode polls GitHub and consumes
API quota.

Start a localhost-only JSON/SSE API with:

```sh
ghr monitor-api --repo r14r/github-runner --listen 127.0.0.1:8765
# alternatively
just monitor-api r14r/github-runner
curl http://127.0.0.1:8765/api/monitor
curl -N http://127.0.0.1:8765/api/monitor/runners/github-runner-r14r-github-runner/events
```

The API also provides `/api/monitor/runners`,
`/api/monitor/runners/{container}`, and
`/api/monitor/runners/{container}/logs`. Event fields are `type`,
`runner`, `source`, `message`, and `time`. Diagnostic files may
contain sensitive data; do not expose the API publicly. This API is intended
for one selected repository/container per server process.

