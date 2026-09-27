# Monitor

## Purpose
Glues the poller, the alerts state machine and the notifier to the store: what is due, what an observation means, what is recorded, what is sent.

## Ownership
Owns `Service` (Due, Observe, Start, Drain, DueFailed, Silence, SendTest), `Webhooks` (sealed webhook config and delivery status), the `alerts.Track`/`poller.Result` JSON stored on target rows, and the monitoring log events (`target_state_changed`, `alert_sent`, `alert_send_failed`, `alert_dropped`, `alert_cancelled`, `poll_store_error`, `poll_due_failed`).

## Local Contracts
- Observe writes the poll result and the transition, then enqueues the webhook; a failed send is recorded on the event and in `alert_webhook_status`, never lost.
- Delivery runs on one sender goroutine (`Start`) fed by a bounded queue (`QueueSize`, default 64), so a slow receiver never holds a poll worker. On a full queue the newest message is dropped: `alert_dropped` is logged and the event's `notify_error` is `queue_full`. `Drain` closes the queue and waits for the sender; `cmd/server` calls it after the poller stops and before the store closes.
- Once the loop's context ends nothing more is sent: an in-flight delivery gets `ShutdownGrace` (default 5 s) then its context is cut, and everything still queued is recorded with `notify_error` `cancelled` (`alert_cancelled` logged, audited as `alert.send_failed` with `reason=cancelled`). Shutdown therefore never waits on a dead receiver.
- Each send runs on a context detached from the loop with a 60 s budget; outcomes are recorded on a context of their own so a cut-off send is still recorded. Every send and failure is an audit row (`alert.sent`, `alert.send_failed`, actor `system`).
- `RecordPoll` and `RecordEvent` are two statements; a crash between them loses at most one event row, and the next poll re-derives the state.
- The webhook is sealed under `kypulse:setting:alert_webhook`; `alert_webhook_status` is plaintext and holds no secret.
- Messages link to `AppURL/#/apps/<id>`, the app detail page.
- Due is computed in Go from `ListTargets`, not in dialect-specific SQL.
- Silence lives in the target's `silenced_until`/`until_fixed` columns, not in the track JSON: `Silence` writes only those columns, so a poll landing in between never overwrites or loses it. `Track` overlays the columns onto the decoded track for `alerts.Decide`; a recovery that clears `until_fixed` also clears the columns.
- Status, `notify_error` and audit details hold `notify.Reason(err)`, never `err.Error()`.
- An unreadable webhook (`Webhooks.Load` returns `notify.ErrUnreadable`, e.g. a rotated deployment key) is a delivery failure, not a silent no-op: logged, recorded in `alert_webhook_status`, `notify_error` `unreadable`, and audited as `alert.send_failed` with `reason=unreadable`.
- `SendTest` makes one attempt with no retries, so the answer arrives inside the HTTP request.

## Verification
- `go test -race ./internal/monitor/`

## Child DOX Index
None.
