# Changelog

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
