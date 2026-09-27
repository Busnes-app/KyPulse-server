# API

## Purpose
Exposes HTTP REST routes, authentication endpoints, Single Sign-On callbacks, backup restore drill handlers, monitoring routes, and static React PWA hosting.

## Ownership
Owns HTTP routing, request parsing, session cookie validation, CORS headers, and error response formatting.

## Local Contracts
- POST `/api/auth/change-password` accepts a restricted local session, current password and a different policy-valid new password. Browser CSRF and per-IP/account limits apply. Success revokes all sessions and requires sign-in again; flagged sessions get `password_change_required` on protected routes and public-only settings.
- All JSON API endpoints return structured errors `{"error": "message"}` upon failure.
- Non-API routes fall back to serving `web.Handler()` for client-side SPA routing.
- New routes are unauthenticated only by deliberate choice; privileged ones are registered wrapped in `s.requireAdmin` in `routes()`, so the trust level of every route is readable in one place.
- Backup routes and theme writes are admin-only: capsules and settings carry site data and secrets. The scaffold has no step-up; admin-only plus `TestPrivilegedEndpointsRequireAdmin` is its equivalent for every destructive backup route. Routes are registered with method patterns, and because the SPA catch-all answers any method, tests pin that a wrong method never reaches a backup handler rather than expecting 405.
- Viewer reads use `s.requireSession`; monitoring writes and the webhook use `s.requireAdmin`. The webhook token is write-only: never in `GET /api/alerts/webhook`, `/api/settings` (sealed `_enc` rows are filtered by suffix) or `/api/status`. Audit actions: `admin.target_create|update|delete|silence`, `admin.webhook_set|delete|test`; `alert.sent|send_failed` are written by `internal/monitor` with actor `system`. A webhook URL is never audited: `admin.webhook_set` records `preset`, `scheme`, `host` and `allow_http`.
- Every audit row goes through `s.audit`, which bounds both text fields and logs `audit_write_failed` when the row cannot be written.

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
| GET | `/api/status` | session | `{checked_at, total, ok, degraded, down, pending, paused, problems:[{id,name,state,since,cause}], webhook:{configured, last}}`; disabled targets count only as `paused`, never as a state or a problem |
| GET | `/api/targets` | session | `{targets:[target + silenced_until, until_fixed, basic]}` |
| GET | `/api/targets/{id}` | session | `{target, last_result, events:[last 20]}` |
| POST | `/api/targets` | admin | 201 `{target}`; 400 validation; 409 duplicate name |
| PUT | `/api/targets/{id}` | admin | `{target}`; an omitted `interval_sec` keeps the stored one |
| DELETE | `/api/targets/{id}` | admin | 204 |
| POST | `/api/targets/{id}/silence` | admin | body `{"for":"1h"\|"8h"\|"until_fixed"\|"off"}`; `{silenced_until, until_fixed}` |
| GET | `/api/alerts` | session | `{events, total}`, `?target=&offset=&limit=` (limit ≤ 200, default 50) |
| GET | `/api/alerts/webhook` | admin | `{configured, preset, url, has_token, last}`; never the token |
| PUT | `/api/alerts/webhook` | admin | body `{preset,url,token,clear_token}`; an empty `token` keeps the stored one only when preset and URL host are unchanged, `clear_token` always stores none; 400 on `notify.Validate` failure |
| DELETE | `/api/alerts/webhook` | admin | 204 |
| POST | `/api/alerts/webhook/test` | admin | `{ok:true}`; 412 no webhook; 502 `{error: notify.Reason}` on failed delivery, never the error text |

- `GET /healthz` is public: `health.Handler("kypulse", lg, database ping)` from ky-primitives, `ky.health/1`, 200 for ok/degraded and 503 for down, cached 5 s by the lib. It is the route an external monitor should watch; kyPulse does not monitor itself. `NewServer(cfg, st, lg, mon)` takes the logger as its third argument and the `*monitor.Service` as its fourth; `health.Handler` panics on a nil logger. Two checks: `database` (`s.store.Ping`) and `audit` (`store.Audit().Ready`; `degraded` with reason `chain_broken` when the stored tail no longer matches its anchor).

- `POST /api/backup/deposit` is one `recoveryclient.Run`: seal once, deliver to the local directory and to KyRecovery when paired. 412 no key, key pin missing, no destination, no database snapshot, or a private destination with `KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY` off; 409 key mismatch or a run in flight; 413 over the capsule caps; 502 when KyRecovery refused (`recoveryclient.ErrRemote`, naming a local copy that was written, so the `ErrPrivateDestination` arm must stay above it: the lib wraps both on the dial path); 500 for a failure before a byte left; 200 with `receipt_unrecorded` when the store holds the capsule but the receipt was not written. It runs on a context detached from the request with a 16-minute write deadline; the acting admin is resolved before the upload and the audit row is written on that same detached context.
- The write-once, irreversible backup handlers (`handlePairRemoteRecovery`, `handlePinKey`, `handleRunBackup`) run on `context.WithoutCancel(r.Context())` so a dropped connection cannot leave a pin, a pairing or a deposit half-written with no audit row; the idempotent ones (`handleSetSchedule`, `handleUnpair`) stay on the request context. Their routes are registered as `s.tracked(s.requireAdmin(s.handleX))`, so the `detached` counter is incremented the moment `ServeHTTP` dispatches -- before `requireAdmin`'s session lookup, which is itself a store round-trip that `ReadTimeout` (15s) lets outlast `shutdownTimeout` (5s). Registering inside the handler was too late: `Shutdown` returns after its timeout with requests still active, and one still in the auth lookup would leave `WaitDetached()` reading zero and the store closing under a request about to pin a key. The header-read window before `ServeHTTP` is entered cannot be covered by any counter, because no handler goroutine exists yet; `Shutdown`'s own drain is what covers it. The counter is a mutex and a `sync.Cond`, not a `sync.WaitGroup`, which panics when an `Add` from zero races an in-progress `Wait` -- two admin requests at SIGTERM do exactly that. `WaitDetached()` is what `cmd/server` blocks on before closing the store, because `http.Server.Shutdown` returns without knowing these goroutines exist.
- Audit actions: `backup.paired`, `backup.pair_failed`, `admin.backup_run` (details start with `outcome="success|failure"`), `admin.backup_unpair`, `admin.backup_key_pin`, `admin.backup_schedule`, `admin.backup_export` (a downloaded capsule, resource the capsule ID). `AuditDetails` flattens the lib's details map into the bounded audit field with locally derived fields first and remote error text last; values are quoted and `=` is escaped before the final `AuditSafe` cut. `cmd/server` uses it for the scheduler and CLI rows. Details carry key or capsule IDs, digests and paths, never the token.
- Rate-limit keys for `login:` and `mfa:` come from `auth.ClientIP`, never from `RemoteAddr` or a raw header, so a limit is neither shared by everyone behind a proxy nor bypassable by forging `X-Forwarded-For`.
- CORS permits only the exact configured `KYPULSE_APP_URL` origin and credentialed browser writes require matching CSRF cookie/header tokens.
- API request bodies are capped at 1 MiB and all responses receive baseline CSP, anti-framing, MIME-sniffing, and referrer-policy headers.
- `GET /api/settings` tiers its payload: public fields for the login screen, `db_driver` for any session, and `extra_settings` for admins only; KyRecovery tokens are omitted in both sealed and legacy plaintext forms, dropped by the `kyrecovery_token` key prefix rather than by literal key name.

## Verification
- `go test -v ./internal/api/...` (`authz_test.go` pins the per-role exposure of every privileged route; `backup_test.go` the backup routes, on SQLite only because a run snapshots the database)
- `scripts/smoke-test.sh` asserts the same boundaries against a running binary

## Child DOX Index
None.
