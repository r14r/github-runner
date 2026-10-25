# BookBuilder GitHub Actions Runner on macOS

Supports **Apple Silicon (arm64)** and **Intel (x86_64)**. The same GitHub
workflows also support Ubuntu; all five jobs use `[self-hosted, bookbuilder]`.
The host doctor detects the actual operating system and fails early if required
dependencies are unavailable.

> Security: use a dedicated, non-administrator macOS login account with no
> private secrets, SSH credentials, cloud credentials or production data.
> GitHub Actions PR jobs can execute arbitrary code. On a personal Mac,
> run only trusted repository code. The LaunchAgent runs with all permissions
> of the logged-in macOS account.

## 1. Prepare macOS

Install Homebrew from https://brew.sh/ if it is not already installed.
Then open **Terminal** in the account that will run jobs:

```bash
xcode-select --install  # Only if Command Line Tools are missing
brew --version
just --version
git --version
uname -m                # arm64 or x86_64
```

The installer requires Homebrew and will install: `python@3.12`, `php@8.2`,
`openjdk@21`, GitHub CLI, Git, Just, FFmpeg, Fontconfig, jq, xz. Homebrew
`php@8.2` is keg-only, so the installer adjusts the environment PATH without
changing the system-wide PHP version. On older Intel Macs, Homebrew may have
to compile unavailable bottles. **Do not use `sudo` for the installer.**

## 2. Authenticate and clone the PR version

```bash
brew install gh git just
gh auth login --scopes repo
gh repo clone r14r/github-runner
cd github-runner
```

The GitHub CLI account must be allowed to manage Actions runners for this
private repository. Tokens are obtained through the authenticated GitHub CLI
only at registration time; neither token nor API keys are committed.

## 3. Install and register

```bash
just install
just doctor
```

The script autodetects arm64/x64, installs Typst **0.15.1** and the matching
GitHub runner **2.338.1**, verifies both download SHA-256 checksums, and registers
one runner under `github-runner/.local/bookbuilder-actions-runner` with the
`bookbuilder` label. Homebrew dependencies and the runner registration are
reused on subsequent executions; existing custom `.env` keys are preserved.

It configures the runner's own `.env` with `PATH` and `JAVA_HOME`, because
`launchd` does **not** load your interactive `.zshrc`. This allows workflows
to find Homebrew PHP 8.2, Python 3.12, OpenJDK 21 and Typst.

## 4. Migrate an existing installation

The macOS runner's installation and GitHub registration are stored in the
**`github-runner/.local/bookbuilder-actions-runner/`** directory of this
checkout. It contains private `.credentials`, `.runner`, `_work` and
`_diag` files, so `.local/` is excluded in the repository `.gitignore`.

If a runner was previously installed at
`~/.local/share/bookbuilder-actions-runner`, ensure **no workflow job is
currently running** before the move. From `github-runner/`:

```bash
just migrate       # optional; performed automatically by just install
just status
```

Migration stops and uninstalls the old launchd service, moves the original
runner directory, then installs and starts launchd at the new absolute path.
The original registration and credentials are preserved; GitHub does not
require a second registration token. Migration refuses to overwrite an
existing target directory and is safe to rerun. If the directory move succeeds
but the new service fails to install, repair using `just install`.

**Important:** `.gitignore` does not prevent Dropbox, iCloud or other cloud
synchronization. Exclude `github-runner/.local/` separately from those tools.
Do not run `git clean -fdx`, since it can delete ignored runner state.

## 5. Service and validation

The runner is launched as a **per-user launchd LaunchAgent**. Unlike a Linux
systemd service, it normally requires the runner account to be signed into
the **macOS graphical session**. The Mac must remain powered on, awake and
network-connected to accept jobs.

```bash
just status
just stop
just start
just monitor
```

Check https://github.com/r14r/BookBuilder-with-typst/settings/actions/runners
for **Online**, macOS, the correct CPU architecture and `bookbuilder` label.
All workflows include a read-only host preflight. One runner processes
one job at a time; other jobs will queue.

To run the dispatchable workflows from a repository checkout:

```bash
gh workflow run epub-check.yml --ref infra/self-hosted-bookbuilder-runner
gh workflow run pdf-smoke.yml --ref infra/self-hosted-bookbuilder-runner
gh run list --repo r14r/BookBuilder-with-typst --limit 10
```

CI and AI context automatically run on a matching pull request. Confirm that
**all five jobs** finish successfully and the EPUB/PDF uploads are nonempty.
The macOS provisioning script cannot guarantee that the project's code passes:
builds must be tested on the actual Mac before merging PR #11.

## 6. Fix: PHP extension and Typst preflight false negatives

The original preflight could report `curl` or `mbstring` missing even when
Homebrew PHP loaded them: piping `php -m` to `grep -q` with Bash `pipefail`
could terminate the producer early. The preflight now checks each module using
PHP's `extension_loaded()` API instead. The Typst check also recognizes the
official `typst 0.15.1 (commit)` CLI output instead of requiring an exact string.

If an earlier install stopped after downloading Typst, update the branch and
rerun the **same installer**. It reuses the already installed Homebrew packages
and checksums and can continue through runner registration:

```bash
git pull --ff-only
just install
just doctor
```

From within `github-runner/`, use `just start`, `just stop`,
`just status` and `just monitor` for administration and live monitoring.

For direct diagnostics:

```bash
"$(brew --prefix php@8.2)/bin/php" -r '
foreach (["curl", "zip", "mbstring"] as $ext) {
    printf("%s: %s\\n", $ext, extension_loaded($ext) ? "OK" : "MISSING");
}'
"$HOME/.local/bin/typst" --version
```

If any PHP module really is absent according to the API, inspect
`php --ini`, `php -m`, and the Homebrew PHP configuration. Do not blindly
install third-party extensions before identifying the active `php.ini`.

## 7. Repair stale or deleted GitHub registration

A macOS launchd output such as `Started: - 0 actions.runner...` only means
that the service was launched locally. If the newest `_diag/Runner_*.log`
contains

```text
Failed to create a session. The runner registration has been deleted from the server, please re-configure.
```

then GitHub no longer accepts the local credentials. After confirming that
no CI job is running:

```bash
git pull --ff-only
cd github-runner   # if not already in this directory
just status       # shows GitHub online/offline separately from launchd
just repair       # explicit; never performed automatically by just install
just status
just monitor
```

`just repair` verifies the GitHub API state (and stops if the runner with
this name is online/busy), stops and uninstalls the old launchd service,
uses the official `./config.sh remove --local` option, fetches a fresh
short-lived GitHub registration token, registers/replaces the offline runner
using the same name and `bookbuilder` label, and reinstalls/starts launchd.

This resets **local GitHub runner registration credentials only**. The
installed binaries, `_work/` and `_diag/` remain. GitHub's server-side
registration can have been removed already; we do not issue an unconditional
runner DELETE. If the API refuses listing runners, ensure `gh auth status`
works and your account can administer runners for this private repository.

**If registration still fails**, verify your Mac's system date/time and
time synchronization, then inspect the latest `_diag` file. A stale or
invalid runner OAuth token can also trigger the server-deleted message.

## 8. Troubleshooting



- **Job queued:** verify `just status` **GitHub online state** (not only launchd), label `bookbuilder`,
  graphical login, and that macOS isn't sleeping.
- **Missing PHP/Java/Typst:** rerun `just install`; the
  launchd environment must include Homebrew paths. Inspect
  `github-runner/.local/bookbuilder-actions-runner/.env`, but do not publish it.
- **macOS login issue:** LaunchAgents commonly cannot run before GUI login.
  Use a dedicated always-signed-in build account (with appropriate security
  controls) or a dedicated headless build infrastructure for unattended work.
- **Outdated packages:** `brew update && brew upgrade` in a maintenance window,
  then rerun installer and doctor. The runner can self-update; pinned fresh
  installations are periodically reviewed in the repository.
- **Logs:** inspect `github-runner/.local/bookbuilder-actions-runner/_diag/`.
- **Removal:** `./svc.sh stop && ./svc.sh uninstall`; follow the official
  GitHub runner removal instructions to deregister before deleting files.

A macOS runner is **not a sandbox**. Untrusted PRs can modify your login
account's files. Prefer a disposable Mac or VM over a daily-use workstation.

## 9. Diagnostic cleanup and hosted fallback

Within `github-runner/`, `just cleanup-preview` lists old diagnostic logs and
`just cleanup` removes regular `_diag/*.log` files older than 30 full days.
Use `just cleanup 7` to change the retention to seven days. The registration,
credentials, `_work` and `_tool` folders remain untouched; the service need
not be stopped.

All BookBuilder workflows route through a GitHub-hosted availability selector.
If the self-hosted Mac is `online` and `idle`, jobs prefer it; otherwise
GitHub-hosted Ubuntu is chosen. To allow the selector to read runner status,
configure `RUNNER_STATUS_TOKEN` as an Actions repository secret using a
fine-grained token for this repository with `Administration: Read-only`.
Without the secret, GitHub-hosted is chosen. Selection is evaluated before
scheduling, not a fallback timeout for already queued jobs. See
[README.md](README.md#prefer-self-hosted-fall-back-to-github-hosted).


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
