# Current State — 2026-10-31

- Feature branch `feat/multi-repo-docker-colima-v0.1.2`: proposed v0.1.2 multi-repository extension.
- Previous native macOS and Ubuntu BookBuilder profiles remain unchanged.
- New scripts under `runner/` install Linux x64 runners using Docker/Colima,
  register one runner per repository, and create reviewable workflow PRs.
- Nine workflow patching unit tests and shell syntax checks passed locally.
- Actual Colima runner registration, Docker image builds, and CI on target host
  still require acceptance testing; do not claim them complete.
- Deployment of an existing registered runner must never be automated by CI.
