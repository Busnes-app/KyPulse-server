# kyPulse server

## Purpose

Unified health and logging for the Ky suite: polls every app's `/healthz`, pulls container
state and logs from KyYard, accepts logs from paired senders, shows status, alerts, activity
and logs, and sends outbound webhook alerts. Read-only towards the apps it watches.

Design authority: `docs/superpowers/specs/2026-09-26-kypulse-design.md`. Plans live in
`docs/superpowers/plans/`.

## Ownership

- `cmd/server/`: process entry, CLI subcommands (`init-admin`, `backup-drill`,
  `export-capsule`, `deposit`, `restore`, `version`), the log bridge, the backup scheduler.
- `internal/`: one package per concern; each has its own AGENTS.md.
- `web/`: the React PWA, embedded into the binary from `web/dist`.
- `scripts/`: smoke test.
- `docs/RESTORE.md`: restore runbook.

## Local Contracts

- Module `github.com/Busnes-app/kypulse-server`; binary `kypulse`; image
  `ghcr.io/busnes-app/kypulse-server`; every setting is `KYPULSE_*` except the suite-wide
  `KY_LOG_LEVEL`.
- Two roles, `admin` and `viewer`; the store refuses anything else and identity providers map
  through `sso.RoleFor`. Viewers see Status and Alerts; admins see everything.
- Every process line is a JSON record on stderr through `ky-primitives/logging`; there is no
  log file and no log socket.
- `GET /healthz` is public and is what an external monitor watches; kyPulse does not monitor
  itself.
- `web/dist` is committed after every frontend change; CI diffs it. `web/src/ky-ui/` is
  vendored and hash-pinned; never hand-edit it.
- No device pairing, no SCIM. Local login with MFA, SSO (KySignOn, OIDC, SAML metadata) and
  KyRecovery backup are kept from the scaffold.

## Work Guidance

- Follow the suite root's engineering principles and the KyRecovery integration contract.
- Business rules (health normalisation, alert transitions, retention) are pure functions the
  handlers and scheduler call.
- Outbound HTTP goes through `internal/egress` (planned in step 2b): private and LAN targets
  allowed, loopback and link-local refused, no redirects.

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
- `web/AGENTS.md`: PWA, themes, ky-ui vendoring, browser regressions.
