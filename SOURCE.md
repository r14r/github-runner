# SOURCE.md

Canonical GitHub repository: https://github.com/r14r/github-runner

Source-of-truth `main`; changes originate in feature branches and pull requests. The initial profile was extracted from `r14r/BookBuilder-with-typst` PR #11. This repository owns runner host provisioning, registration lifecycle and operator scripts. The application repository retains its workflow YAML files and uses a pinned checkout of this repository for host preflight.

Root `VERSION` is the sole release version source. No cloud runner credentials are stored in Git.
