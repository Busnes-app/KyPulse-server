# kyPulse Logs Backend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Accept bounded, source-authenticated log batches, query logs and activity as an administrator, and enforce retention without putting collected data into recovery capsules.

**Architecture:** Keep SQL ownership in `internal/store`; `internal/logstore` owns pure parsing, display sanitization and retention selection; `internal/ingest` owns the batch wire format. API handlers reuse existing session/admin wrappers, audit writer and trusted-proxy IP resolution. Logs and derived activity commit together; source tokens are hashes in the main database and never authenticate a user session.

**Tech Stack:** Existing Go 1.26.6, database/sql, SQLite/Postgres, ky-primitives v0.9.0, standard-library JSON and crypto; no new dependency.

**Spec:** `docs/superpowers/specs/2026-09-26-kypulse-design.md`, sections 2 (backup scope), 4, Error handling and Testing.

## Global Constraints

- “At most 1 MiB per request and 16 KiB per line.”
- “Each token may send 60 requests per minute”; 429 carries `Retry-After`.
- “A 2xx means the whole batch is stored”.
- Pairing: “6-digit code: 15-minute TTL, single use”; “5 attempts per minute per IP and 30 per minute globally”.
- “The source name comes from the token. Any source field in the body is ignored.”
- “Logs and Activity are admin-only”. This includes recent app-detail logs.
- “An hourly job deletes rows older than 7 days”, then oldest-first to `KYPULSE_LOG_MAX_BYTES`, default “1 GiB”. Activity shares the budget.
- “Logs and activity are excluded” from backup. Keep kyPulse's own keyed audit chain intact.
- Existing suite DOX, role, logging and outbound-egress contracts apply. No push, PR opening or merge without Yoshi's instruction. Leave the KyYard main checkout alone.

## Review Focus

1. A bad last NDJSON record or failed activity insertion must leave zero records from the batch (tasks 1 and 3).
2. Concurrent claims and revoke-versus-ingest must have database-enforced outcomes (tasks 2 and 3).
3. Future event timestamps, duplicated activity payloads and tiny size limits must not defeat retention (task 4).
4. A deleted SQLite row can remain in free pages; capsules must contain neither accessible rows nor residual collected text (task 5).
5. Logs reached through app detail, bearer tokens or malformed filters must not bypass admin authorization or turn input into executable HTML/SQL (tasks 1 and 3, frontend companion).

## Scope and implementation decisions

This is PR A. PR B is [sender, screens and KyYard collection](2026-09-27-kypulse-logs-clients.md). These are proposed implementation decisions for review, not already-shipped contracts:

- **Wire framing:** each NDJSON record is an object containing `line` (string), optional `time` (RFC3339Nano transport timestamp), and optional `truncated` (boolean). An arbitrary `source` is ignored. The string holds an original JSON or plain-text log line. This reconciles malformed-batch rejection with accepting non-JSON application output. Accept unknown fields for additive compatibility; reject missing/non-string `line`, invalid UTF-8, malformed time and non-object records. No compression in v1 (415 for Content-Encoding other than identity).
- A terminal LF is optional; CRLF framing is accepted. Empty batch/blank framing records are 400. Embedded newlines inside `line` remain one stored record and are stripped for display. At most 1,000 records per request additionally bounds empty-line amplification. Sender normally uses at most 500.
- Truncate decoded `line` at a UTF-8 boundary to 16 KiB and set `truncated=true`; discard the remaining bytes of that string, not the remainder of the batch. Wire bytes, including JSON escaping, count toward 1 MiB. The sender must split by encoded size as well as line count.
- Source attribution is authoritative; the application's `app` field remains untrusted labeling, not identity. Pairing-code creation may bind a source to a watched target. Bound sources use that target ID for detail queries; raw `app` text cannot cause a line to appear in another target's detail. Unbound sources remain visible in the global screens.
- Age is measured from `received_at`, preventing forged/future clocks from retaining data indefinitely. Sort eviction by `(received_at,id)`. Preserve original event time separately. The byte cap measures stored UTF-8 field payloads, including derived activity separately, not database/index/WAL file size. Document this distinction.
- Enforce the byte cap inside each append transaction as well as hourly age cleanup. Reject startup caps smaller than 128 KiB: that comfortably admits one bounded record plus its activity. A successfully accepted batch can be evicted by retention, like other retained data; acknowledge only after the transaction commits.
- Imported activity never enters `audit_records`. A log with positive `seq`, `hash`, `fields`, plus a non-empty `action` is treated as an audit-shaped record; there is no claim of authenticity or chain verification. Map `user_id`, `action`, `resource`, `outcome` (fallback `result`), `ip_address`. Ordinary auth events with no audit shape stay logs.
- Query APIs exist solely to support the screens: descending local integer IDs, page limit default 100/max 200, exclusive `before_id`, literal substring search (escape SQL LIKE `%`, `_` and escape character), fixed filter columns, no full-text index.

## File ownership

| Files | Responsibility |
|---|---|
| `internal/store/logs.go`, `sources.go`, matching tests; `store.go`, `sqlstore.go`, `models.go`, `migrations/migrations.go` | Typed persistence, source claims, transactional append and pruning |
| `internal/logstore/parse.go`, `retention.go`, matching tests, `AGENTS.md` | Pure line normalization, safe display, eviction selection |
| `internal/ingest/batch.go`, `batch_test.go`, `AGENTS.md` | Wire decoding and validation |
| `internal/api/logs.go`, `sources.go`, matching tests; `server.go`, `authz_test.go` | Explicit route trust boundaries, read filters, pairing and request limits |
| `internal/config/config.go`, `config_test.go`; `cmd/server/logretention.go`, `main.go` | Byte-limit configuration and owned shutdown of cleanup loop |
| `internal/backup/payload.go`, `payload_test.go` | Sanitize only the temporary snapshot before sealing |
| `README.md`, relevant `AGENTS.md`, `scripts/smoke-test.sh` | Operator contract and verification |

Read the owning AGENTS.md before implementation. New package AGENTS.md files get root index entries when the packages actually exist. The planning-only change does not change existing DOX contracts.

## Task 1: Atomic log/activity persistence and parsing

**Interfaces:** Add `Store.Logs() LogStore`. Define persistence models in `internal/store/logs.go`:

```go
type LogLine struct {
    ID int64
    Time, ReceivedAt time.Time
    SourceID, Source, TargetID, App, Level, Event, Message, Raw string
    Truncated bool
    Bytes int64
}
type Activity struct {
    ID int64
    Time, ReceivedAt time.Time
    SourceID, TargetID, App, Actor, Action, Target, Outcome, IP string
    ExternalKey string // non-empty only for imported feed rows
    Bytes int64
}
type LogFilter struct {
    TargetID, App, Level, Text string
    From, To time.Time
    BeforeID int64
    Limit int
}
type ActivityFilter struct {
    TargetID, App, Actor, Outcome string
    From, To time.Time
    BeforeID int64
    Limit int
}
type LogBatch struct { Logs []LogLine; Activity []Activity }
type LogStore interface {
    Append(ctx context.Context, sourceID string, batch LogBatch, maxBytes int64) error
    List(ctx context.Context, f LogFilter) ([]LogLine, error)
    ListActivity(ctx context.Context, f ActivityFilter) ([]Activity, error)
    Prune(ctx context.Context, now time.Time, maxBytes int64) error
}
```

`Append` with a source ID checks that source is active in the same transaction as insertion. An empty ID is reserved for internal KyYard collection, never accepted from HTTP. Add explicit snake_case JSON tags to read models; omit internal byte accounting and external dedupe keys from JSON. `TargetID` is nullable in SQL and empty in Go when absent. Target deletion sets it NULL; history and token revocation history survive target deletion.

`logstore.Parse(raw, sourceID, source, targetID string, transportTime, receivedAt time.Time, truncated bool) (store.LogLine, *store.Activity)` returns bounded rows. `logstore.Display(string) string` strips CSI/OSC ANSI sequences and C0/C1/DEL controls, including incomplete terminal escapes; render all fields through it at the API boundary. Storage retains bounded raw text for triage. Invalid or missing application timestamps fall back to transport time, then receive time. Application JSON shape errors fall back to raw text, unlike invalid wire framing.

- [ ] Add migration 7 for `log_lines`, `activity`, and `log_usage` (one accounting row). Index target/app plus ID for queries, `received_at,id` for pruning, and non-empty external activity keys uniquely. Use the existing dialect mapping and migration transaction. Add sources in task 2 before adding source foreign keys. For ID allocation, follow the existing target-event dialect pattern.
- [ ] Add failing parse tests, including this kernel and fixtures emitted by real ky-primitives `logging.Logger`/`Logger.Audit`:

```go
func TestPlainAndControlText(t *testing.T) {
    now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
    line, activity := Parse("\x1b[31m<img onerror=x>\x1b[0m\x00", "s", "host", "t", now, now, false)
    if activity != nil || line.Source != "host" || line.TargetID != "t" {
        t.Fatalf("wrong attribution: %+v %+v", line, activity)
    }
    if got := Display(line.Message); got != "<img onerror=x>" {
        t.Fatalf("display = %q", got)
    }
}
```

- [ ] Add SQL tests using `newTestStore(t)` for log/activity all-or-nothing insertion, duplicate external keys, deterministic pages with equal timestamps, literal `%_` search, and a trigger-induced failure on activity insertion. Confirm rollback includes usage accounting. Run `go test ./internal/store ./internal/logstore` and observe the new missing implementation/test failures.
- [ ] Implement parser and store. Use one transaction and serialize changes to `log_usage` with an UPDATE first (SQLite write lock, PostgreSQL row lock); all append/prune paths obey the same lock order. Store `Bytes` derived by trusted code from every persisted variable-length field, never a caller-supplied count. Use a fixed allowance for IDs/times/flags. Do not marshal a second full copy merely to measure it. Treat exact replay of an external key as a no-op without incrementing usage.
- [ ] Implement tests for 16 KiB multibyte boundaries, unknown JSON, arrays/scalars, missing audit fields and malformed app timestamps. Verify `Parse` never yields oversized strings. `Display` must leave angle brackets as text, not remove them; React supplies HTML escaping.
- [ ] Run `go test -race ./internal/store ./internal/logstore`, then the same packages with `KYPULSE_TEST_POSTGRES_DSN` against an available test database. Commit the passing task locally with its package docs.

## Task 2: Source pairing and revocation

**Files:** `internal/store/sources.go`, `sources_test.go`, migration registry, store wiring; `internal/api/sources.go`, `sources_test.go`, `server.go`, `authz_test.go`.

**Interfaces:** `Store.Sources() SourceStore`; concrete types and signatures:

```go
type LogSource struct {
    ID, Name, TargetID string
    CreatedAt time.Time
    RevokedAt *time.Time
}
type SourceStore interface {
    CreateCode(ctx context.Context, codeHash, targetID string, expiresAt time.Time) error
    Claim(ctx context.Context, codeHash, tokenHash, name string, now time.Time) (LogSource, error)
    Authenticate(ctx context.Context, tokenHash string) (LogSource, error)
    List(ctx context.Context) ([]LogSource, error)
    Revoke(ctx context.Context, id string, now time.Time) error
}
```

Route contract (all JSON responses `Cache-Control: no-store`):

| Method/path | Authority | Request → response |
|---|---|---|
| POST `/api/log-sources/pairing` | `requireAdmin` + normal CSRF | `{target_id?:string}` → `{code,expires_at}` |
| POST `/api/log-sources/claim` | unauthenticated, limited | `{pairing_code,name}` → `{token,source:{id,name,target_id}}` |
| GET `/api/log-sources` | `requireAdmin` | → `{sources:[...]}`; no hashes/codes/tokens |
| DELETE `/api/log-sources/{id}` | `requireAdmin` + normal CSRF | → `{revoked:true}` |

- [ ] Add migration 8 for source and pending-code tables. Code hashes unique; token hashes unique. Scope `CreateCode` to an optional existing target. Random code generation uses `crypto/rand.Int` and six zero-padded digits, bounded collision retries; token uses 32 random bytes hex-encoded. Store only SHA-256 hashes of codes/tokens, and never put them in settings or audit text.
- [ ] Add the race test around the real SQL store. Kernel:

```go
var winners atomic.Int32
var wg sync.WaitGroup
for i := 0; i < 8; i++ {
    wg.Add(1)
    go func(i int) {
        defer wg.Done()
        _, err := st.Sources().Claim(ctx, codeHash, fmt.Sprintf("hash-%d", i), "host", now)
        if err == nil { winners.Add(1) }
    }(i)
}
wg.Wait()
if winners.Load() != 1 { t.Fatalf("claims won = %d", winners.Load()) }
```

Set up `ctx`, `st`, `now`, and `codeHash` in the test with `CreateCode`; hashes in production are full digests. Test expiry at the exact deadline, a revoked hash, nonexistent target and duplicate source names (names need not be globally unique; ID is identity). Run tests red.
- [ ] Implement `Claim` by deleting the unexpired code with `DELETE ... RETURNING` inside the transaction that inserts the source. Failure rolls back code consumption. Expired codes are deleted during create/claim maintenance. Bound name to `[A-Za-z0-9][A-Za-z0-9_.-]{0,63}`. Invalid/used/expired claims all receive the same 403; unknown/revoked ingest tokens receive 401.
- [ ] Add handlers and explicit route wrappers. Apply 30/min global limiter before 5/min `s.requestIP(r)` limiter; reuse the existing bounded attempt-map mechanism and preserve the global counter outside evictable IP entries. Six attempts from one IP return 429 with Retry-After; 31 across IPs also do. Test trusted-proxy handling with forged headers from an untrusted peer. Limit these JSON request bodies to 4 KiB and require exactly one object.
- [ ] Audit generation, claim success/failure, revoke and refused requests via `s.audit`; log identifiers/reason codes, never request body, code, token or token digest. Keep revoked source records for historical attribution. Do not create one audit row per successful ingest.
- [ ] Test every route as anonymous/admin/viewer and with source Bearer only. Session cookies alone cannot claim ingest authority. Claim with a browser session still follows existing CSRF policy. Run `go test -race ./internal/store ./internal/api`, plus PostgreSQL source races. Commit locally.

## Task 3: Ingest and admin read endpoints

**Files:** `internal/ingest/batch.go`, `batch_test.go`, `internal/api/logs.go`, `logs_test.go`, `server.go`, `authz_test.go`.

**Interfaces:**

```go
// internal/ingest
const MaxRequestBytes = 1 << 20
const MaxLineBytes = 16 << 10
const MaxRecords = 1000
type Record struct {
    Line string `json:"line"`
    Time time.Time `json:"time,omitempty"`
    Truncated bool `json:"truncated,omitempty"`
}
func Decode(body []byte) ([]Record, error)
```

`POST /api/ingest/logs` uses Bearer only; `GET /api/logs` and `GET /api/activity` use `requireAdmin`. Read responses are `{items:[],next_before_id:0}` (last returned ID only when a further page exists). Log filters: `target_id,app,level,from,to,text,before_id,limit`; activity: `target_id,app,actor,outcome,from,to,before_id,limit`. Return 400 for unknown/invalid filter values, negative IDs, limit outside 1..200, text/actor/app over 256 UTF-8 bytes, invalid timestamps or inverted time ranges. Use UTC timestamps in responses and `Cache-Control: no-store`.

- [ ] Write the all-or-nothing decoder test before implementation:

```go
func TestBadLastRecordRejectsBatch(t *testing.T) {
    rows, err := Decode([]byte("{\"line\":\"first\"}\n{\"line\":7}\n"))
    if err == nil || rows != nil { t.Fatalf("accepted malformed batch: %#v %v", rows, err) }
}
```

Add empty/CRLF/no-final-LF/invalid-UTF8/oversized/truncated multibyte cases. `Decode` returns no partial rows on any error. Run `go test ./internal/ingest` red, then implement bounded whole-body decoding; use explicit raw JSON field checks so missing `line` and empty `line` are distinct.
- [ ] Implement handler order: parse bounded Bearer → hash/authenticate → per-source limiter → content-type/encoding → `http.MaxBytesReader`/`io.ReadAll` → `Decode` → parse all rows → one `Append` → 204. MIME parameters are accepted using `mime.ParseMediaType`. 400 malformed, 401 unknown/revoked, 413 oversized, 415 content type/encoding, 429 rate limit, 500 storage error. Read errors/timeouts never acknowledge. Discard source fields in the payload.
- [ ] Extend source store transactions so revoke and append take a source-row write lock in the same order before touching log usage. On SQLite acquire the write lock before reading the source. If append wins, it commits before revocation; if revoke wins, append refuses. Scope error translation without exposing SQL details. A 2xx cannot precede activity and usage commit.
- [ ] Add real HTTP tests for 1 MiB exact/plus-one including chunked requests, oversized line marked within an accepted request, wrong bearer on session routes, forged source/target fields, source revoked between handler auth and append, and 61st request returning 429 with a positive Retry-After. On a malformed final record, assert zero rows in both tables. Use a fixed clock for limit tests.
- [ ] Add admin-read handler tests for viewer denial even with a valid target ID, SQL metacharacters as literal text, deterministic pagination during concurrent inserts, and control/ANSI removal from every returned text field. Keep recent app logs on the separate admin route; never add log rows to the viewer-readable target response.
- [ ] Run `go test -race ./internal/ingest ./internal/logstore ./internal/api ./internal/store`; commit the passing task locally.

## Task 4: Retention, configuration and shutdown

**Files:** `internal/logstore/retention.go`, `retention_test.go`, `internal/store/logs.go`, `logs_test.go`, `internal/config/config.go`, `config_test.go`, `cmd/server/logretention.go`, `logretention_test.go`, `main.go`.

**Interfaces:** Add `Config.Logs.MaxBytes int64`; env `KYPULSE_LOG_MAX_BYTES`, decimal bytes, default `1<<30`, minimum `128<<10`. Pure selector:

```go
type RetainedRow struct { Kind string; ID int64; ReceivedAt time.Time; Bytes int64 }
func Evict(rows []RetainedRow, now time.Time, maxBytes int64) []RetainedRow
```

Production pruning reads candidates in bounded pages, not all retained payloads; `Evict` supplies the age/order/budget rule and tests. Shared usage tracks both tables. Remove ages first, then oldest globally, deterministic tie-break `(received_at,kind,id)`.

- [ ] Add failing age/size test kernel:

```go
func TestAgeBeforeSize(t *testing.T) {
    now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
    rows := []RetainedRow{
        {"log", 1, now.Add(-8*24*time.Hour), 50},
        {"activity", 2, now.Add(-time.Hour), 50},
        {"log", 3, now, 50},
    }
    got := Evict(rows, now, 50)
    if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 { t.Fatalf("evictions: %+v", got) }
}
```

Pin exactly seven days (keep until older), future event time with old receipt time, multibyte byte counts, independent activity eviction and repeat pruning. Run red, then implement selection and transactional SQL pruning.
- [ ] Exercise two appenders plus pruning on SQLite and PostgreSQL: sum of stored Bytes equals usage, never negative, and committed usage never exceeds configured cap. Insertion above the cap evicts oldest within the same transaction. Account for derived activity even if its originating log is later evicted.
- [ ] Validate config: zero, negative, overflow and nonnumeric values fail startup, not fallback silently. Document the logical payload-byte limit and database-space reuse; hourly deletion alone would not be a hard logical cap.
- [ ] Add `logRetentionLoop(ctx context.Context, st store.Store, maxBytes int64, lg *logging.Logger, done chan<- struct{})`: prune immediately and hourly; cancellation closes `done`. Wire startup/shutdown alongside the existing monitor/KyYard loops, waiting before store close. Use declared logging events and fixed reason fields on errors. Also expire unclaimed source codes hourly.
- [ ] Run `go test -race ./internal/logstore ./internal/store ./internal/config ./cmd/server`; verify cancellation with a running loop, not a one-hour sleep. Commit locally.

## Task 5: Exclude collected data from sealed backups

**Files:** `internal/backup/payload.go`, `payload_test.go`, `AGENTS.md`, `docs/RESTORE.md`.

**Interface:** Keep `Collect` and `snapshotSQLite` signatures. Modify only the temporary SQLite snapshot after `recoveryclient.SQLiteSnapshot` completes and before reading bytes. Source identities/token hashes belong in the capsule. Expiring pairing codes and imported-feed cursors do not.

- [ ] Add a capsule-payload regression: create users/targets/sources and an audit chain, append a unique random log secret and activity, run `Collect`, reopen the resulting `data/kypulse.db` and assert zero log/activity rows, zero pending pairing codes/cursors, reset usage, preserved target/source/users and a valid own audit chain. Assert the secret byte string is absent from the whole snapshot bytes, then verify live rows still exist. Run red.
- [ ] Sanitize the temporary copy using fixed table names in a transaction, then compact it:

```sql
PRAGMA secure_delete=ON;
BEGIN;
DELETE FROM activity;
DELETE FROM log_lines;
DELETE FROM log_pairing_codes;
UPDATE log_usage SET bytes=0;
COMMIT;
VACUUM;
```

Task 6 in the companion plan adds `log_cursors`; when that migration exists, add its DELETE here and extend the same regression. Close the temporary DB before `os.ReadFile`. Treat any deletion/compaction failure as snapshot failure and seal nothing. Never touch live tables and never remove kyPulse's `audit_records`/anchor.
- [ ] Run `go test ./internal/backup ./cmd/server` including existing snapshot, restore and drill tests. Document that Postgres capsules remain unsupported, as before; this task does not introduce a dump system. Update owning docs and commit locally.

## Task 6: Backend acceptance and operator contract

**Files:** `README.md`, `scripts/smoke-test.sh`, relevant package AGENTS.md and root child index.

- [ ] Document wire example and statuses, source trust/binding, permissions, retention byte semantics, and capsule exclusions. Example body:

```json
{"line":"{\"timestamp\":\"2026-09-27T12:00:00Z\",\"app\":\"kyvault\",\"level\":\"INFO\",\"event\":\"started\",\"message\":\"service started\"}"}
```

- [ ] Extend existing smoke flow to create a code as admin, claim without a session, ingest one line, read it as admin, reject viewer reads, revoke, then reject ingest. Use shell variables for credentials; never echo tokens. Existing health and backup smoke checks must still pass.
- [ ] Run `make ci` and `make test-postgres` against the configured disposable server. Record an unavailable Postgres prerequisite explicitly rather than calling it passed. `git diff --check` must pass. No frontend files change in this plan, so no frontend build artifact refresh is required beyond the existing CI gate.
- [ ] Review the complete diff against the five Review Focus cases, update closest DOX files for actual contracts, and make a local commit only. This plan ends with a working backend; proceed to the companion only after the plan-review gate and the chosen execution workflow. Push and PR creation remain a separate user instruction.
