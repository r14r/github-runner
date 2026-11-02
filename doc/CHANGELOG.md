# Changelog

## 0.5.4 — Full runner doctor

- Expand `ghr doctor --repo OWNER/REPO` to six read-only checks covering:
  running local Docker container/image, toolchain, persisted registration,
  GitHub online/idle runner and repository label, all YAML/YML workflow
  job selectors, and the `RUNNER_LABELS_JSON` repository variable.
- Verify variable value is a JSON label array for the expected repository
  runner. Reject missing variables or GitHub-hosted selections.
- Check all workflow files; report job-level missing/static selectors
  and treat reusable-workflow call jobs as exempt.
- Print an overall PASS/FAIL with individual stage diagnostics; do not
  stop on unrelated errors. Never mutate container/runner/GitHub state.
- Retain the standalone image/toolchain doctor for `ghr build`, so builds
  do not require a registered or running runner.
- Add unit tests for all six stages, missing labels, offline/busy
  registrations, variable values and .yaml/.yml workflow routing.


## 0.5.2 — Tail-follow only monitor

- `ghr monitor --repo OWNER/REPO` streams **only container logs**
  using a single `docker logs --follow --tail N` process, like `tail -f`.
- Remove all status polling, GitHub Actions run tables, headers and repeated
  log snapshots from the monitor command.
- Initial tail defaults to 10 lines (like `tail -f`). Set
  `RUNNER_MONITOR_TAIL=0` for only newly emitted lines.
- Remove the old polling interval `RUNNER_MONITOR_SECONDS` behavior.
- Tests verify exactly one Docker stream, absence of GitHub/status requests,
  argument validation and propagation of Docker errors.


## 0.5.1 — Safe automatic runner refresh

- `ghr refresh --repo OWNER/REPO` now stops, replaces and restarts an already-running **idle** runner automatically.
- Validates persisted GitHub runner identity and GitHub `busy` state before stopping; errors or active jobs abort with no runner interruption.
- Previously stopped runners remain stopped. Existing registration volumes are preserved.
- Preflights Docker-socket privileges before stopping; retains explicit `RUNNER_DOCKER_SOCKET=1` opt-in.
- Adds recovery attempt if old-container removal fails, and regression tests for all lifecycle states and failure cases.

## 0.5.0 — Repository-neutral toolchain and readable diagnostics

- Remove hardcoded external target repositories from CLI, test fixtures,
  Docker image stages, workflow CI and documentation.
- Remove legacy application-specific host provisioning scripts, profiles,
  workflow checkouts and obsolete historical architecture notes.
- Give every repository an isolated `toolchain` Docker image tag by default.
- Derive image dependencies from workflow actions and audited package maps,
  distinguishing `WORKFLOW ACTION` from image-preinstalled tools.
- Format `ghr doctor` as numbered stages with explicit repository/revision,
  clear per-tool `OK`/`FAIL` status, and a concise result summary.
- Hide routine Git sparse-clone transfer output, retaining diagnostics on
  failure.
- Preserve existing Docker registrations and volumes; do not perform any
  registration-changing migration automatically.
- Keep `just ci` read-only for Go source files.

## 0.4.0 — Workflow requirements

- Add Go workflow-analysis and YAML manifest export.
- Add generic `requirements`, `build` and `doctor` command structure.

## 0.3.x — CLI and lifecycle

- Add detailed command help, `--repo` aliases and safe list/uninstall
  operations.
- Migrate Docker lifecycle and workflow patching to Go.
