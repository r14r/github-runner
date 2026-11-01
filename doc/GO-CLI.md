# Go CLI v0.5.1 — repository-neutral runner manager

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
