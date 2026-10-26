# kyPulse server

## Purpose

Unified health and logging for the Ky suite: polls every app's `/healthz`, pulls container
state and logs from KyYard, accepts logs from paired senders, shows status, alerts, activity
and logs, and sends outbound webhook alerts. Read-only towards the apps it watches.

Design authority: `docs/superpowers/specs/2026-09-26-kypulse-design.md`. Plans live in
`docs/superpowers/plans/`.

## Ownership

- `docker-compose.yml`: publishes on `${KYPULSE_BIND:-127.0.0.1}`; settings an operator sets in `.env` are forwarded with empty meaning the server default (`cmd/server/deploy_test.go` holds both).
- `cmd/server/`: process entry, CLI subcommands (`init-admin`, `backup-drill`,
  `export-capsule`, `deposit`, `restore`, `audit-verify`, `version`, `apply-setup`), the log bridge, the
  backup scheduler, KyYard inventory loop and separately joined log/audit collection worker.
- `cmd/kypulse-send/`: sender pairing CLI and file/stdin/Docker readers.
- `internal/`: one package per concern; each has its own AGENTS.md.
- `internal/egress`, `internal/poller`, `internal/alerts`, `internal/notify`, `internal/monitor`,
  `internal/kyyard`: the monitoring backend; each has its own AGENTS.md.
- `web/`: the React PWA, embedded into the binary from `web/dist`.
- `scripts/`: smoke test.
- `docs/RESTORE.md`: restore runbook.
- `docs/STRONG-CRYPTO.md`, `docker-compose.strong-crypto.yml` and
  `docs/check-strong-crypto-compose.py`: supported key/TLS configuration,
  migration and synthetic deployment-preservation proof.

## Local Contracts

- Container-local installer setup uses the shared `ky.product-setup/1` envelope and private runtime-owned Unix socket, never a public setup route. Discover capabilities before using it; read [installer setup](docs/INSTALLER_SETUP.md). New identities and monitoring targets/sources are explicitly owned; foreign objects are not adopted. Saved runtime configuration activates at graceful restart; monitoring rows activate live. Read-only inspection verifies ownership and current state. KyIdentity offboarding arrives as a syncauth v1 signed directory delivery that only sets owned identities' availability and ends their sessions. The adapter supports local backup configuration; recovery pairing and KyYard enrollment remain explicit follow-on gates.

- Module `github.com/Busnes-app/kypulse-server`; binary `kypulse`; image
  `ghcr.io/busnes-app/kypulse-server`; application settings are `KYPULSE_*` except
  suite-wide `KY_LOG_LEVEL`. The documented Go runtime profile uses `GODEBUG`.
- Two roles, `admin` and `viewer`; the store refuses anything else and identity providers map
  through `sso.RoleFor`. Every admin-only route answers 403 to a viewer. A viewer sees Status,
  Alerts, app detail pages and Settings & DB (read-only, no webhook form); Backup and every
  write are admin-only.
- Every process line is a JSON record on stderr through `ky-primitives/logging`; there is no
  log file and no log socket.
- Sender CLI SIGINT/SIGTERM and reader failures allow a five-second delivery drain, then
  cancel HTTP/retry waits and join readers. Clean EOF alone continues normal retries.
  Reader failures surface after draining; unacknowledged positions remain unchanged.
- Collected Logs and imported Activity are admin-only, including app-detail queries.
  Sender Bearer tokens authenticate only log ingestion; source identity and watched-target
  binding come from the claimed token, never a request body. Revocation ends ingestion.
  Optional ordered ingest receipts acknowledge the latest exact batch without duplicate rows;
  authenticated non-browser OPTIONS proves that capability and this source's identity before
  sending. Browser preflight and other source-token privileges remain separate.
  Administrator-granted offboarding sources atomically create failure Alerts on their
  watched target; the existing sender consumes pending notifications across restart.
  Sender ordering and wire rules live in `internal/api/AGENTS.md`. Receipts survive recovery
  snapshots alongside source identities, while collected payloads are excluded.
- Collected logs and imported activity share seven-day receive-time retention and the
  `KYPULSE_LOG_MAX_BYTES` logical payload budget. SQLite capsules exclude both tables and
  pending pairing codes and collection cursors; source identities and token hashes remain.
- The bootstrap admin's generated password goes only to `<DataDir>/initial-admin-password`
  (0600, `cmd/server/bootstrap.go`), never a log line; a file that cannot be written fails
  startup. `KYPULSE_ADMIN_PASSWORD` still chooses it and writes no file.
- `GET /healthz` is public and is what an external monitor watches; kyPulse does not monitor
  itself.
- The audit trail is a keyed hash chain (`internal/store/audit.go`); a log the store cannot
  place refuses to start.
- `web/dist` is committed after every frontend change; CI diffs it. `web/src/ky-ui/` is
  vendored and hash-pinned; never hand-edit it.
- No device pairing, no SCIM. Local login with MFA, SSO (KySignOn, OIDC, SAML metadata) and
  KyRecovery backup are kept from the scaffold. Local accounts enrol and remove TOTP
  themselves in Settings (`internal/api/AGENTS.md`); identity providers own SSO accounts' MFA.

## Work Guidance

- Follow the suite root's engineering principles and the KyRecovery integration contract.
- Business rules (health normalisation, alert transitions, retention) are pure functions the
  handlers and scheduler call.
- Monitoring, KyYard, webhook and sender network HTTP goes through `internal/egress`:
  private/LAN targets allowed, loopback and link-local refused, no redirects.
  OIDC and recovery retain their maintained libraries' transport integration;
  SSO and the shared recovery client own their additional policy. Local Docker
  reads use a Unix socket. The process TLS profile applies across Go HTTPS clients.
- Preserve original session/encryption/audit keys, source authority, sender state
  and recovery custody when enabling the profile; follow `docs/STRONG-CRYPTO.md`.

## Verification

- `make ci`: tidy check, gofmt, vet, race tests, frontend tests, smoke test.
- Repeat full race suites with `GODEBUG=fips140=on` on SQLite and PostgreSQL;
  CI covers both within existing required jobs. The synthetic Compose checker
  verifies base/build/LAN-DNS/static-IP combinations without starting services.
- `.github/workflows/ci.yml` additionally builds the release binary twice from different directories and requires identical bytes (the Dockerfile's `-trimpath` makes that hold), and runs Postgres tests, Playwright browser regressions,
  govulncheck, the Docker image smoke and the image-coordinate check.
- The version has one source: `internal/appversion/VERSION` (`X.Y.Z`, the release line; `web/package.json` has no version). Go code calls `appversion.String()`; a release image build sets the tag's full version through `-ldflags -X …/internal/appversion.release`, so images report `X.Y.Z` or `X.Y.Z.N` and source builds report the line.
- Publishing a GitHub release `vX.Y.Z` or `vX.Y.Z.N` runs `release-image.yml` (a dispatch on a published release's tag runs it too). It refuses a malformed tag, a tag whose `X.Y.Z` disagrees with `internal/appversion/VERSION`, and any commit on which a ci.yml job in its `REQUIRED` list (all seven, by their `name:`) did not pass. It then builds `:X.Y.Z[.N]` with provenance and SBOM, attests and verifies it (identity `release-image.yml@refs/tags/<tag>`), and moves `:stable` to that digest only for the highest non-prerelease. It never moves `:latest`; `master` CI owns that. A job added to ci.yml must be added to `REQUIRED` too.
- `dependency-release.yml` runs daily at 23:00 UTC: if Dependabot commits landed on `master` since the newest release on the current line and the push CI run on `master`'s tip succeeded, it publishes the next `vX.Y.Z.N` (one per day however many PRs merged) and dispatches `release-image.yml` on it, since a `GITHUB_TOKEN` release raises no release event. Run by hand, it releases `master` without the Dependabot condition. It waits while the line in `VERSION` has no `vX.Y.Z` release: bump `VERSION` and publish `vX.Y.Z` by hand for a new line.

## Child DOX Index

- `internal/api/AGENTS.md`: HTTP routes, auth wrappers, backup handlers, `/healthz`, SPA hosting.
- `internal/auth/AGENTS.md`: password policy, TOTP, recovery codes, sessions, PoW CAPTCHA.
- `internal/backup/AGENTS.md`: KyRecovery adapter over `ky-primitives/recoveryclient`.
- `internal/config/AGENTS.md`: `KYPULSE_*` loading, defaults, validation.
- `internal/crypto/AGENTS.md`: AES-GCM, HMAC, randomness.
- `internal/sso/AGENTS.md`: KySignOn, OIDC, SAML SP, `RoleFor`.
- `internal/store/AGENTS.md`: SQLite/Postgres DAL, migrations, roles.
- `internal/testdb/AGENTS.md`: isolated test databases.
- `internal/logstore/AGENTS.md`: bounded log parsing, display sanitization and activity burst rules.
- `internal/ingest/AGENTS.md`: bounded NDJSON batch validation.
- `internal/sender/AGENTS.md`: sender pairing, owned state, bounded delivery and checkpoints.
- `internal/egress/AGENTS.md`: the one outbound HTTP client.
- `internal/poller/AGENTS.md`: health polling and response normalisation.
- `internal/alerts/AGENTS.md`: alert thresholds, transitions, reminders, silences.
- `internal/notify/AGENTS.md`: webhook presets and delivery retries.
- `internal/monitor/AGENTS.md`: glues polling, alerts and delivery to the store.
- `internal/kyyard/AGENTS.md`: KyYard pairing, inventory/sample snapshots, durable log/audit collection.
- `web/AGENTS.md`: PWA, themes, ky-ui vendoring, browser regressions.

## Operator documentation

`INSTALL.md` is the root-owned standalone entry point. When installation prerequisites, bootstrap, persistence or verification change, update it together with `README.md` and the linked operator guides.


## README and topic guides

Keep `README.md` as the concise product overview and navigation. Detailed procedures live in `docs/RUNNING.md`, `docs/LOG-COLLECTION.md`, `docs/MONITORING.md`, `docs/CONFIGURATION.md`, `docs/OPERATIONS.md`, `docs/DEVELOPMENT.md` (including the coding standard), `docs/VERIFYING-RELEASES.md`. `docs/ASSURANCE-CASE.md` is the security argument: a change that moves a trust boundary or a security requirement updates it in the same change. Update the owning topic guide and preserve README links when a procedure changes. Commands in these guides run from the repository root.
