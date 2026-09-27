# kyPulse Sender, Screens and KyYard Collection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship `kypulse-send`, admin Logs/Activity screens, and the deferred KyYard log/audit collectors on the log backend.

**Architecture:** The sender uses a bounded in-memory queue, one delivery worker and per-input acknowledged checkpoints. React screens call the admin APIs from PR A. KyYard collection uses the existing guarded client and commits imported rows with durable cursors; it remains independent of health polling.

**Tech Stack:** Existing Go standard library, ky-primitives keyfile/logging, React/TypeScript, Vitest and Playwright. Use the Docker HTTP API directly over a Unix socket; no Docker SDK dependency.

**Spec:** `docs/superpowers/specs/2026-09-26-kypulse-design.md`, sections 3 and 4 and Screens. Depends on [Logs Backend](2026-09-27-kypulse-logs-backend.md), which defines source pairing, wire records, storage and read interfaces.

## Global Constraints

- Binaries: “kypulse” and “kypulse-send”.
- “A batch is sent every 2 s or at 500 lines, whichever comes first.” Also split at the encoded 1 MiB server request cap.
- “Delivery is at-least-once”; positions advance only after acknowledgment.
- “Up to 16 MiB of lines are held in memory”; excess drops oldest and produces a “dropped N lines” marker.
- File input “follows a file across rotation and truncation”. Docker reads “stderr and stdout”.
- KyYard logs: “only for containers linked to a watched app”, “capped at 1,000 lines per container per pull”.
- “Audit feed rows since the last seen ID go into activity.”
- “Logs and Activity are admin-only”; text is rendered as plain text with control characters and ANSI escapes stripped.
- “Bursts of failed sign-ins are highlighted.” No security-event webhook alerts or imported chain verification.
- `web/dist` must be rebuilt and committed; vendored `web/src/ky-ui/` stays unchanged. Run Playwright for all changed form labels.
- No pushes, PR creation or merge unless asked. Any KyYard work uses an isolated worktree based on its current remote master; preserve `/home/yoshi/git/busnes.app/KyYard-Server` and the other session's commits.

## Review Focus

1. An acknowledged network batch followed by a failed checkpoint write must replay safely, never silently skip (tasks 1–3).
2. Rename rotation, copy-truncate, Docker framing and repeated timestamps must not mix independent input positions (tasks 2–3).
3. Outage overflow, 429 and encoded-size expansion must stay bounded while preserving an honest drop marker (task 1).
4. Viewer deep links, role changes and injected HTML/control sequences must not reveal or execute logs (task 4).
5. KyYard re-pairing, slow audit transactions and capped log history must not silently advance a cursor past uncommitted/missing data (tasks 5–6).

## File ownership and sequence

| Task | Files |
|---|---|
| 1 | New `internal/sender/{sender.go,state.go,sender_test.go,state_test.go,AGENTS.md}`, `cmd/kypulse-send/main.go` |
| 2 | New `internal/sender/{file.go,file_test.go,stdin.go,stdin_test.go}` |
| 3 | New `internal/sender/{docker.go,docker_test.go}`; `Makefile`, `.github/workflows/ci.yml`, `README.md` |
| 4 | New `web/src/logs.ts`, `pages/Logs.tsx`, `pages/Activity.tsx`, `components/LogSources.tsx`, corresponding tests, `web/browser/logs.spec.mjs`; modify `web/src/App.tsx`, `pages/AppDetail.tsx`, `web/dist`, web DOX |
| 5 | Separate KyYard prerequisite: `internal/api/tenant_handlers.go`, `internal/store/{store.go,tenant_access.go,sqlstore.go}`, existing/new tenant audit tests and owning DOX |
| 6 | New `internal/kyyard/{logs.go,logs_test.go}`, modify `client.go`, `service.go`, `cmd/server/kyyard.go`, store cursor persistence/migration, `internal/backup/payload.go`, corresponding tests and DOX |

Tasks 1–4 can ship against PR A. Task 5 is a separate upstream change; task 6 audit collection depends on that contract. This is three reviewable change sets in total: backend, sender/screens, and KyYard collection with its upstream prerequisite. The handoff's two-PR split was a suggestion; the missing cursor API warrants a separate integration change.

## Task 1: Pairing, delivery, queue and acknowledged state

**Interfaces:** Shared `internal/ingest.Record` is the wire object defined by PR A. Add:

```go
// internal/sender
type Position struct {
    Kind string `json:"kind"` // file, docker, stdin
    Input string `json:"input"`
    Device, Inode uint64 `json:"device,omitempty"`
    Offset int64 `json:"offset,omitempty"`
    Timestamp string `json:"timestamp,omitempty"`
    Ordinal int `json:"ordinal,omitempty"`
}
type Item struct { Record ingest.Record; Position Position }
type State struct {
    URL, SourceID string
    Positions map[string]Position
}
type Sender struct {
    HTTP *egress.Client
    StateDir string
    Token []byte
    State State
}
func (s *Sender) Run(ctx context.Context, input <-chan Item) error
func LoadState(dir string) (State, []byte, error)
func SaveState(dir string, state State) error
```

Use JSON tags on State (`url`, `source_id`, `positions`). Token lives separately at `<state-dir>/token` using `keyfile.Store(..., keyfile.Hex)`/`Load(...,32)`; mutable `<state-dir>/positions.json` contains no token. Both are owner-only under a 0700 directory. The token bytes hex-encode to the Bearer returned by PR A. Reject unsafe existing state/token paths; do not replace unreadable credentials with new ones. Default state dir: `$XDG_STATE_HOME/kypulse-send`, otherwise `$HOME/.local/state/kypulse-send`. Each running sender owns its own state dir. Take a process-lifetime OS lock on that dir before pairing or starting (Linux is the supported v1 platform); fail clearly on a second process rather than sharing mutable positions.

CLI shape:

```text
kypulse-send pair --url https://pulse.example --code 123456 --name host-a --state-dir /var/lib/kypulse-send
kypulse-send file --path /var/log/app.log --state-dir /var/lib/kypulse-send
kypulse-send docker --container a,b --socket /var/run/docker.sock --state-dir /var/lib/kypulse-send
kypulse-send stdin --state-dir /var/lib/kypulse-send
```

- [ ] Write state tests for unsafe/permissive token file, atomic state replacement, corrupt JSON and simultaneous state-dir owners. Use keyfile as-is for credential persistence. Pair validates URL with the existing egress client (HTTPS, no redirects); code/name validation matches PR A. Set explicit request timeouts. Print success without the token. If the remote claim succeeds but local save fails, print a revoke-and-pair-again instruction, never the token.
- [ ] Test delivery against a fake transport/server: 2-second flush, 500-line flush, encoded-size flush, cancellation, 429 Retry-After seconds/date (bounded to 5 minutes), 5xx/backoff and dropped response after server commit. Kernel for encoded-size splitting:

```go
record := ingest.Record{Line: strings.Repeat("\x00", ingest.MaxLineBytes)}
encoded, err := json.Marshal(record)
if err != nil { t.Fatal(err) }
if len(encoded) <= ingest.MaxLineBytes { t.Fatal("fixture must expand on the wire") }
// Enqueue 500 copies through Run; fake receiver rejects any body above MaxRequestBytes.
// Assert all 500 arrive and each acknowledged position survives LoadState.
```

Use injected clock/timer and HTTP transport in package tests so the test does not sleep for minutes; keep production API concrete. Run `go test ./internal/sender` red.
- [ ] Implement queue byte accounting for retained raw payload and metadata; include the frozen in-flight batch within the 16 MiB budget. Readers clip one application line to 16 KiB while draining its remainder so a huge input never allocates unbounded memory. One delivery worker freezes a batch and its positions; new arrivals may drop oldest queued items, never mutate an in-flight request. Retry the same batch on transient failures with jittered capped backoff, respecting Retry-After. 400/401/403/413/415 stop with a fixed diagnostic and leave positions unadvanced; operator must fix configuration/protocol. Any 2xx acknowledges the whole batch.
- [ ] Overflow creates a pending marker count; snapshot its count with a batch and subtract only that snapshot after success, preserving new drops during the request. Include dropped-input positions when acknowledging the marker: they are the deliberately discarded data, and must not be resurrected indefinitely on every restart. Keep one latest dropped position per configured input (bounded input set). Never checkpoint unsent surviving items.
- [ ] Save checkpoint with temp file in the same directory, mode 0600, Sync, Close, Rename, directory Sync. Persist positions only after acknowledgment; on save failure stop instead of consuming more. Replaying after an ambiguous acknowledgment is expected. Test this by reopening State after a deliberately failed save and confirming the old offset. Log fixed events through ky-primitives logging on stderr.
- [ ] Run `go test -race ./internal/sender` and `go build ./cmd/kypulse-send`; update new-package/root DOX and commit locally.

## Task 2: File and stdin inputs

**Interfaces:** `ReadFile(ctx context.Context, path string, start Position, out chan<- Item) error` and `ReadStdin(ctx context.Context, r io.Reader, out chan<- Item) error` in `internal/sender`. Input owns channel closure in the CLI; input functions never close a shared channel. Position.Input uses an absolute file path or `stdin`. The CLI cancels and joins readers on terminal delivery failure.

- [ ] Write real-temp-file tests with an emitted-items channel: starting offset, half-written line completed later, rename plus replacement, copy-truncate, inode reuse/device mismatch, file temporarily missing, and oversized line followed by a good line. The good line's offset must equal bytes actually consumed, not truncated bytes emitted. Run `go test ./internal/sender -run 'File|Stdin'` red.
- [ ] Implement file following with bounded `bufio.Reader.ReadSlice` fragments and `os.Stat`/`File.Stat`; store `(device,inode,offset)` after the newline (or terminal record policy below). Keep reading an opened old inode through rename until EOF, then open the replacement at zero. If the current file shrinks beneath the read offset, restart at zero as a new observed generation. On restart, search siblings for the saved inode to finish a renamed file before switching to the configured path. If rotation deleted/compressed it, emit a gap marker and begin at zero in the replacement; never reuse an offset on a different inode. Poll at 250 ms with cancellation. Document that a truncate-and-regrow between observations cannot always be detected by inode/size alone.
- [ ] Handle incomplete records explicitly: file mode holds a partial line until completed, rotation, or orderly shutdown; stdin EOF flushes its final unterminated line. Cancel unblocks source goroutines (CLI may close its owned stdin descriptor). A 16 KiB prefix plus discarded remainder is one truncated Item. Invalid input bytes become valid replacement characters before the wire encoding, bounded again after replacement.
- [ ] Pin the rotation behavior with this fixture sequence inside the test:

```go
if err := os.Rename(path, path+".1"); err != nil { t.Fatal(err) }
if _, err := old.WriteString("old-tail\n"); err != nil { t.Fatal(err) }
if err := os.WriteFile(path, []byte("new-head\n"), 0600); err != nil { t.Fatal(err) }
// Consume both records. Ack/reopen after old-tail and verify new-head is still delivered.
```

Create `path`, open `old`, and start ReadFile before the rename. Use a deadline on the consume operation so failure cannot hang CI.
- [ ] State the durability limit accurately: stdin and the outage queue are in memory and cannot survive a crash; file/Docker positions are replayable only while upstream history exists. The spec's overflow policy intentionally drops data. Do not advertise an unconditional no-loss guarantee.
- [ ] Run `go test -race ./internal/sender`, then a CLI stdin-to-fake-ingest smoke covering final partial-line flush. Commit locally.

## Task 3: Docker input and distribution

**Interfaces:** `ReadDocker(ctx context.Context, socket, container string, start Position, out chan<- Item) error`. Limit `--container` to 32 distinct nonempty names, resolving to immutable container IDs; checkpoints key by ID and stream. Use a local `http.Transport.DialContext` dialing only the configured Unix socket. This is local Docker IPC, not an exception for remote network egress.

- [ ] Add a Unix-socket fake Docker server serving container inspect and logs. Fixtures cover TTY raw bytes and multiplexed 8-byte headers split across reads, stdout/stderr, zero-length frames, malformed/oversized frame lengths, HTTP refusal and cancellation. Drain oversized frame payloads incrementally rather than allocate their declared size. Capture outgoing query parameters.
- [ ] Implement GET container inspect and logs with `stdout=1&stderr=1&timestamps=1&follow=1&since=<checkpoint>`; negotiate/use a supported API version from Docker's version endpoint. Check response status before decoding. Reuse a connection for streaming and cancel via context; use a bounded header timeout, not a whole-stream timeout. This privileged local socket capability is exclusive to sender Docker input.
- [ ] Strip Docker's RFC3339Nano timestamp prefix before constructing `ingest.Record`, retaining it as transport Time. Resume inclusively at the last timestamp, preserving per-stream counts for equal timestamps (extend Position with a stream field here). Test three identical messages sharing a timestamp, restart after acknowledging the first, and ensure the remaining two are delivered. Do not globally dedupe by content hash: identical messages can be real separate events. If replay ordering/history is uncertain, permit duplicates and emit a gap indicator when history is unavailable; do not skip on a guessed cursor.
- [ ] Wire independent readers to the shared bounded queue; one source's cursor never advances when only another source's batch is acknowledged. Test one busy container and one quiet container with a failed intermediate HTTP request.
- [ ] Add Makefile build output for both binaries and a Linux static-build CI check:

```bash
CGO_ENABLED=0 go build -o /tmp/kypulse-send ./cmd/kypulse-send
go test -race ./internal/sender
```

Document Linux support, all three modes, source revocation, owner-only state, 2 s/500-line/encoded-byte batching, memory/drop behavior, TLS trust through normal system roots, and file/history limitations. Docker socket access is root-equivalent; recommend a read-only socket proxy with only version/inspect/log routes. A socket mounted `:ro` does not make its API read-only. Do not add arbitrary remote Docker URL support.
- [ ] Run `make ci` and the static build; commit locally.

## Task 4: Admin Logs, Activity, sources and recent app lines

**Interfaces:** `web/src/logs.ts` owns checked JSON decoding and typed API helpers for PR A endpoints. Use `secureFetch` for source writes. `Logs` and `Activity` use hash routes `#/logs` and `#/activity`; app-detail recent lines query `/api/logs?target_id=<id>&limit=20` only for admins. Keep raw log text out of viewer-readable monitor payloads.

- [ ] Add UI tests before implementation for filters, load-more pagination, errors/retry, paired/revoked sources, and role-gated rendering. Add an explicit viewer app-detail test proving no log request is made. Test text `<img src=x onerror=alert(1)>` remains text and does not create an img element. Run the focused Vitest tests red.
- [ ] Implement Logs with app, level, UTC time-range and literal text filters, explicit Apply/Clear, a bounded list and Load more. Reset cursor on filter changes; cancel/ignore stale fetches. Show source, event time, receive time when different, level, message and truncated marker; raw text can expand within the same plain-text renderer. Keep request failures distinct from empty results.
- [ ] Add LogSources within Logs: optional watched-app binding, Add source, expiring code plus exact pair command, source list and explicit revoke confirmation. Never display a source token. Clear the code on expiry/unmount; it is not localStorage data. Forms use unique accessible labels, for example “Log source name”, not an ambiguous generic “Name”. Binding is chosen before generating the code; name is supplied by the sender.
- [ ] Implement Activity filtered by app/actor/outcome, with action/target/IP/time. Use the actual parser vocabulary: recognize failed sign-ins by known `action`/`event` names and failure outcome, not arbitrary text matching. Proposed highlight rule: at least 5 failures for the same `(app,actor)` or `(app,IP)` in a rolling 5-minute window, computed over retained activity by the backend. Empty actor/IP does not group all anonymous events globally. Mark as a screen-only triage hint; no webhook or security verdict.
- [ ] Add a bounded backend activity summary for this rule in `internal/logstore/activity.go`, store query and `internal/api/logs.go`, with tests for 4 versus 5, exact 5-minute boundary, cross-app separation and a page boundary. Expose `{bursts:[{app,actor,ip,count,from,to}]}` in `/api/activity` using the same filters and time bounds, so pagination cannot hide a burst. Limit returned groups to 100, ordered newest first. The empty-state text is “no audit events: not on shared logging” when the selected app has no retained audit events; mention the 7-day retention window so this is not a claim about lifetime activity.
- [ ] Gate navigation and route rendering on `role === 'admin'`; clear data when role/session changes. Both a deep link and a manual API request as viewer are denied. Keep Settings, Status and Alerts behavior as it is. Render strings with JSX text only; API sanitization is shared with app detail. Do not insert stored HTML.
- [ ] Extend the browser fixture to pair/ingest sample log and audit records, and test every new workflow. Acceptance kernel for an authenticated viewer:

```js
await page.goto('/#/logs');
await expect(page.getByRole('heading', { name: 'Logs', exact: true })).toHaveCount(0);
const result = await page.request.get('/api/logs');
expect(result.status()).toBe(403);
```

Use the existing browser setup's real admin/viewer accounts and CSRF helpers. Add Activity deep-link denial, app-detail denial, XSS/control text, 5-failure burst, code/revoke, error and filter flows. Preserve the existing exact webhook URL selector.
- [ ] Run `npm test` and `npm run build` in `web`, rebuild `.browser/server`, then run the entire `npm run test:browser`. Commit generated `web/dist` and update web/API DOX. Run `make ci` at the complete sender/screens boundary. Commit locally.

## Task 5: KyYard audit cursor prerequisite (separate repository)

The current organization audit endpoint returns a newest-first array with offset/limit, maximum 200. Silently sending `after_id` today would be ignored. Implement the cursor contract upstream before enabling audit pulls. This task requires reading that repo's full DOX chain and inspecting all audit append paths in the execution worktree.

**Proposed contract:** `GET /api/organizations/{organization}/audit?after_id=0&limit=200` returns `{items:[...],next_after_id:N}` ordered ascending by ID; preserve old array behavior when `after_id` is absent. The existing `pulse_reader` audit permission and organization/environment filters remain authoritative. Invalid/negative after_id, repeated parameters or mixing offset with after_id are 400. Empty page echoes the supplied cursor. Use a response object so kyPulse can detect older servers.

- [ ] Create an isolated worktree from current remote master using the worktree skill; do not switch/reset the main checkout. Pin tests to the new cursor contract and existing legacy behavior. Add `ReadAuditAfter(ctx context.Context, a TenantAccess, afterID int64, limit int) ([]AuditRecord,error)` to the tenancy store; scope SQL through `readTenant` as existing `ReadAudit` does:

```sql
SELECT id,user_id,action,resource,details,ip_address,created_at,
       scope,organization_id,environment_id,correlation_id,result
FROM audit_records
WHERE scope='organization' AND organization_id=?
  AND (?='' OR environment_id=?) AND id>?
ORDER BY id ASC LIMIT ?
```

- [ ] Add the concurrency regression before implementing: hold transaction A after it has allocated a lower audit ID, commit transaction B with a higher ID, read one cursor page, commit A and read next. Both records must eventually appear. Plain `id > last_seen` over unconstrained PostgreSQL sequence allocation fails this test.
- [ ] Serialize **every** audit insert before ID allocation until transaction commit (Postgres transaction advisory lock shared by all append sites, SQLite's existing single-writer transaction). Route tenant `auditTenant`, generic `auditStore.LogAudit`, and credential-change insert through that discipline. Acquire locks in a consistent order with their user/tenant locks; review all call sites for an inverse ordering and test concurrent password change, tenant mutation and service-read summary. Existing atomic audited mutations must stay atomic. Do not “fix” the test with an arbitrary time overlap; late commits can exceed it.
- [ ] Implement the cursor handler and tests for >200 rows, equal timestamps, concurrent appends, no rows, foreign organization IDs, viewer/service permissions and revoked tokens. `next_after_id` is the last visible row, never an ID belonging to another organization. No new mutating authority for `pulse_reader`.
- [ ] Run KyYard's local CI and PostgreSQL race tests under its own instructions. Record the upstream commit/PR as the dependency when publication is authorized. Until the upstream release is available, kyPulse audit collection reports `audit_cursor_unsupported`, preserves its cursor and keeps health/logs working. No changes to the separate checkout's unpushed commits.

## Task 6: KyYard log and audit collection

**Interfaces:** Extend the existing `kyyard.Client` with:

```go
type LogPage struct { Lines []ingest.Record; Notice string; Capped bool }
type AuditPage struct { Items []AuditEvent; NextAfterID int64 }
type AuditEvent struct {
    ID int64 `json:"id"`
    UserID string `json:"user_id"`
    Action string `json:"action"`
    Resource string `json:"resource"`
    Result string `json:"result"`
    IPAddress string `json:"ip_address"`
    CreatedAt time.Time `json:"created_at"`
}
func (c *Client) Logs(ctx context.Context, endpointID, containerID, since string) (LogPage,error)
func (c *Client) AuditAfter(ctx context.Context, afterID int64) (AuditPage,error)
```

Add `log_cursors` in migration 9; key by pairing generation ID plus stream kind and endpoint/container identity. Add a random generation ID to the sealed KyYard pairing whenever it is replaced (migrate old pairings to a stable generated ID once). Token/URL must not appear in cursor keys or diagnostics. Persist cursors with their imported rows, not through unrelated Settings writes. Add to `LogStore`:

```go
type LogCursor struct { Key, Value string }
Cursor(ctx context.Context, key string) (string,error)
AppendImported(ctx context.Context, batch LogBatch, previous, next LogCursor, maxBytes int64) error
```

`AppendImported` compare-and-swaps the prior cursor in the same transaction as rows/pruning; a stale worker fails without appending. Audit ExternalKey is `pairing-generation:organization:remote-audit-id`, not a log-content hash. Logs permit at-least-once replay duplicates. Exclude cursors from backups and reset them on restore, as specified in backend task 5.

- [ ] Add fake-KyYard tests for text/plain log responses (the existing JSON `get` cannot parse these), bearer headers, URL escaping and `since`/`tail=1000&timestamps=1&follow=0`. Parse server notices only when their token matches `X-KyYard-Notice-Token`; a container's forged notice remains a log. Never interpret a notice as an application audit record.
- [ ] Give log pulls a separately configured guarded client with a 32 MiB response cap and bounded whole-request timeout, preserving the small inventory/audit caps. This admits 1,000 capped lines plus timestamp/notice overhead. Parse timestamped lines incrementally from bounded response bytes, cap raw lines at 16 KiB, and keep only 1,000 application lines. A too-large response is an error: cursor remains unchanged. If KyYard splits an overlong line or reports dropped bytes, retain a collector notice so operators see the discontinuity.
- [ ] Add persistent-cursor failure tests: insert failure, cursor compare mismatch, crash after commit and re-pair while a request is blocked. Re-pair/unpair must prevent a previous-generation batch from committing after the change; couple generation validation with import commit under the existing service lifecycle lock (no network calls under that lock). Do not rely only on checking the generation before the network request.
- [ ] Collect only resolved linked watched containers; use stable target IDs for attribution. When several watched targets link the same container, fetch once and create an attributed row for each linked target in the same imported batch; byte accounting includes each stored copy. Test two linked targets and assert one HTTP fetch, two attributed copies and one cursor advance. Disabled health polling does not erase a link or already collected history.
- [ ] Docker/KyYard tail is newest N, not a lossless forward page. When a response fills the 1,000-line cap, record “history may be incomplete: pull reached 1000 lines” and preserve last timestamp inclusive for the next pull. Do not promise catch-up without gaps at higher throughput. Keep equal-timestamp duplicates rather than guessing which identical line was seen. Ask operators needing complete high-volume collection to use file/Docker sender input or their existing external collector.
- [ ] For audit, request 200 rows ascending, at most 5 pages per collection tick, atomically advancing each page. Validate increasing IDs and exact `next_after_id`, reject old array responses with `audit_cursor_unsupported`. Persist a partial catch-up page before the next request; a failure repeats only uncommitted work. Import fields as app `kyyard`, actor UserID, target Resource, outcome Result, IP IPAddress; set `ExternalKey` and do not verify the remote chain.
- [ ] Run collection every 60 s on a separately owned worker so a stalled log fetch does not delay inventory snapshots or health checks. Cancellation is joined before closing the store. Keep separate inventory/log/audit last-success and fixed error reasons; show each collector's stale age in admin Logs/Activity and KyYard settings. The existing inventory success must not clear a log/audit failure. One container's failure does not suppress other containers or audit pulls.
- [ ] Run `go test -race ./internal/kyyard ./internal/store ./internal/backup ./internal/api ./cmd/server`, PostgreSQL import/CAS tests, `make ci`, and frontend browser regressions after collection status UI changes. Update `internal/kyyard/AGENTS.md` to replace “Nothing KyYard-derived is written to the database” with the actual cursor/log/activity contract; retain in-memory inventory/sample behavior. Commit locally.

## Completion and scope boundary

- [ ] Verify every section 4 requirement against the backend/sender/screens tests and both deferred section 3 pulls against integration tests. Re-run full CI only after the final relevant changes. Record unavailable services/checks accurately.
- [ ] Update README and DOX for the exact shipped behavior; neither app `/healthz` adoption nor the parked minor issues are part of these changes.
- [ ] Mirror the execution handoff (results, remaining checks/dependencies and hazards) to `kypulse-server-build` via myslop. Keep the durable copy locally. Publication/merge remains Yoshi's decision.
