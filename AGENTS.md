# AGENTS.md — github-runner-cli

- Canonical repository: https://github.com/r14r/github-runner
- Single version source: root `VERSION`. Go builds inject this version through ldflags.
- Go module: `github.com/r14r/github-runner`.
- Required recipes: `just bootstrap`, `just ci`, `just build`, `just install`.
- Do not reintroduce Python scripts, pytest, pip, or Python CI dependencies.
- Active Docker lifecycle logic and workflow rewriting must stay in Go.
- Docker entrypoint and legacy macOS/Ubuntu host compatibility assets may
  remain shell scripts until separately migrated and tested on real hosts.
- No changes may erase Docker volumes or existing runner registrations.
- Never re-register an existing online/busy runner automatically.
- Docker socket is privileged and opt-in; public repositories require
  `--allow-public`.
- Do not assert host validation that was not executed; CI builds and tests
  the BookBuilder Docker image on GitHub-hosted Ubuntu.
- Follow feature branch → PR → tests → merge.
