# Multi-repository Docker/Colima extension (proposed v0.1.2)

The root `r14r/github-runner` remains the single source of truth for runner
lifecycle, installation, diagnosis, migration and maintenance. The `runner/`
subdirectory extends it with per-repository Docker/Colima runners and a
conservative PR-based workflow migration utility.

- Legacy native Mac/Ubuntu BookBuilder profiles remain as-is. Their default
  `r14r/BookBuilder-with-typst` should be separately evaluated against current
  `r14r/BookBuilder` before changing existing registered runners.
- Registration tokens are created by `gh api`, never stored in source files,
  and can still appear briefly as arguments to the upstream `config.sh` process.
- Docker image uses Ubuntu 24.04 + runner 2.338.1. The runner process runs
  without `sudo`; it does not get a Docker socket by default. If opting in,
  the socket effectively grants broad access to the Colima daemon.
- The workflow editor has no dependency on PyYAML, and deliberately rejects
  expressions, dynamic/multiline `runs-on`, or mixed-OS migrations without
  `--allow-cross-os`. All requested job IDs must resolve.
- New CI recipe checks all new shell scripts and runs unit tests.
- PR creation is the default; `--merge` must be explicitly provided to
  automatically merge. CI gate does not bypass repository protection.
- No automated deletion of `.runner`, `.credentials`, Docker volumes, or
  existing server-side registration.
- Docker-outside-of-Docker is not fully equivalent to a GitHub-hosted Ubuntu
  environment. Container actions, Compose bind mounts, Playwright, and resource
  requirements must be verified on the actual Colima host.

## Official references

- https://github.com/actions/runner/releases/tag/v2.338.1
- https://docs.github.com/en/actions/reference/security/secure-use
- https://github.blog/changelog/2026-09-23-node-20-is-no-longer-available-in-github-actions/
