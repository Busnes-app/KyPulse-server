# API

## Purpose
Exposes HTTP REST routes, authentication endpoints, Single Sign-On callbacks, backup restore drill handlers, monitoring routes, and static React PWA hosting.

## Ownership
Owns HTTP routing, request parsing, session cookie validation, CORS headers, and error response formatting.

## Local Contracts
- `ApplyInstaller` serves only private local IPC. It validates the candidate runtime configuration before store writes, preserves the immutable live snapshot and reports restart activation pending until the saved configuration is actually loaded. Public `GET /api/installer/network` returns only this request client address, peer trust and scheme, with no-store; it reuses authentication address resolution and trusts only the nearest configured proxy. Installer ownership rows are omitted from settings responses.
- `POST /api/installer/directory` (and a `v1=` signature on `/api/sso/kysignon/sync`) verifies KyIdentity suite-directory deliveries with `syncauth.Verify` under the installer directory secret (64 KiB body cap), parses the SCIM user and its `meta.version` revision, and hands availability to `store.ApplyInstallerDirectory` (`user.mfa_reset` is acknowledged without change): 401 bad signature, 400 malformed, 422 refused, 204 applied. Admin is `kypulse.admin` or global `admin`.
- The private bundle additionally accepts `monitoring` targets/sources. Validate source
  references, keys and the runtime self origin before reading credentials; prepare separate
  target-file token hashes before the combined store transaction. Native monitoring activates
  live and adds no restart requirement. Setup-created sources retain the same ingestion-only
  authority and optional target-bound offboarding alert grant as administrator pairing;
  public callers cannot reach the setup route or supply prepared token hashes.
- POST `/api/auth/change-password` accepts a restricted local session, current password and a different policy-valid new password. Browser CSRF and per-IP/account limits apply. Success revokes all sessions and requires sign-in again; flagged sessions get `password_change_required` on protected routes and public-only settings.
- All JSON API endpoints return structured errors `{"error": "message"}` upon failure.
- Non-API routes fall back to serving `web.Handler()` for client-side SPA routing.
- New routes are unauthenticated only by deliberate choice; privileged ones are registered wrapped in `s.requireAdmin` in `routes()`, so the trust level of every route is readable in one place.
- Backup routes and theme writes are admin-only: capsules and settings carry site data and secrets. The scaffold has no step-up; admin-only plus `TestPrivilegedEndpointsRequireAdmin` is its equivalent for every destructive backup route. Routes are registered with method patterns, and because the SPA catch-all answers any method, tests pin that a wrong method never reaches a backup handler rather than expecting 405.
- Viewer reads use `s.requireSession`; monitoring writes and the webhook use `s.requireAdmin`. The webhook token is write-only: never in `GET /api/alerts/webhook`, `/api/settings` (sealed `_enc` rows are filtered by suffix) or `/api/status`. Audit actions: `admin.target_create|update|delete|silence`, `admin.webhook_set|delete|test`; `alert.sent|send_failed` are written by `internal/monitor` with actor `system`. A webhook URL is never audited: `admin.webhook_set` records `preset`, `scheme`, `host` and `allow_http`.
- Every audit row goes through `s.audit`, which bounds both text fields and logs `audit_write_failed` when the row cannot be written.
- Caller-triggerable refusals and failures (admin-wrapper and CSRF denials, claim refusals, ingest refusals, failed login/MFA) go through `s.auditRefusal`: the first per client (IPv6 by /64), user, action and details in a 10-minute window is written; repeats are written later as one row ending `repeats=N`, flushed by the next refusal after the window closes. Past 512 live buckets, new keys share one `audit.refusal_overflow` bucket. Never write a refusal row per request.
- Login and MFA audit `auth.login`, `auth.mfa_totp`, `auth.mfa_recovery_code` with `outcome=success` or `outcome=failure reason=<code>` (`rate_limit`, `captcha`, `unknown_user`, `account_rate_limit`, `password`, `inactive`, `transaction`, `ineligible`, `code`, `replay`). Unknown, wrong-password, locked and disabled-with-wrong-password logins all cost one password verification and get the same 401 body; only a correct password learns an account is disabled (403).
- Every `/api/` response is `Cache-Control: no-store`. JSON-body auth routes (login, MFA, change-password) answer 415 without `Content-Type: application/json`, and log-source pairing/claim bodies require it, so cross-site simple-content-type forms cannot reach CSRF-exempt routes.
- A PoW solution is accepted once (`auth.PoWGuard`). SSO callback expires its PKCE and nonce cookies (both scoped to the callback path), returns a generic error while logging `sso_failed`, refuses inactive accounts before a session, and provisions only on `store.ErrNotFound`.
- Log-source routes are explicit method patterns: admin `POST /api/log-sources/pairing` (`{target_id?}` → six-digit code and expiry), public `POST /api/log-sources/claim` (`{pairing_code,name}` → one-time token and source identity), admin `GET /api/log-sources` and admin `DELETE /api/log-sources/{id}`. All responses, including errors, use `Cache-Control: no-store`; admin and session-backed claim writes use normal CSRF protection. Source Bearer tokens do not authenticate a user session. Pairing and claim bodies are one JSON object capped at 4 KiB. Claim checks a 5/min trusted-proxy-aware IP counter first; only requests it admits count toward a fixed global 30/min counter outside the evictable attempt map, so one client cannot spend the global budget. 429 carries `Retry-After`. Codes and tokens never enter audit rows. Successful claim and revoke use `s.audit`; failures and refusals use `s.auditRefusal` with reason codes. The shared admin wrapper audits source-route authentication/role denials, and the pre-route CSRF gate audits source-route CSRF denials, summarised per client as above. Successful list reads do not create audit rows.
- `POST /api/ingest/logs` accepts only source Bearer tokens and NDJSON. A 600/min per-client counter runs before the token lookup (429 `ip_rate_limit`). Authenticate before reading the body; source quotas use their own stable 60/min counter and return 429 with `Retry-After`. Require `application/x-ndjson` and identity content encoding (415 otherwise), cap the wire body at 1 MiB (413), reject malformed batches as a whole (400), and return 204 only after the batch commits using `cfg.Logs.MaxBytes` or its latest sequence receipt verifies. Invalid or revoked tokens return 401. `GET /api/logs` and `GET /api/activity` are admin-only, return sanitized text in descending ID pages (`items`, `next_before_id`), and send `Cache-Control: no-store`. Read authorization denials are audited by the admin wrapper. Filters are fixed per route, malformed filters return 400, page size defaults to 100 and is capped at 200, and `before_id` is exclusive. App-detail logs use the same admin-only `/api/logs?target_id=…` route; the stored target binding comes from the source token, never an application-supplied field.

- Sequenced ingestion uses one canonical positive int64 `Ky-Log-Sequence`, starting at 1. A 204 includes `Ky-Log-Receipt: ky.log-ingest/1`, the sequence and `Ky-Log-Digest` (SHA-256 of the exact NDJSON bytes). Only the next sequence or an exact replay of the latest body succeeds; gaps, stale sequences, changed replay bytes and legacy writes after activation return 409 without a receipt. Malformed headers return 400. Authentication, quotas and whole-batch validation still apply to replay. Use a dedicated source and one ordered sender; advance only after persisting the verified acknowledgment.
- Non-browser `OPTIONS /api/ingest/logs` authenticates exactly one source Bearer header and
  uses the same source quota, returning 204 with `Ky-Log-Receipt: ky.log-ingest/1` and
  `Ky-Log-Source` (this source UUID). It writes no log or activity rows. Verify this before
  first sequenced mutation; old servers cannot safely supply the receipt protocol. Ordinary
  browser CORS preflights keep their existing response and expose no source capability.
  Revoked/invalid credentials get 401 without capability headers; source tokens still cannot
  read collected logs/activity or administer sources.
- Admin pairing optionally accepts `offboarding_alerts:true` with a required watched
  `target_id`; the claim cannot change this grant or target. A granted, still-bound source's
  authenticated OPTIONS adds `Ky-Log-Alerts: ky.offboarding-alerts/1`. Each sequenced batch
  must be one recognized kyoffboard started/success/failed projection; invalid shape is
  400, unsequenced writes and lost target scope are 409, without a receipt. Failures create
  native Alerts rows atomically; source tokens retain no administration or read authority.

| Method | Path | Handler | Response |
|---|---|---|---|
| POST | `/api/backup/drill` | `handleBackupDrill` | `recoveryclient.DrillResult`; 409 when another HTTP/CLI drill holds the data-directory lock |
| POST | `/api/backup/export-capsule` | `handleExportCapsule` | `.kycap` attachment; POST so the CSRF check covers it |
| POST | `/api/backup/pair-remote` | `handlePairRemoteRecovery` | `{recovery_key_id, threshold, total_shares}` |
| POST | `/api/backup/deposit` | `handleRunBackup` | `recoveryclient.Result` (+`receipt_unrecorded`) |
| DELETE | `/api/backup/pairing` | `handleUnpair` | `{paired:false}`; URL and token rows only, key pin stays |
| POST | `/api/backup/pin-key` | `handlePinKey` | write-once; 409 on a different key |
| PUT | `/api/backup/schedule` | `handleSetSchedule` | `{interval_sec}` read back from the store |
| GET | `/api/backup/status` | `handleBackupStatus` | pairing, key, local copies, schedule, members, `database_driver`; never the token |

### Monitoring

| Method | Path | Auth | Response |
|---|---|---|---|
| GET | `/api/status` | session | `{checked_at, total, ok, degraded, down, pending, paused, problems:[{id,name,state,since,cause}], webhook:{configured, last}}`; disabled targets count only as `paused`, never as a state or a problem; `configured` is true whenever a webhook row exists, including one that cannot be opened (`Webhooks.Load` error) — delivery is broken, not absent |
| GET | `/api/targets` | session | `{targets:[target + silenced_until, until_fixed, basic]}` |
| GET | `/api/targets/{id}` | session | `{target, last_result, events:[last 20]}` |
| POST | `/api/targets` | admin | 201 `{target}`; 400 validation; 409 duplicate name |
| PUT | `/api/targets/{id}` | admin | `{target}`; an omitted `interval_sec` keeps the stored one |
| DELETE | `/api/targets/{id}` | admin | 204 |
| POST | `/api/targets/{id}/silence` | admin | body `{"for":"1h"\|"8h"\|"until_fixed"\|"off"}`; `{silenced_until, until_fixed}` |
| GET | `/api/alerts` | session | `{events, next_before_id}` newest id first, `?target=&before_id=&limit=` (limit ≤ 200, default 50; `before_id` exclusive, `next_before_id` 0 on the last page) |
| GET | `/api/alerts/webhook` | admin | `{configured, preset, url_origin, has_url, has_token, last}`; never the token or the URL past scheme and host (PUT answers the same shape) |
| PUT | `/api/alerts/webhook` | admin | body `{preset,url,token,clear_token}`; an empty `token` keeps the stored one only when preset, URL scheme and host (including port) are unchanged; `clear_token` removes it only for presets where a token is optional. Gotify requires a nonblank app token; invalid edits return 400 and preserve the stored webhook |
| DELETE | `/api/alerts/webhook` | admin | 204 |
| POST | `/api/alerts/webhook/test` | admin | `{ok:true}`; 412 no webhook; 502 `{error: notify.Reason}` on failed delivery, never the error text; one attempt, no retries |
| GET | `/api/kyyard` | session | `kyyard.StatusView`; never the token |
| POST | `/api/kyyard/pair` | admin | body `{url, pairing_code}` → 200 `StatusView` with `paired:true, stale:true`, no `fetched_at` ("first pull pending" — the response never waits for a pull); 400 for a bad URL, a refused address (loopback, link-local, `localhost` by name, or a name resolving to one at dial time), or plain http without `KYPULSE_KYYARD_ALLOW_HTTP`; 403 KyYard refused the code; 429 rate-limited; 502 `{error, reason}` (only those two keys) for any other network or status failure, `reason` from `kyyard.Reason` |
| DELETE | `/api/kyyard` | admin | 204, idempotent; deletes the URL and sealed token here only — revoking the token in KyYard is a separate admin step the UI names |
| GET | `/api/kyyard/containers` | admin | `[]kyyard.Suggestion`; prefills the add-target form |

- `/api/status` carries `"kyyard": kyyard.StatusView`; `GET /api/targets/{id}` carries `"kyyard": kyyard.ContainerFacts|null`, populated when the target's `container` field matches the latest KyYard inventory.
- Pairing while already paired replaces the sealed row (audited `replaced=true`); the previous token is not revoked here, only in KyYard. A claim that succeeds but cannot be saved is audited `outcome=failure … reason=save` and answers 500 telling the admin to revoke the issued token in KyYard (Members → Service tokens) before retrying.
- `handleKyYardPair` never calls `PullNow` inline. Order: `Service.Replace` saves and adopts under the lifecycle lock shared with collection imports; on success `Kick()` inventory and respond from `Status`. Unpair uses the same lifecycle lock. `StatusView` includes independent inventory/log/audit last-success, fixed-error and stale fields; admin Logs/Activity/settings display their ages.
- Audit actions: `admin.kyyard_pair` (details `outcome=success host=… org=… allow_http=… replaced=…`; `outcome=failure host=… reason=<kyyard.Reason>` on a claim failure; `outcome=failure host=… org=… reason=save`; never the pairing code or the token), `admin.kyyard_unpair`.

- `GET /healthz` is public: `health.Handler("kypulse", lg, database ping)` from ky-primitives, `ky.health/1`, 200 for ok/degraded and 503 for down, cached 5 s by the lib. It is the route an external monitor should watch; kyPulse does not monitor itself. `NewServer(cfg, st, lg, mon, yard)` takes the logger as its third argument, the `*monitor.Service` as its fourth and the `*kyyard.Service` as its fifth; `health.Handler` panics on a nil logger. Two checks: `database` (`s.store.Ping`) and `audit` (`store.Audit().Ready`; `degraded` with reason `chain_broken` when the stored tail no longer matches its anchor).

- `POST /api/backup/deposit` is one `recoveryclient.Run`: seal once, deliver to the local directory and to KyRecovery when paired. 412 no key, key pin missing, no destination, no database snapshot, or a private destination with `KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY` off; 409 key mismatch or a run in flight; 413 over the capsule caps; 502 when KyRecovery refused (`recoveryclient.ErrRemote`, naming a local copy that was written, so the `ErrPrivateDestination` arm must stay above it: the lib wraps both on the dial path); 500 for a failure before a byte left; 200 with `receipt_unrecorded` when the store holds the capsule but the receipt was not written. It runs on a context detached from the request with a 16-minute write deadline; the acting admin is resolved before the upload and the audit row is written on that same detached context.
- The write-once, irreversible backup handlers (`handlePairRemoteRecovery`, `handlePinKey`, `handleRunBackup`) run on `context.WithoutCancel(r.Context())` so a dropped connection cannot leave a pin, a pairing or a deposit half-written with no audit row; the idempotent ones (`handleSetSchedule`, `handleUnpair`) stay on the request context. Their routes are registered as `s.tracked(s.requireAdmin(s.handleX))`, so the `detached` counter is incremented the moment `ServeHTTP` dispatches -- before `requireAdmin`'s session lookup, which is itself a store round-trip that `ReadTimeout` (15s) lets outlast `shutdownTimeout` (5s). Registering inside the handler was too late: `Shutdown` returns after its timeout with requests still active, and one still in the auth lookup would leave `WaitDetached()` reading zero and the store closing under a request about to pin a key. The header-read window before `ServeHTTP` is entered cannot be covered by any counter, because no handler goroutine exists yet; `Shutdown`'s own drain is what covers it. The counter is a mutex and a `sync.Cond`, not a `sync.WaitGroup`, which panics when an `Add` from zero races an in-progress `Wait` -- two admin requests at SIGTERM do exactly that. `WaitDetached()` is what `cmd/server` blocks on before closing the store, because `http.Server.Shutdown` returns without knowing these goroutines exist.
- Audit actions: `backup.paired`, `backup.pair_failed`, `admin.backup_run` (details start with `outcome="success|failure"`), `admin.backup_unpair`, `admin.backup_key_pin`, `admin.backup_schedule`, `admin.backup_export` (a downloaded capsule, resource the capsule ID). `AuditDetails` flattens the lib's details map into the bounded audit field with locally derived fields first and remote error text last; values are quoted and `=` is escaped before the final `AuditSafe` cut. `cmd/server` uses it for the scheduler and CLI rows. Details carry key or capsule IDs, digests and paths, never the token.
- Rate-limit keys for `login:` and `mfa:` come from `auth.ClientIP`, never from `RemoteAddr` or a raw header, so a limit is neither shared by everyone behind a proxy nor bypassable by forging `X-Forwarded-For`. Every IP-keyed limiter uses `s.limitIP`, which keys IPv6 by its /64.
- Per-account windows (`login-user:` 10/15 min, `mfa-user:`, `password-change-user:`) use `s.allowAccount`: keyed only by an existing user's ID, kept apart from the evictable IP map and never evicted by `makeRoom`.
- CORS permits only the exact configured `KYPULSE_APP_URL` origin and credentialed browser writes require matching CSRF cookie/header tokens.
- API request bodies are capped at 1 MiB and all responses receive baseline CSP, anti-framing, MIME-sniffing, and referrer-policy headers.
- `GET /api/settings` tiers its payload: public fields for the login screen, `db_driver` for any session, and `extra_settings` for admins only; KyRecovery tokens are omitted in both sealed and legacy plaintext forms, dropped by the `kyrecovery_token` key prefix rather than by literal key name.


- `GET /api/activity` additionally returns `bursts:[{app,actor,ip,count,from,to}]`, computed
  across retained matching events independently of `limit`/`before_id`, bounded to 100
  newest groups. Only the grouping dimension's actor or IP is populated. Burst strings
  use the same display sanitization as timeline fields. `has_app_events` checks retained
  app/target coverage independently of actor/outcome/time/page filters; it prevents the
  UI from mislabeling a filtered-empty page as an app with no retained audit events.

## Verification
- `go test -v ./internal/api/...` (`authz_test.go` pins the per-role exposure of every privileged route; `backup_test.go` the backup routes, on SQLite only because a run snapshots the database)
- `scripts/smoke-test.sh` asserts the same boundaries against a running binary
- Set `KYQ_TEST_INSTALLER_RELAY_ROOT` to the isolated KyQuickStart checkout and run
  `go test -race ./internal/api -run TestOffboardAuditRelayNative -count=1` with the local
  product workspace. The real native API/store commits then loses a socket response; the
  installer fixture resumes its retained backoff through the actual `kyoffboard` executable
  using verified public TLS trust and protected target configuration. Assert three POSTs
  produce exactly two log/activity rows with native source attribution, then joined SIGTERM
  and retained receipt/lease proof. No live deployment or product account mutation occurs.
- With the same fixture variable, `go test -race ./internal/api -run
  '^TestOffboardFailureRelayNative$' -count=1` verifies an actual installer audit outbox
  against native ingestion, source alert capability, durable failure creation, fresh monitor
  webhook delivery and replay suppression on SQLite or disposable PostgreSQL. It uses a
  private synthetic native revocation failure and webhook transport stand-in; it does not
  prove deployed product failures, production webhook connectivity or KyIdentity composition.

## Child DOX Index
None.
