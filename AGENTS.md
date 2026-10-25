# AGENTS.md – github-runner

- Canonical repository: https://github.com/r14r/github-runner
- Version source: root `VERSION` (SemVer).
- Changes via feature branch, commit, PR, tests and merge.
- `README.md`, `MACOS.md` and `UBUNTU.md` document operator behavior.
- Local runner runtime (`.local/`) holds credentials: never read, export, commit or delete it during unrelated changes.
- Never overwrite or remove a registered runner automatically. Migrate only from an explicit old path when no job is running.
- `just install`, `just doctor`, `just test`, `just ci` are the supported recipes; read `justfile` before introducing more.
- Keep supported host profile (macOS arm64/x64, Ubuntu 24.04 x64) and read-only doctor stable.
- `RUNNER_REPO` defaults to `r14r/BookBuilder-with-typst`; treat runtime service accounts and workspace data as host concerns.
- Updates to runner integration in a consumer repository require coordinated workflows and pinned checkout version, not copying scripts back into the application repo.
- Do not claim physical host tests or a successful GitHub Actions run without evidence.
- Document durable decisions in `docs/ai/DECISIONS.md` and progress in `docs/ai/CURRENT-STATE.md`.
