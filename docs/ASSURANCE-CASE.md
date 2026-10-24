# Assurance case

[Product overview](../README.md) · [Development](DEVELOPMENT.md) · [Verifying releases](VERIFYING-RELEASES.md)

This is the argument that kyPulse meets its security requirements: what it protects, from whom,
where the trust boundaries are, and why the design and the code hold at each one. The detailed
contracts live in [AGENTS.md](../AGENTS.md) and the `AGENTS.md` beside each package; this page
ties them together and points at the evidence. Where a claim names code, the path is relative to
the repository root. Vulnerability reports go through the Busnes-app
[security policy](https://github.com/Busnes-app/.github/blob/main/SECURITY.md) (the repository's
**Security** tab, **Report a vulnerability**).

## What kyPulse must protect

kyPulse watches other apps: it polls their health endpoints, reads container state and logs from
KyYard, accepts logs from paired senders, and sends webhook alerts. It never changes the apps it
watches. Its security requirements:

1. **Only the right person signs in.** Passwords and sessions cannot be guessed, replayed or
   recovered from storage.
2. **A viewer cannot change anything.** Every write, Backup, collected Logs and imported Activity
   are admin-only, enforced by the server, not the UI.
3. **A log sender can only send logs.** A source token authenticates log ingestion for its own
   source and nothing else, and revoking it ends ingestion.
4. **Data from watched apps, KyYard and senders is untrusted.** A hostile health response, log
   batch or KyYard reply cannot crash the server, exhaust it, or run script in an admin's browser.
5. **The server cannot be turned against the host it runs on** through the URLs an admin enters
   (health targets, webhook, KyYard).
6. **Secrets at rest stay secret** from anyone who copies the database without the key files.
7. **The audit trail is tamper-evident**, and no secret reaches a log line or an audit row.

## Threat model

| Attacker | Can | Wants |
|---|---|---|
| Anonymous network client | Send any request to the published port | Take over the admin account, exhaust resources |
| Signed-in viewer | Use a viewer session and the read APIs | Change targets or the webhook, read collected logs |
| Holder of a sender token | Call the ingest endpoint | Read data, act as another source, flood storage |
| Compromised watched app or KyYard | Answer polls and pulls with anything | Crash or stall kyPulse, inject script into the UI, hide an outage |
| Admin-entered URL (by mistake or by a compromised admin session) | Point polls or webhooks anywhere | Reach the host's loopback services or a cloud metadata endpoint |
| Someone with a copy of the data volume or a backup | Read the database file | Recover the webhook, KyYard and KyRecovery tokens, or passwords |
| Network attacker between browser and proxy | Read and alter traffic | Steal a session |

Out of scope: an attacker who already runs code on the host or holds the data volume **and** the
key files (they are the server), and a malicious administrator. kyPulse serves plain HTTP and
publishes on loopback by default; TLS is the reverse proxy's job.

## Trust boundaries

1. **Browser → reverse proxy → kyPulse.** The client address comes from one helper,
   `auth.ClientIP` (`internal/auth/clientip.go`): `X-Forwarded-For` is read right to left and only
   while the hop is in `KYPULSE_TRUSTED_PROXIES`; otherwise the TCP peer is used. Rate limits and
   session binding both use it. Compose publishes on `127.0.0.1` unless `KYPULSE_BIND` says
   otherwise (`docker-compose.yml`).
2. **kyPulse → outbound targets** (health polls, webhook, KyYard, KyRecovery). Every HTTP call to
   another service goes through `internal/egress`: loopback, link-local (which includes
   `169.254.169.254`), unspecified, multicast, documentation, reserved and two named metadata
   addresses are refused on every resolved IP at dial time, redirects are refused, and bodies are
   capped. The one other `http.Client` in the module, in `internal/sender/docker.go`, dials only a
   local Docker Unix socket.
3. **Senders and KyYard → kyPulse.** Log batches arrive with a source Bearer token, are capped at
   1 MiB and validated as a whole before anything is stored (`internal/ingest`). KyYard replies are
   parsed under size caps (`internal/kyyard`). Stored text is sanitised for display
   (`logstore.Display`) and rendered as text.
4. **kyPulse → its data volume.** Secrets are sealed or hashed before they reach the database; the
   encryption key and the audit key live in separate files (`encryption.key` in the data
   directory, the audit key under `/app/secrets` in the image).
5. **Viewer → admin.** Every privileged route is registered wrapped in `s.requireAdmin` in
   `internal/api/server.go`, so the trust level of every route can be read in one place.

## Secure design principles applied

- **Least privilege.** Two roles; the store refuses any other. A source token can only ingest. The
  container runs as uid 10001, and Compose runs it read-only with all capabilities dropped and
  `no-new-privileges` (`Dockerfile`, `docker-compose.yml`). kyPulse is read-only towards the apps
  it watches.
- **Fail-safe defaults.** Forwarded headers are ignored until a proxy is named. Loopback and
  link-local targets stay refused even when plain HTTP is allowed. Cookies are `Secure` by default
  in production or behind an `https://` app URL. A key of the wrong length is a startup error.
- **Complete mediation.** The session is looked up and the account's status checked on every
  request; an inactive account's session is deleted when it is presented.
- **Economy of mechanism.** One outbound client, one client-address helper, one audit writer.
  Password hashing, the audit chain and the sealing primitive come from the suite's shared
  `ky-primitives` module instead of being reimplemented here.
- **Separation of privilege.** Monitoring reads need a session; writes need the admin role and a
  CSRF token; sender ingestion needs a separate token that grants no session.
- **Open design.** Only published algorithms: Argon2id, AES-256-GCM, HKDF and HMAC with SHA-256.
  The code and this argument are public.

## Common weaknesses and how they are countered

| Weakness | Counter | Where |
|---|---|---|
| Credential guessing and stuffing | Argon2id (64 MiB, 3 passes, 4 lanes), 12-character minimum, per-IP limits plus 10 wrong passwords per account per 15 minutes; unknown users cost the same password verification and get the same answer | `ky-primitives/password`, `internal/auth/policy.go`, `internal/api/auth_handlers.go` |
| Session theft or fixation | 32 random bytes per session, only the SHA-256 stored; `HttpOnly`, `SameSite=Lax` and `Secure` cookies; the user's sessions and MFA challenges deleted on password change | `internal/auth/session.go`, `internal/store/sqlstore.go` (`revokePasswordGrants`) |
| CSRF | Every unsafe request with a session cookie must echo the `ky_csrf` cookie in `X-CSRF-Token` (constant-time compare). The exempt routes, listed by exact path, are login and the two MFA sign-in answers (`/api/auth/mfa/totp`, `/api/auth/mfa/recovery-code`), which require a JSON body, and the token- or signature-authenticated ingest and directory routes; the TOTP enrol, confirm and disable session routes are not exempt | `internal/api/server.go` (`csrfExempt`), `auth.ValidateCSRF`, `internal/api/mfa_enroll_test.go` |
| Broken access control | Admin routes wrapped in `requireAdmin`; tests pin the per-role answer of every privileged route | `internal/api/server.go`, `internal/api/authz_test.go`, `TestViewerCannotWriteMonitoring` |
| Second-factor bypass or takeover | A local account enrols TOTP only by confirming a valid current code for the pending secret and its password; the pending write is a compare-and-swap on `totp_enabled = false`, so enrolment cannot replace an enabled secret and is refused (409) while TOTP is on. Turning it off needs the password and a current TOTP or unused recovery code. Enable and disable apply only under the password the request checked and while the calling session still exists, and revoke every other session and all MFA challenges and chain an audit row in one transaction. A session is created only while the account still has the password hash and TOTP state its sign-in checked, so a sign-in racing an enable cannot keep a password-only session; `kypulse init-admin` turns TOTP off for the admin it resets. Each accepted TOTP counter is spent once; confirm and disable have per-account attempt limits | `internal/api/mfa_handlers.go`, `internal/store/sqlstore.go` (`totpTransition`), `internal/api/mfa_enroll_test.go`, `internal/store/totp_test.go` |
| Token misuse by a sender | Source tokens stored as SHA-256 hashes; exactly one well-formed `Authorization: Bearer` header; source identity and target come from the token, never the request body | `internal/api/sources.go`, `internal/api/logs.go` |
| SQL injection | `database/sql` with `?` parameters for every value; where a statement is assembled, its table and column names come from fixed lists in the code (`log_lines`/`activity`, `sessions`/`mfa_challenges`, the filter columns) | `internal/store` (`logs.go`, `activity.go`, `targets.go`, `sqlstore.go`) |
| Cross-site scripting | React escaping, no `dangerouslySetInnerHTML` anywhere in `web/src`; collected text passed through `logstore.Display`; a CSP with `script-src 'self'` and `frame-ancestors 'none'`, plus `X-Frame-Options: DENY` | `web/src`, `internal/logstore/parse.go`, `internal/api/server.go` |
| Server-side request forgery | One guarded outbound client, destination checked at dial time on every resolved address, no redirects | `internal/egress/egress.go` |
| Resource exhaustion from untrusted input | Body caps on every JSON route and 1 MiB on ingest, per-client and per-source ingest quotas, seven-day retention and a byte budget for collected logs | `internal/api`, `internal/ingest/batch.go`, `internal/store/logs.go` |
| Parser bugs on hostile input | Go native fuzz targets for the ingest decoder, health-response normaliser, KyYard log parser and display sanitiser, run in CI | `internal/{ingest,poller,kyyard,logstore}/fuzz_test.go` |
| Secrets at rest | Webhook, KyYard and KyRecovery tokens sealed with AES-256-GCM under per-purpose keys derived from `encryption.key` by HKDF; TOTP secrets sealed with AES-256-GCM under `encryption.key` itself; passwords Argon2id; session and source tokens hashed | `internal/monitor/webhook.go`, `internal/kyyard/pairing.go`, `internal/backup/settings.go` |
| Audit tampering | A keyed hash chain over every audit row; a broken chain turns `/healthz` `degraded` with reason `chain_broken` | `internal/store/audit.go`, `ky-primitives/auditchain`, `internal/api/server.go` |
| Secrets in logs | The generated admin password is written to a `0600` file, never logged; webhook URLs, pairing codes and tokens are kept out of audit rows | `cmd/server/bootstrap.go`, [internal/api/AGENTS.md](../internal/api/AGENTS.md) |
| Vulnerable dependencies | Dependabot, `govulncheck` and `npm audit --audit-level=high` in CI; a daily dependency release ships the updates | `.github/dependabot.yml`, `.github/workflows/` |
| Supply-chain tampering | Every GitHub Action pinned by commit; base images pinned by digest; release images carry a build-provenance attestation checked before `:stable` moves; the binary rebuilds to the same bytes | [VERIFYING-RELEASES.md](VERIFYING-RELEASES.md) |

## Residual risk

- kyPulse has no step-up re-authentication; admin-only access plus the per-route tests stand in
  for it on destructive backup routes ([internal/api/AGENTS.md](../internal/api/AGENTS.md)).
- There is no operator reset of a viewer's second factor: a viewer who loses the
  authenticator and the recovery codes needs an admin to recreate the account. `kypulse
  init-admin` turns TOTP off for the admin it resets.
- TOTP secrets are sealed under `encryption.key` directly rather than a per-purpose derived
  key: that is the format the sign-in path opens and restored capsules carry.
- Rate limits are process-local, which suits the single-instance deployment kyPulse is built for.
- Health targets may use plain HTTP, and `KYPULSE_ALERT_ALLOW_HTTP` and `KYPULSE_KYYARD_ALLOW_HTTP`
  admit plain HTTP for the webhook and KyYard. The server logs a warning at startup when either is
  on.
- No Ky product has had an independent security audit.

## Evidence

- **Tests:** the Go suite runs under the race detector on SQLite and again on PostgreSQL in CI,
  plus Vitest, Playwright browser regressions against the production server, and an end-to-end
  smoke test (`scripts/smoke-test.sh`) that checks the same boundaries against a running binary.
  The Go statement coverage is printed in each CI run's summary.
- **Static analysis:** CodeQL (GitHub default setup: Go, JavaScript/TypeScript, Actions) and
  `go vet`, `govulncheck`, the TypeScript compiler in `strict` mode, and `shellcheck` for scripts.
- **Dynamic analysis:** the race detector, four bounded fuzz runs on every CI run, and the Docker
  job, which runs the image read-only with all capabilities dropped and checks it is not root.
- **Releases:** attested images and a repeatable binary ([VERIFYING-RELEASES.md](VERIFYING-RELEASES.md)).

This case is updated when a trust boundary or a security requirement changes. A pull request that
moves one must update this page in the same change.
