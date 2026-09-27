# Storage Layer

## Purpose
Provides the unified Database Abstraction Layer (DAL) supporting pluggable backends (SQLite zero-CGO default and PostgreSQL enterprise) with automated dialect-aware migrations.

## Ownership
Owns data models, store interfaces (`UserStore`, `SessionStore`, `AuditStore`, `SettingsStore`, `TargetStore`), dialect translations, and schema migrations.

## Local Contracts
- `targets.track_json` and `targets.last_result` are JSON owned by `internal/monitor` (`alerts.Track`, `poller.Result`); the store stores and returns them verbatim. `state`, `state_since` and `cause` are denormalised from the track for listing. Silences live in `silenced_until`/`until_fixed`, not in the track JSON, so an admin's silence and a poll never write the same value. Deleting a target cascades its events.
- `CompletePasswordChange` atomically compares the old password, updates a flagged local account, clears the flag, deletes sessions/MFA challenges and records `auth.password_changed`. Session/MFA issuance locks the same user row against the verified hash; MFA challenges persist the creation-time password hash, and consumption returns that snapshot to reject stale completions. Migration 4 discards preexisting challenges because their credential snapshot is unknown.
- `ResetAdminPassword` reactivates a local administrator with the replacement flag set and shares the atomic grant purge and audit path with `CompletePasswordChange`; it also works for disabled accounts.
- Audit rows are links of a keyed hash chain (`internal/store/audit.go`, `ky-primitives/auditchain`), keyed by `config.DatabaseConfig.AuditKey`. `server_settings["audit_anchor"]` holds the chain's count and head outside the log. `store.Open` refuses to start (`ErrAuditUnplaceable`) whenever the log and its anchor disagree on count or on the tail. A log written before migration 6 is keyed once on first start after upgrade; every append after that chains at write time inside the caller's transaction — `revokePasswordGrants` writes `auth.password_changed` through `auditStore.append` on the same tx that revokes sessions. `append` returns a `finish(committed bool)` closure; the caller must call it exactly once with the transaction's commit outcome, which releases the chain lock and, on a failed commit, drops the in-memory chain so the next append re-reads the tail from disk. `AuditStore.VerifyChain` walks the whole log against the anchor without writing. `Ready` takes no lock and runs in its own read-only transaction (repeatable-read on Postgres), so it reflects one snapshot and cannot deadlock against a concurrent append.
- `store.Open(ctx, cfg)` initializes and auto-migrates the configured database backend.
- SQLite runs in WAL mode with foreign keys enabled.
- PostgreSQL queries are rebound dynamically from standard positional parameters.
- MFA challenges are consumed with database state transitions that permit exactly one successful use.
- Recovery-code hash updates use optimistic concurrency so simultaneous redemption cannot reuse a code.
- Users carry `RoleAdmin` or `RoleViewer`; `CreateUser` and `UpdateUser` return `ErrInvalidRole` for anything else. New users default to viewer.

## Verification
- `go test -v ./internal/store/...`

## Child DOX Index
None.
