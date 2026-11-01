# Changelog

## 0.3.2 — Detailed built-in command help

- Add `ghr help`, `ghr help COMMAND`, `ghr help runner SUBCOMMAND`,
  `ghr help host SUBCOMMAND` and `--help` / `-h` at every command level.
- Document purpose, required/optional parameters, permitted values, examples,
  environment overrides and safety warnings for each implemented command.
- Process help requests before `--repo` validation and runtime asset checks,
  allowing `ghr status --help` without installing/configuring a runner.
- Add Go regression tests for all help topics, aliases, no-assets behavior,
  and unknown commands.

## 0.3.0 — Native Go Docker lifecycle and workflow migration (2026-10-31)

- Remove all Python project files and pytest dependency from CI and install.
- Port conservative workflow patching, validation and PR creation to Go.
- Move repository-scoped Docker/Colima download, registration, status, monitoring,
  image selection and container refresh to Go.
- Retain safely isolated macOS/Ubuntu host service scripts as runtime
  compatibility assets; no runner registrations or Docker volumes are migrated.
- Replace Python regression tests with Go unit and race tests.
- Simplify `just bootstrap`, `just ci`, `just build`, and `just install`.
- Verify these recipes on GitHub Actions using a pinned just binary and continue
  building and verifying the BookBuilder Docker image in CI.

## 0.1.2 — Multi-repository Docker/Colima runner extension (2026-10-31)

- Add repository-scoped Linux x64 runners for older Intel macOS via Colima/Docker.
- Support multiple repositories with isolated runner names and persistent volumes.
- Verify SHA-256 for runner 2.338.1 assets; obtain fresh registration tokens using gh.
- Add conservative workflow migration via PR; do not automatically merge by default.
- Keep the Colima Docker socket opt-in and protect public-repository registration.
- Add nine workflow patching regression tests, shell syntax gate and just commands.
- Preserve existing native macOS/Ubuntu installations, registrations and service recipes.

## 0.1.0 — Standalone GitHub Actions runner management (2026-10-25)

- Extract macOS and Ubuntu BookBuilder runner provisioning, read-only doctors, service lifecycle, log cleanup and safe GitHub Actions history controls into this own repository.
- Keep default `RUNNER_REPO=r14r/BookBuilder-with-typst` and label `bookbuilder`, allow optional repository and label overrides.
- Add explicit `just migrate-from-bookbuilder <absolute-old-runner-dir>` to preserve existing macOS registration/launchd identity when relocating from the BookBuilder checkout.
- Provide shell syntax and mocked regression tests. CI and live host acceptance are independent validation steps.
