# Source of truth

The canonical source of this runner-manager application is
`https://github.com/r14r/github-runner`, branch `main`.
Changes are developed in feature branches and merged through reviewed
pull requests with successful CI.

This project owns only generic Docker-backed GitHub Actions runner
provisioning, registration, lifecycle and workflow-requirement analysis.
The actual workflows and their configuration remain in each target repository.
No target repository is hardcoded, referenced or required by this codebase.
