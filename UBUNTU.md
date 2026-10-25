# Dedicated self-hosted GitHub Actions runner

## Scope

The four workflows in `.github/workflows` require a repository-specific runner labeled
`self-hosted`, `bookbuilder`. **Ubuntu 24.04 x64 and macOS Apple Silicon/Intel**
are supported, with a platform-aware read-only host preflight.

For macOS installation follow **[MACOS.md](MACOS.md)** instead of the Ubuntu
instructions below. One runner accepts one job at a time;
the jobs queue rather than run concurrently. This is **not** the optional backend API
deployment from the project's root `docker-compose.yml`; Docker is not required.

| Workflow | Jobs | Tools / expected output |
| --- | --- | --- |
| `ci.yml` | Python, PHP | Python 3.12, Ruff, pytest, Typst, PHP 8.2 + curl/zip/mbstring, 22 PDF previews/artifacts |
| `context-check.yml` | context | AI context validation, Python 3.12, pytest |
| `epub-check.yml` | epubcheck | Python 3.12, Java 21, official EPUBCheck 5.4.0, EPUB artifact |
| `pdf-smoke.yml` | amazon-book | Python 3.12, Typst 0.15.1, Healing Kitchen PDF artifact |

CI's Python tests execute before `setup-typst`; Typst therefore must be installed on the
host in advance so Typst tests are **not silently skipped**. Every Python job uses an
isolated workspace `.venv`, rather than writing packages into the shared runner toolcache.
The existing `setup-python`, `setup-java`, Typst and artifact actions remain in use.
PHP 8.2 is preinstalled at host provisioning and validated in the workflow. Jobs never
require sudo, root or a Docker socket.

## Ubuntu host specification

- **Dedicated Ubuntu 24.04 LTS x86_64 VM**, systemd, 4+ vCPUs, 16 GB RAM and
  80+ GB free SSD (size for large PDF compilations; tune to actual measured usage).
- Outbound HTTPS to GitHub Actions, release/artifact endpoints, PyPI, Typst packages
  and W3C's EPUBCheck downloads; no inbound port forwarding necessary.
- Host administrator with `sudo` and `gh` authenticated to this repository with
  runner-administration permissions. Registration requires a temporary GitHub token
  obtained through `gh api`; **do not store this token** in the repo.
- Provisioning installs PHP 8.2 from the **third-party** Ondrej PHP apt PPA because
  the Ubuntu 24.04 archive defaults to PHP 8.3. Review this trust dependency for
  your environment.

## Installation on an Ubuntu VM

Use a fresh VM and a regular administrator account, not `root`:

```bash
sudo apt-get update
sudo apt-get install -y git gh just
gh auth login --scopes repo
gh repo clone r14r/BookBuilder-with-typst
cd BookBuilder-with-typst
# During PR review, bootstrap the runner directly from this feature branch:
git fetch origin infra/self-hosted-bookbuilder-runner
git switch --track origin/infra/self-hosted-bookbuilder-runner
cd github-runner
# Once PR #11 is merged, the same script also exists on main.
just install
just doctor
```

The installer verifies the OS/architecture, provisions Python 3.12, Java 21,
PHP 8.2, Typst 0.15.1, fonts, FFmpeg and Just, downloads a pinned
GitHub Actions runner archive with SHA-256 verification, and registers it under
`/opt/bookbuilder-actions-runner` as `bookbuilder-runner`. The runner's
GitHub registration is idempotent: reruns keep the existing registration.
The systemd service runs as `bookbuilder-runner` **without sudo**.
The script does not write API keys or `.env` files.

Verify `Online` and the `self-hosted` / `bookbuilder` labels at
https://github.com/r14r/BookBuilder-with-typst/settings/actions/runners .
The installed binary is initially pinned to Actions runner **2.336.0**;
GitHub's runner service can update itself when new compatible releases appear.
Update the script's version and verified checksum when re-provisioning a new VM.

## Validation

After the runner appears Online, trigger the manually dispatchable EPUB and
PDF smoke workflows from Actions. Then push a harmless test change in a PR to
trigger CI and the AI-context check (the latter uses a path filter, so change
a matching file or manually verify its next real run). Verify that **all four
workflows** succeed and the resulting PDF/EPUB artifacts are nonempty.

```bash
just doctor
just status
just monitor
# GitHub Actions logs: https://github.com/r14r/BookBuilder-with-typst/actions
```

Locally in a checkout with project dependencies, return to the repository
root (`cd ..` from `github-runner/`) before running application commands:

```bash
just bootstrap
just context-check
just precommit
```

Some builds require downloading Typst packages and the official EPUBCheck ZIP.
Without Internet access these parts will not pass. The runner being Online is
not by itself evidence that builds have passed.

## Operations and security

The runner is a **persistent** GitHub Actions runner, not an ephemeral VM-per-job
service. Jobs can execute arbitrary code from pull requests. Restrict writers,
protect `main`, require reviews and do not execute untrusted fork PRs on this
runner. Do not reuse this VM for production workloads; never attach cloud
credentials, API keys, a privileged Docker socket, or passwordless sudo. If an
untrusted job runs, dispose of the VM and re-provision rather than assuming
the runner's workspace cleanup removes persistence.

Restrict SSH, apply OS/apt security updates in a maintenance window, monitor
disk usage under `/opt/bookbuilder-actions-runner/_work`, keep one runner
per VM and rotate/decommission the VM according to your security policy.
Do not call `./config.sh remove` while its service is running; stop and
uninstall the service before deregistering.

Within `github-runner/`, service control uses `just start`, `just stop`,
`just status`, and live monitoring uses `just monitor`.

If an Actions job waits for a runner, check that the service is Online and
that the `bookbuilder` label is present. If PHP validation fails, rerun the
administrator installer; the CI job intentionally cannot modify host packages.
On macOS run `install-macos.sh` without sudo. The macOS runner is a per-user
LaunchAgent and requires a logged-in graphical user session.

## Maintenance and GitHub-hosted fallback

From `github-runner/`, run `just cleanup-preview` to inspect diagnostic logs
older than 30 full days; `just cleanup` removes only those `_diag/*.log`
files. The restricted runner user owns the runtime; use suitable host
permissions. `_work`, runner credentials and tool caches are never cleaned
by this command.

All BookBuilder workflows choose a running, idle `bookbuilder` runner when
repository secret `RUNNER_STATUS_TOKEN` permits reading runner status
(`Administration: Read-only`). Otherwise they run on GitHub-hosted
`ubuntu-24.04` with the necessary job-specific dependencies. This is a
one-time routing decision before each job, not an automatic reroute after
queueing. See [README.md](README.md#prefer-self-hosted-fall-back-to-github-hosted).


## Eigenständiges Repository

Runner-Installation und Registrierung werden seit der Auslagerung von
[github-runner](https://github.com/r14r/github-runner) verwaltet. Clone dieses
Repository getrennt vom BookBuilder-Projekt und verwende darin `just`.
Zum Übertragen einer laufenden macOS-Installation zuerst die laufenden Jobs
beenden und anschließend in diesem Repository
`just migrate-from-bookbuilder "/vollständiger/Pfad/zum/alten/BookBuilder/github-runner/.local/bookbuilder-actions-runner"`
ausführen. Der alte launchd-Dienst wird deinstalliert, die komplette
Registrierung ohne Neuanmeldung verschoben und der Dienst neu eingerichtet.
`just install` lässt bestehende Registrierungen bestehen. Cloud-Sync aus
`.local/` ausschließen.
