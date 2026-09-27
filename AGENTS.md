# kyPulse server

## Purpose

Unified health and logging for the Ky suite: polls every app's `/healthz`, pulls container
state and logs from KyYard, accepts logs from paired senders, shows status, alerts, activity
and logs, and sends outbound webhook alerts. Read-only towards the apps it watches.

Design authority: `docs/superpowers/specs/2026-09-26-kypulse-design.md`. Plans live in
`docs/superpowers/plans/`.

## Ownership

- `cmd/server/`: process entry, CLI subcommands (`init-admin`, `backup-drill`,
  `export-capsule`, `deposit`, `restore`, `audit-verify`, `version`), the log bridge, the
  backup scheduler.
- `internal/`: one package per concern; each has its own AGENTS.md.
- `internal/egress`, `internal/poller`, `internal/alerts`, `internal/notify`, `internal/monitor`:
  the monitoring backend; each has its own AGENTS.md.
- `web/`: the React PWA, embedded into the binary from `web/dist`.
- `scripts/`: smoke test.
- `docs/RESTORE.md`: restore runbook.

## Local Contracts

- Module `github.com/Busnes-app/kypulse-server`; binary `kypulse`; image
  `ghcr.io/busnes-app/kypulse-server`; every setting is `KYPULSE_*` except the suite-wide
  `KY_LOG_LEVEL`.
- Two roles, `admin` and `viewer`; the store refuses anything else and identity providers map
  through `sso.RoleFor`. Every admin-only route answers 403 to a viewer. Today a viewer sees
  Overview and Settings & DB (read-only) and no Backup tab; from step 2c viewers also get
  Status and Alerts, and everything else stays admin-only.
- Every process line is a JSON record on stderr through `ky-primitives/logging`; there is no
  log file and no log socket.
- `GET /healthz` is public and is what an external monitor watches; kyPulse does not monitor
  itself.
- The audit trail is a keyed hash chain (`internal/store/audit.go`); a log the store cannot
  place refuses to start.
- `web/dist` is committed after every frontend change; CI diffs it. `web/src/ky-ui/` is
  vendored and hash-pinned; never hand-edit it.
- No device pairing, no SCIM. Local login with MFA, SSO (KySignOn, OIDC, SAML metadata) and
  KyRecovery backup are kept from the scaffold.

## Work Guidance

- Follow the suite root's engineering principles and the KyRecovery integration contract.
- Business rules (health normalisation, alert transitions, retention) are pure functions the
  handlers and scheduler call.
- Outbound HTTP goes through `internal/egress`: private and LAN targets allowed, loopback and
  link-local refused, no redirects.

## Verification

- `make ci`: tidy check, gofmt, vet, race tests, frontend tests, smoke test.
- `.github/workflows/ci.yml` additionally runs Postgres tests, Playwright browser regressions,
  govulncheck, the Docker image smoke and the image-coordinate check.

## Child DOX Index

- `internal/api/AGENTS.md`: HTTP routes, auth wrappers, backup handlers, `/healthz`, SPA hosting.
- `internal/auth/AGENTS.md`: password policy, TOTP, recovery codes, sessions, PoW CAPTCHA.
- `internal/backup/AGENTS.md`: KyRecovery adapter over `ky-primitives/recoveryclient`.
- `internal/config/AGENTS.md`: `KYPULSE_*` loading, defaults, validation.
- `internal/crypto/AGENTS.md`: AES-GCM, HMAC, randomness.
- `internal/sso/AGENTS.md`: KySignOn, OIDC, SAML SP, `RoleFor`.
- `internal/store/AGENTS.md`: SQLite/Postgres DAL, migrations, roles.
- `internal/testdb/AGENTS.md`: isolated test databases.
- `internal/egress/AGENTS.md`: the one outbound HTTP client.
- `internal/poller/AGENTS.md`: health polling and response normalisation.
- `internal/alerts/AGENTS.md`: alert thresholds, transitions, reminders, silences.
- `internal/notify/AGENTS.md`: webhook presets and delivery retries.
- `internal/monitor/AGENTS.md`: glues polling, alerts and delivery to the store.
- `web/AGENTS.md`: PWA, themes, ky-ui vendoring, browser regressions.
