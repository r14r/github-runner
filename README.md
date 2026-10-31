# github-runner

Eigenständige Verwaltung von GitHub Actions Self-hosted Runners für macOS (Intel/Apple Silicon) und Ubuntu 24.04 x64. Dieses Repository enthält **keine** Runner-Registrierungen, Tokens oder lokalen Workspaces.

Die erste unterstützte Provisionierungs-Konfiguration entspricht den Anforderungen von [BookBuilder](https://github.com/r14r/BookBuilder-with-typst) (Python 3.12, PHP 8.2, Java 21, Typst 0.15.1, FFmpeg/Fonts). Andere Projekte können die Repo-Auswahl und Labels über Umgebungsvariablen festlegen; das Systemprofil bleibt zunächst BookBuilder-spezifisch.

## Installation

```bash
gh repo clone r14r/github-runner
cd github-runner

# Optional bei einem anderen Repository:
export RUNNER_REPO="r14r/BookBuilder-with-typst"
export RUNNER_LABEL="bookbuilder"

just install
just doctor
just status
just monitor
```

Für die erstmalige Registrierung benötigt `gh` einen angemeldeten Account mit Runner-Administrationsrechten im Ziel-Repository. Unter macOS wird Homebrew vorausgesetzt; auf Ubuntu erfolgt die Host-Provisionierung mit sudo, während die Jobs als nichtprivilegierter Runner-Benutzer laufen.

## Befehle

| Rezept | Wirkung |
| --- | --- |
| `just install` | OS-spezifische Provisionierung, Registrierung, Dienststart; vorhandene Registrierung behalten |
| `just doctor` | Nur lesende Prüfung der vorausgesetzten Laufzeitwerkzeuge |
| `just start`, `just stop`, `just status` | Dienst verwalten (launchd auf macOS, systemd auf Ubuntu) |
| `just monitor` / `just monitor 30` | Dienststatus, Runner-Status, GitHub-Läufe und Diagnoseprotokolle |
| `just repair` | macOS: veraltete Registrierung kontrolliert neu einrichten |
| `just migrate` | Alte Standardinstallation `~/.local/share/bookbuilder-actions-runner` migrieren |
| `just migrate-from-bookbuilder "/absoluter/alter/pfad"` | Runner aus BookBuilder-Checkout in dieses Repository verschieben |
| `just cleanup-preview` / `just cleanup` | `_diag/*.log` älter als 30 Tage ansehen/löschen; `just cleanup 7` für 7 Tage |
| `just actions-cancel` | Alle laufenden/queued Runs im konfigurierten Ziel-Repository abbrechen |
| `just actions-delete` | Abgeschlossene Runs dauerhaft löschen; mit Repo-Namen bestätigen |
| `just actions-clear` | Bestätigen, abbrechen, Abschluss abwarten und Ausgangs-Runs löschen |
| `just test` | Lokale Shell- und Regressionstests (pytest) |
| `just ci` | Statische und Regressionstests |

## Umzug der bestehenden BookBuilder-Installation auf dem Mac

**Nie während eines laufenden GitHub Actions Jobs ausführen.** Zuerst dieses Repository in einen lokalen, nicht synchronisierten Ordner klonen (z. B. `~/Developer/github-runner`).

```bash
cd ~/Developer/github-runner

# Den vollständigen bisherigen Pfad aus dem alten BookBuilder-Checkout angeben:
just migrate-from-bookbuilder "/Users/Shared/CLOUD/Projekte/Digital-Products/BookBuilder-with-typst/current/github-runner/.local/bookbuilder-actions-runner"

just install
just status
```

Die Migration stoppt den alten launchd-Dienst, deinstalliert dessen alte Pfadreferenz, verschiebt das komplette Runner-Verzeichnis **mit** `.runner`, `.credentials`, `_work` und `_diag` und installiert den Dienst im neuen Pfad. Bestehende GitHub-Registrierungsdaten bleiben erhalten. Falls das neue Zielverzeichnis bereits existiert, wird **nichts überschrieben**. Auf Fehler reagiert der Prozess mit einem Abbruch; keine Registrierung wird automatisch serverseitig gelöscht. `just install` verwendet den neuen Standort:

```text
~/Developer/github-runner/.local/bookbuilder-actions-runner/
```

`.local/` ist Git-ignoriert. Cloud-Synchronisationsprogramme beachten `.gitignore` jedoch nicht; Runner-Credentials niemals in iCloud/Dropbox synchronisieren und niemals `git clean -fdx` im Arbeitsverzeichnis ausführen.

## BookBuilder-Integration

Die GitHub Actions Workflows bleiben im BookBuilder-Repository. Ein Job, der den Self-hosted Runner auswählt, lädt die Prüfroutinen aus **diesem** Repository in ein gesondertes Checkout und führt `doctor.sh` aus. Diese Repo-Grenze verhindert, dass BookBuilder die Runner-Verwaltung dupliziert. Wenn man die Provisionierungsanforderungen ändert, muss die gepinnte Integration in BookBuilder bewusst aktualisiert werden.

Plattform-Anleitungen: [MACOS.md](MACOS.md), [UBUNTU.md](UBUNTU.md). Versionsverlauf: [CHANGELOG.md](CHANGELOG.md). Entscheidungshistorie: [docs/ai/DECISIONS.md](docs/ai/DECISIONS.md).

**Sicherheit:** Self-hosted Runner führen Workflow-Code mit den Rechten ihres Betriebssystemkontos aus. Für nicht vertrauenswürdige PRs einen isolierten Host verwenden; keine persönlichen Zugangsdaten, kein Docker-Socket oder passwortloses sudo an CI-Jobs geben.

## Qualitätssicherung

```bash
just ci
```

Die Regressionstests verwenden gemockte Dienste und GitHub CLI; echte Installations-/Registrierungstests auf macOS und Ubuntu bleiben eine separate Freigabebedingung.

## Multi-Repository-Runners mit Docker/Colima (v0.1.2)

Für macOS Monterey (Intel) kann ein Linux-x64-Runner in Docker/Colima laufen.
Registrierungen und Workspaces bleiben in `.local/`; die bestehenden
BookBuilder-Installer und Runner-Registrierungen werden nicht verändert.
Die Bereitstellung ist **opt-in** und führt keine Registrierung bei CI-Tests aus.

Siehe [RUNNER-EXTENSION.md](RUNNER-EXTENSION.md) und [runner/README.md](runner/README.md).

```bash
just runner-download r14r/website-builder
just runner-register r14r/website-builder
just runner-install-service r14r/website-builder
just runner-start r14r/website-builder
just runner-status r14r/website-builder
just runner-pr r14r/website-builder discover-layouts
```

`runner-pr` eröffnet standardmäßig nur einen PR, ohne ihn automatisch zu mergen.
Der Docker-Socket wird nur durch `RUNNER_DOCKER_SOCKET=1` freigegeben.
