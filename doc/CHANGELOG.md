# Changelog

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
