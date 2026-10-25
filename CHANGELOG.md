# Changelog

## 0.1.0 — Standalone GitHub Actions runner management (2026-10-25)

- Extract macOS and Ubuntu BookBuilder runner provisioning, read-only doctors, service lifecycle, log cleanup and safe GitHub Actions history controls into this own repository.
- Keep default `RUNNER_REPO=r14r/BookBuilder-with-typst` and label `bookbuilder`, allow optional repository and label overrides.
- Add explicit `just migrate-from-bookbuilder <absolute-old-runner-dir>` to preserve existing macOS registration/launchd identity when relocating from the BookBuilder checkout.
- Provide shell syntax and mocked regression tests. CI and live host acceptance are independent validation steps.
