# UI verification

kyPulse screens: alert bar, Status, Alerts, app detail, webhook, admin Logs/Activity and KyYard collection status.

## Capture conditions

Captured by `web/browser/monitor.spec.mjs` against the real Go server with disposable SQLite
data, production CSP and real login, at 1280×900 and 390×900 in Busnes Light and Dark. The
watched app is a fake `ky.health/1` server on the runner's LAN address; the webhook is the
same server answering 500.

## Checks

`npm test` (vitest: router, monitor helpers and client, AlertBar, Status, Alerts, AppDetail,
WebhookForm, KyYardCard, Backup, ChangePassword) and `npm run test:browser` (shell spec plus
the monitor spec: empty install, add app, down after three polls, red bar linking to the
detail page, checks, silence until fixed, Alerts table and silences, webhook saved, test send
failing, "Alerts not being delivered", recovery to green with the silence cleared, delete back
to Status, no horizontal overflow). Viewer-role rendering of Status, Alerts and the detail page
is covered by component tests; the `#/backup` redirect and the Settings webhook gate rely on
the server's 403s (`internal/api` authorisation tests).

`logs.spec.mjs` also runs a fake KyYard on the private interface and exercises real pairing,
linked-container collection on the 60-second worker, and independent inventory/log/audit
status on Logs, Activity and Settings. The fake's old array audit API remains visibly
unsupported while inventory and logs succeed. The same case verifies source pairing,
filters, pagination, offline retry, plain-text rendering, burst hints and real viewer denial.
Component tests cover stale ages and separate errors; container facts/suggestion variants
retain their component coverage. This is not a real KyYard deployment or Docker-engine soak.

Logs and Activity screenshots are saved per project under ignored `web/test-results/`;
existing committed screenshots below cover the monitoring screens.

## Screenshots

| | |
| --- | --- |
| ![Status, one app down, light desktop](docs/status-down-light-desktop.png) | ![Status, all healthy, dark mobile](docs/status-ok-dark-mobile.png) |
| ![App detail while down, light desktop](docs/detail-down-light-desktop.png) | ![Alerts, dark desktop](docs/alerts-dark-desktop.png) |

## Reproduce

Build the frontend, run `go build -o .browser/server ./cmd/server` at the repo root, then
`cd web && npx playwright install chromium && npm run test:browser`. The monitor spec skips on
a machine with no private IPv4 interface.
