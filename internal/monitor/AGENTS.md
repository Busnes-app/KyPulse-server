# Monitor

## Purpose
Glues the poller, the alerts state machine and the notifier to the store: what is due, what an observation means, what is recorded, what is sent.

## Ownership
Owns `Service` (Due, Observe, Recent, Start, Drain, DueFailed, Silence, SendTest), `Webhooks` (sealed webhook config and delivery status), `Mail` (sealed KyPost channel and its status), the `alerts.Track`/`poller.Result` JSON stored on target rows, and the monitoring log events (`target_state_changed`, `alert_sent`, `alert_send_failed`, `alert_dropped`, `alert_cancelled`, `poll_store_error`, `poll_due_failed`).

## Local Contracts
- Observe writes the poll result and the transition, then enqueues the webhook; a failed send is recorded on the event and in `alert_webhook_status`, never lost. An observation whose URL no longer matches the stored target (edited while the poll was in flight) is dropped. Editing the URL also ends an "until fixed" silence.
- The same serial sender checks one durable pending offboarding failure per second. Native
  ingestion creates these events independently of poll health; restart consumes retained
  pending work. Timed/until-fixed silence and disabled targets retain the event with
  `silenced`/`paused`; absent webhook records `unconfigured`. Delivery otherwise uses the
  normal retry, status and keyed audit path. Shutdown keeps interrupted offboarding delivery
  pending for restart. A crash after a webhook succeeds but before its outcome commits may
  repeat that webhook; alert rows and receipt replay remain deduplicated. Run one monitoring
  service per database; this is not an exactly-once webhook protocol.
- Delivery runs on one sender goroutine (`Start`) fed by a bounded queue (`QueueSize`, default 64), so a slow receiver never holds a poll worker. On a full queue a reminder is dropped (`alert_dropped` logged, the event's `notify_error` `queue_full`); any other message is held, newest per target, and queued by the sender as room frees up, so a recovery is never dropped for reminders. While a target has a held message its later ones wait behind it (a superseded one is recorded `queue_full`), keeping each target's messages in order. A flapping `Decision.Notice` is its own event, recorded after the transition, and is what the webhook reports. `Drain` closes the queue and waits for the sender; `cmd/server` calls it after the poller stops and before the store closes.
- Once the loop's context ends nothing more is sent: an in-flight delivery gets `ShutdownGrace` (default 5 s) then its context is cut, and everything still queued is recorded with `notify_error` `cancelled` (`alert_cancelled` logged, audited as `alert.send_failed` with `reason=cancelled`). Shutdown therefore never waits on a dead receiver.
- Each send runs on a context detached from the loop with a 60 s budget; outcomes are recorded on a context of their own so a cut-off send is still recorded. Every send and failure is an audit row (`alert.sent`, `alert.send_failed`, actor `system`).
- `RecordPoll` and `RecordEvent` are two statements; a crash between them loses at most one event row, and the next poll re-derives the state.
- The webhook is sealed under `kypulse:setting:alert_webhook`; `alert_webhook_status` is plaintext and holds no secret.
- Messages link to `AppURL/#/apps/<id>`, the app detail page.
- Certificates run a second track (`targets.cert_track`, `CertTrack`) beside health, through `alerts.NextCert` and `Decide`, sharing the target's silence; it runs while a target has ever shown a certificate, so reminders continue through a poll without one. Its events and messages use `cert_ok|cert_warning|cert_critical|cert_failed`; a fresh certificate settling on ok is not recorded. Each target has a health and a certificate delivery lane in the overflow map, so one never supersedes the other. Thresholds live in one `cert_thresholds` setting (`Thresholds`, `SetThresholds`: 1 ≤ critical < warn ≤ 365). An observation whose SNI no longer matches is dropped like an edited URL.
- Due is computed in Go from `ListTargets`, not in dialect-specific SQL.
- `Recent` keeps each target's last `Beats` (24) raw poll answers in memory for the Status heartbeat strip; a restart empties it.
- Silence lives in the target's `silenced_until`/`until_fixed` columns, not in the track JSON: `Silence` writes only those columns, so a poll landing in between never overwrites or loses it. `Track` overlays the columns onto the decoded track for `alerts.Decide`; a recovery that clears `until_fixed` also clears the column through `ClearUntilFixed`, which leaves a timed silence set meanwhile in place; a failure is logged as `poll_store_error`.
- Status, `notify_error` and audit details hold `notify.Reason(err)`, never `err.Error()`.
- An unreadable webhook (`Webhooks.Load` returns `notify.ErrUnreadable`, e.g. a rotated deployment key) is a delivery failure, not a silent no-op: logged, recorded in `alert_webhook_status`, `notify_error` `unreadable`, and audited as `alert.send_failed` with `reason=unreadable`.
- `Mail` ("Email through KyPost", nil-safe) is a second channel on the same sender: every delivery the webhook gets also goes to KyPost under event ID `kypulse-alert-<event id>`, retried over the webhook backoff on `kypost.ErrRetry`. Its config (`{owner_id, recipients}`) is sealed under `kypulse:setting:alert_kypost` (`alert_kypost_enc`); `alert_kypost_status` holds only the last `kypost.Reason`. The owner must be `Eligible` (active, KySignOn subject) at send time, else `owner_unavailable`. Audits are `alert.mail_sent|alert.mail_failed` with target, state and reason; with no webhook configured the event's `notified`/`notify_error` carry the mail outcome. Never an address.
- `SendTest` makes one attempt with no retries, so the answer arrives inside the HTTP request.

## Verification
- `go test -race ./internal/monitor/`

## Child DOX Index
None.
