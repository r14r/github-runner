# Current State — 2026-10-25

- New standalone GitHub repository for previously BookBuilder-local `github-runner/` scripts.
- Version 0.1.0 in initial extraction branch, pending CI and real macOS/Ubuntu smoke tests.
- macOS migration from the previous BookBuilder `.local` must be invoked by the user on that machine; repository access cannot move host-local credentials or launchd services.
- BookBuilder PR #11 must update all workflow preflight paths to an external, pinned checkout before its local scripts can be deleted.
