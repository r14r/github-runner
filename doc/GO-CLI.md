# Go CLI v0.5.5 — repository-neutral runner manager

## Typical workflow

```sh
ghr requirements --repo OWNER/REPO --output toolchain.yaml
ghr build --repo OWNER/REPO
ghr doctor --repo OWNER/REPO
ghr runner register --repo OWNER/REPO
ghr start --repo OWNER/REPO
ghr status --repo OWNER/REPO
```

`ghr help COMMAND` and `ghr COMMAND --help` print full syntax, valid values,
examples and safety notes before attempting authentication or Docker access.

## Full runner doctor (read-only)

```sh
ghr doctor --repo OWNER/REPO
```

The command prints only six compact lines and a final result in three
columns (two-digit step number, color-coded status, description). On
interactive terminals, `OK` has a green background and `FAIL` a red
background. Non-TTY output and `NO_COLOR=1` use fixed-width plain text.
The Toolchain Doctor's separate headings and footer are suppressed here;
`ghr build` retains its original detailed toolchain diagnostics.

```text
01    OK    Docker: github-runner-r14r-bookbuilder (running; image matches)
02    OK    Toolchain: 0 tools verified in image, 4 provided by workflow actions; 6 workflows
03    OK    Registration: example-agent (ID 42; volume github-runner-r14r-bookbuilder-state)
04    OK    GitHub: example-agent (online; busy=false)
05    OK    Workflows: 6 files, 7/7 jobs use RUNNER_LABELS_JSON
06    OK    RUNNER_LABELS_JSON: ["self-hosted","Linux","X64","repo-r14r-bookbuilder"]
--    OK    RESULT: PASS — all 6 checks OK (r14r/BookBuilder)
```

The command performs six checks, collecting independent failures:

1. Local Docker/Colima container exists, is running and uses the selected image.
2. Workflow requirements and toolchain available in that image (recognized setup
   actions are reported as workflow-managed).
3. Persistent local registration volume contains a valid runner ID, name and
   repository identity. A leading UTF-8 BOM in the JSON is tolerated.
4. The exact GitHub runner is registered, `online`, **idle** (`busy=false`)
   and has labels `self-hosted`, `Linux`, `X64`, `repo-OWNER-REPO`.
5. Every executable job in every `.github/workflows/*.yml` and `*.yaml`
   file selects `runs-on` via `fromJSON(vars.RUNNER_LABELS_JSON ...)`.
   Reusable-workflow calling jobs with `uses:` and no local `runs-on`
   are reported as exempt.
6. GitHub's `RUNNER_LABELS_JSON` repository variable exists, is a JSON array
   and contains all required self-hosted runner labels.

A missing variable or a variable selecting `ubuntu-24.04` instead of the
self-hosted runner is flagged as FAIL. A runner executing a workflow is
reported as busy/not idle; rerun doctor after the job completes.

To configure workflows:

```yaml
runs-on: ${{ fromJSON(vars.RUNNER_LABELS_JSON || '"ubuntu-24.04"') }}
```

To select the self-hosted runner:

```sh
gh variable set RUNNER_LABELS_JSON --repo OWNER/REPO --body '["self-hosted","Linux","X64","repo-owner-repo"]'
```

The doctor does **not** change GitHub variables or workflow files,
re-register runners, or alter Docker state. The existing `ghr build`
command continues to perform only the image/toolchain check before a
runner is installed.

## Toolchain model

The Go requirements analyzer reads the target repository's workflow files
through a temporary sparse checkout. No workflow code is executed. A
manifest records workflow filenames, setup actions, required commands and
warning messages. The `toolchain` image builds from pinned Ubuntu 24.04
runner-base and provisions only a fixed allowlist of dependencies.

A runtime provided by a recognized setup action is marked
`WORKFLOW ACTION` during the doctor check. For preinstalled tools the
doctor checks availability and supported pinned versions inside the exact
built image. Unknown/dynamic tools require a setup action or explicitly
provided custom image; they are never passed blindly to a shell.

## Automatic safe refresh

```sh
ghr build --repo OWNER/REPO
ghr doctor --repo OWNER/REPO
ghr refresh --repo OWNER/REPO
ghr status --repo OWNER/REPO
```

For a running container, `refresh` verifies the persisted runner ID/name
against GitHub, checks `busy=false`, then executes **stop → replace → start**.
A stopped container is replaced without being started. Unknown runner
identity, GitHub API unavailability or a busy registration aborts before
stopping. An existing privileged Docker socket is preserved only when
`RUNNER_DOCKER_SOCKET=1` is explicitly set. The GitHub registration
and persistent Docker volume are never removed by refresh.

## Live log monitoring

`ghr monitor` behaves like `tail -f` on the runner container logs.
There is no status snapshot, Actions run table or periodic polling.

```sh
ghr monitor --repo OWNER/REPO
RUNNER_MONITOR_TAIL=0 ghr monitor --repo OWNER/REPO
RUNNER_MONITOR_TAIL=30 ghr monitor --repo OWNER/REPO
```

The default is the last 10 lines **once**, then continuous live streaming.
`RUNNER_MONITOR_TAIL` accepts 0–1000. Setting it to `0` prints only
new log entries. The obsolete `RUNNER_MONITOR_SECONDS` is ignored.
`Ctrl+C` exits the monitor, not the runner.

## GitHub Actions runs: list, cancel and delete

```sh
ghr actions list --repo OWNER/REPO [--state STATE]
ghr actions cancel --repo OWNER/REPO [--state STATE] [--yes]
ghr actions delete --repo OWNER/REPO [--state STATE] [--with-cancel] [--yes]

ghr actions help
ghr actions list --help
ghr actions cancel --help
ghr actions delete --help
ghr actions delete help
```

All Actions subcommands require `--repo`; none require Docker runtime
assets. The status filter is optional for listing/dry-runs, defaulting to
`all`. The **mutating** variants require both `--yes` and explicit
`--state` to avoid accidental bulk actions. Without `--yes`,
`cancel` and `delete` preview the affected runs without altering GitHub.

The CLI supports completed conclusions (`success`, `failure`,
`cancelled`, `timed_out`, `action_required`, `neutral`,
`skipped`, `stale`), `completed` itself, and active statuses
(`queued`, `in_progress`, `waiting`, `pending`, `requested`).
It also accepts `all` to match everything. Use
`ghr actions delete help` to see every allowed value and meaning.

`delete` without `--with-cancel` skips active runs.
`delete --with-cancel` requests cancellation of matching active runs,
waits for confirmation of completion (up to one minute), and only then
deletes them. The GitHub run is rechecked before every destructive request;
all matching run IDs are gathered before deletion to avoid skipped runs.
GitHub's REST API supports a server-side `status=STATE` filter
that limits each filtered search to 1,000 results. The CLI partitions
large searches by creation timestamp to cover all available results
before making changes; it refuses deletion if it cannot fully
enumerate the matching set.

The GitHub GraphQL schema includes `WorkflowRun` but no
`deleteWorkflowRun` mutation. Bulk selection uses REST filtering,
and each deletion is an individual REST operation; a single
GraphQL bulk delete cannot be implemented with the current GitHub API. Failed
GitHub API requests stop the operation. Run deletion cannot be undone.

## Runner state and compatibility

Each repository gets its own container name, volume and image tag.
The Docker backend is supported on Linux and macOS via Docker/Colima;
legacy native-host management has been retired.
Existing registration volumes and GitHub registration tokens are not
deleted by code updates.

```sh
ghr list-installed [--repo OWNER/REPO]
ghr uninstall --repo OWNER/REPO --dry-run
ghr uninstall --repo OWNER/REPO --delete-data
ghr uninstall --all --yes
ghr self-uninstall --yes
```

Uninstall requires exact registration identity and refuses busy jobs.
Deleting data requires `--delete-data`.

## Tests

```sh
just bootstrap
just fmt-check
just ci
just build
just install
```

CI runs Go unit/race tests and generic Docker image build/doctor tests.
`just update` runs `git pull --ff-only`, bootstrap, CI and build.
