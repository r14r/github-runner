# Technische Entscheidungen

## D-001 – Eigenständiger Runner-Code
**Datum:** 2026-10-25 · **Status:** Accepted

`r14r/github-runner` besitzt Skripte für Provisionierung, Registrierung, Dienststeuerung und Wartung. BookBuilder enthält ausschließlich eigene GitHub Actions Workflows sowie einen explizit gepinnten Checkout für den read-only Host-Doctor. Duplizierte Verwaltungsskripte sind nicht zulässig.

## D-002 – Sichere lokale Migration
**Datum:** 2026-10-25 · **Status:** Accepted

Private Registrierungsdateien `.runner`, `.credentials`, `_work` und `_diag` bleiben außerhalb Git unter `.local/`. Migration aus BookBuilder erfolgt nur auf ausdrücklichen `just migrate-from-bookbuilder <path>` Aufruf. Alter launchd-Dienst wird vor dem Umzug deinstalliert und im neuen Pfad installiert; Runner-Identität wird beibehalten. Quelle und Ziel werden niemals stillschweigend zusammengeführt. Cloud-Sync unabhängig von `.gitignore` ausschließen.

## D-003 – Zielrepository konfigurierbar, Voraussetzungen profiliert
**Datum:** 2026-10-25 · **Status:** Accepted

`RUNNER_REPO` und `RUNNER_LABEL` sind anpassbar; voreingestellt ist das ursprüngliche BookBuilder-Repository und das Label `bookbuilder`. Der Host-Doctor ist weiterhin ein versioniertes BookBuilder-Profil (Python 3.12, PHP 8.2, Java 21, Typst 0.15.1). Keine pauschale Kompatibilität mit beliebigen Anwendungsprojekten behaupten.
