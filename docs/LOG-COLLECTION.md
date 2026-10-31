# Log collection

[Product overview](../README.md) · [Standalone installation](../INSTALL.md)

Commands and unlinked file paths below are relative to the repository root.

## Sending logs

An admin creates a six-digit code with `POST /api/log-sources/pairing` and JSON
`{"target_id":"<watched target ID>"}`. Omit `target_id` for a source that appears only in
global log queries. The code expires after 15 minutes and can be claimed once. A sender
claims it without a user session using `POST /api/log-sources/claim` and
`{"pairing_code":"123456","name":"host-app"}`. Keep the returned `token` secret: it is
shown only on claim, and the database stores its hash. An admin can list source IDs with
`GET /api/log-sources` and revoke one with `DELETE /api/log-sources/<id>`; revocation stops
future ingestion and keeps historical rows. Browser session writes require CSRF; a claim
without a session does not.

Send UTF-8 NDJSON to `POST /api/ingest/logs` with
`Authorization: Bearer <source token>` and `Content-Type: application/x-ndjson`:

```json
{"line":"{\"timestamp\":\"2026-09-27T12:00:00Z\",\"app\":\"kyvault\",\"level\":\"INFO\",\"event\":\"started\",\"message\":\"service started\"}"}
```

Each record needs a string `line`, which may contain an original JSON line or plain text;
optional `time` is an RFC3339Nano transport timestamp and `truncated` is a boolean.
Unknown fields are accepted. Source identity and watched-target binding come from the
token, never the body; an application `app` field is an untrusted label. A bound source
appears in that target's detail queries even if its `app` label differs. An unbound source
appears in global queries. Logs shaped like audit events also create imported Activity
rows; they are indexed observations, not verified entries in KyPulse's own audit chain.

The complete request is limited to 1 MiB and 1,000 records. Decoded lines beyond 16 KiB
are cut at a UTF-8 boundary and marked truncated. Empty batches, blank records, malformed
NDJSON or times fail the whole batch with 400; oversized requests return 413 and content
types or encodings other than NDJSON and identity return 415. Invalid or revoked tokens
return 401. Each source may send 60 requests per minute; 429 includes `Retry-After`.
A 204 means the full batch committed, subject to normal retention. Pairing claims are
limited to five attempts per minute per client IP and 30 globally; 429 also includes
`Retry-After`. Admins can read `GET /api/logs` and `GET /api/activity` with descending ID
pages (`limit` defaults to 100, maximum 200; `before_id` is exclusive). Both return
`{"items":[...],"next_before_id":0}` when no next page exists; viewers get 403.

Before a dedicated ordered sender's first batch, send authenticated non-browser OPTIONS to
`/api/ingest/logs`; require 204, `Ky-Log-Receipt: ky.log-ingest/1` and `Ky-Log-Source`
with the paired source's UUID. This uses the source quota, writes no collected rows and
does not grant access to logs or administration. An older server without these headers
must be refused before sending a sequenced batch. Then add `Ky-Log-Sequence: 1` and increase by one only
after durably saving the acknowledgment. A successful response includes
`Ky-Log-Receipt: ky.log-ingest/1`, `Ky-Log-Sequence` and `Ky-Log-Digest` (lowercase
SHA-256 of the exact NDJSON request bytes). Persist the current body and sequence before
sending, and retry those same bytes if the response is lost. Only the latest matching
batch is acknowledged again without new rows. Gaps, older sequences, changed replay
bytes and writes without the header after activation return 409. Normal quotas and
validation apply to retries. One source must have one ordered writer. This option is
used by the installer's optional resident offboarding audit relay; `kypulse-send` still
uses its existing unsequenced delivery. Native release and deployment qualification remain
required before enabling this integration in production.

Receipts survive retention and recovery snapshots; collected logs and activity do not.
An acknowledgment therefore proves past acceptance, not permanent archival. A sender
ahead of a restored receipt needs explicit reconciliation rather than skipping the gap.

For offboarding failure alerts, an administrator pairs a dedicated source with
`{"target_id":"<watched target UUID>","offboarding_alerts":true}`. The source's claim
cannot change this grant or target. Its authenticated OPTIONS adds
`Ky-Log-Alerts: ky.offboarding-alerts/1`; KyQuickStart checks this with
`audit.require_alerts:true`. Each batch must contain one sequenced kyoffboard audit
projection. Failed outcomes create one native Alerts event atomically with the receipt.
The existing serial sender checks pending failures each second, including after restart,
and uses the configured webhook without changing poll health. Pause/silence/no webhook
produce explicit `paused`/`silenced`/`unconfigured` notification results. A cancelled send
stays pending; other failures use the normal retries and safe error codes. Recorded success
and receipt replay do not resend. A crash after external webhook success but before saving
its outcome may repeat the webhook. Use one monitoring service per database. SQLite
recovery snapshots retain source grants and alert events while excluding collected payloads.

### Linux sender

`make build` produces `kypulse` and `kypulse-send`. Pair a source using the admin's
six-digit code, then run one input mode with the same owner-only state directory:

```sh
kypulse-send pair --url https://pulse.example.com --code 123456 --name host-app
kypulse-send file --path /var/log/app.log
app 2>&1 | kypulse-send stdin
kypulse-send docker --container app,worker --socket /var/run/docker.sock
```

The sender requires Linux. Its default state is `$XDG_STATE_HOME/kypulse-send` or
`~/.local/state/kypulse-send`; the directory is mode 0700, the token and checkpoint are
mode 0600, and one process holds its lock. Use `--state-dir` on each command for another
source. KyPulse can revoke a source by deleting it from the admin source list, after
which delivery gets 401. TLS uses the host's normal system root certificates.

One bounded queue combines all configured inputs. It sends every 2 s or at 500 lines,
splitting earlier at the encoded 1 MiB request limit. Up to 16 MiB is held in memory;
overflow discards oldest lines and sends a `dropped N lines` marker. Checkpoints advance
only after a successful request. A crash can replay acknowledged lines when the checkpoint
was not saved; stdin and the queue are lost on restart. File input follows rename rotation
and truncation, but deleted history and an indistinguishable inode reuse can leave gaps or
duplicates. Docker resolves names to immutable IDs and follows stdout and stderr separately,
up to 32 distinct containers. It requests history inclusively from the earliest saved
Unix-second timestamp. It replays every line at the checkpoint timestamp, including
acknowledged lines, because rotation can remove an earlier line and renumber equal-time
occurrences. This can produce duplicates after sender restart. If newer lines arrive
without the saved timestamp, the sender emits a gap marker. Docker cannot prove every
history loss; malformed or oversized frames produce errors or gap markers. A TTY container
has one merged raw stdout stream. A completed Docker follow response ends that reader;
restart the sender to watch a later restart of that container. Other configured readers
continue until their own streams complete.

SIGINT/SIGTERM stops input and gives pending delivery up to five seconds to drain,
including a file's terminal partial record. A reader error uses the same bounded drain
and is returned to the operator. Expiry cancels the active request or retry wait; only
acknowledged positions remain saved. Unsent stdin and buffered records are lost on exit,
while file/Docker replay still depends on retained upstream history. Clean EOF without a
signal continues delivery and retries normally. Retry, drop and delivery-stop diagnostics
use the structured stderr logger.

Docker socket access is root-equivalent. Prefer a read-only socket proxy exposing only
`/version`, versioned container inspect and logs routes. Mounting a Docker socket with `:ro`
does not make its API read-only. The sender accepts only a local Unix socket, never a remote
Docker URL.
