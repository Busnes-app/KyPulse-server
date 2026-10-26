# kyPulse

[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15931/badge)](https://www.bestpractices.dev/projects/15931)

**Start here:** [Standalone installation](INSTALL.md). Local operator sign-in works without KyIdentity; the first `admin` password is written to `initial-admin-password` in the data directory, never to the log. Add health endpoints to monitor any compatible service; KyYard integration is optional for container information and log collection. Alerts require a configured, reachable webhook.

Unified health and logging for the Busnes.app Ky suite. kyPulse polls every Ky app's
`/healthz`, reads container state from KyYard, shows what is broken first, and sends an
outbound webhook when an app goes down or degrades. It is read-only towards the apps it
watches: it observes and reports, and never changes them.

Design: [`docs/superpowers/specs/2026-09-26-kypulse-design.md`](docs/superpowers/specs/2026-09-26-kypulse-design.md).

## What works today

| Area | State |
|---|---|
| Health polling, alert state machine, webhook alerts | done |
| Status, Alerts, app detail, alert bar, webhook form | done |
| KyYard pairing, container facts, stale marking | done |
| kyPulse's own tamper-evident audit trail | done |
| Source pairing, log ingest, admin log and activity APIs, retention | done |
| `kypulse-send` Linux sender | done |
| Admin Logs/Activity screens and KyYard log/audit collection | done; audit requires the KyYard cursor API |

## Screens

- **Alert bar**, on every page. Red with one line per down or degraded app; green with the
  healthy count and the last check; a separate line when webhook delivery keeps failing or
  KyYard data is stale.
- **Status**, the home tab. Every watched app as a tile, broken first. Admins add apps here.
- **Alerts**. State changes and hourly reminders with their delivery result, and current
  silences.
- **App detail** (`#/apps/<id>`, the link webhook messages carry). Current state with the
  exact failing request, the checks from the last response, alert history, KyYard container
  facts, and, for admins, silence (1 h, 8 h, until fixed), edit and delete.
- **Logs** (admins). Filtered log timeline, raw text, source pairing/revocation and collection status.
- **Activity** (admins). Imported audit events, filters, failed-sign-in burst hints and collection status.
- **Settings & DB**. Two-factor authentication for local accounts, the alert webhook, KyYard pairing/collector ages (admins), theme, database driver.
- **Backup** (admins). KyRecovery pairing, schedule, local copies, restore drill.

Roles are `admin` and `viewer`. A viewer sees Status, Alerts, app detail and a read-only
Settings & DB; every write and Backup are admin-only, and the server enforces it.
Collected Logs and imported Activity, including app-detail log queries, are admin-only.

## License

See [`LICENSE.txt`](LICENSE.txt).

Container-local installer setup and its current acceptance limits are documented in
[installer setup](docs/INSTALLER_SETUP.md).

## Documentation

- [Running](docs/RUNNING.md) — Quick start. <a id="quick-start"></a><a id="without-root-named-volumes"></a><a id="upgrading-port-8080-now-on-loopback"></a><a id="upgrading-to-a-non-root-container"></a><a id="audit-key"></a>
- [Log collection](docs/LOG-COLLECTION.md) — Sending logs. <a id="sending-logs"></a><a id="linux-sender"></a>
- [Monitoring](docs/MONITORING.md) — Watching apps, Alert webhook, KyYard. <a id="watching-apps"></a><a id="alert-webhook"></a><a id="kyyard"></a>
- [Configuration](docs/CONFIGURATION.md) — Configuration, Compose overlays. <a id="configuration"></a><a id="compose-overlays"></a>
- [Operations](docs/OPERATIONS.md) — Backup and restore, Command line. <a id="backup-and-restore"></a><a id="command-line"></a>
- [Development](docs/DEVELOPMENT.md) — API, Develop. <a id="api"></a><a id="develop"></a>
- [Verifying releases](docs/VERIFYING-RELEASES.md) — Check a release image's signature and source commit, and rebuild its binary to the same bytes.
- [Assurance case](docs/ASSURANCE-CASE.md) — Threat model, trust boundaries, and why the security requirements are met.
