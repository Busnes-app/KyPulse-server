# Monitor

## Purpose
Glues the poller, the alerts state machine and the notifier to the store: what is due, what an observation means, what is recorded, what is sent.

## Ownership
Owns `Service` (Due, Observe, Silence, SendTest), `Webhooks` (sealed webhook config and delivery status), the `alerts.Track`/`poller.Result` JSON stored on target rows, and the monitoring log events (`target_state_changed`, `alert_sent`, `alert_send_failed`, `poll_store_error`).

## Local Contracts
- Observe writes the poll result and the transition before sending; a failed send is recorded on the event and in `alert_webhook_status`, never lost.
- Sends run on a context detached from the poll with a 60 s budget; every send and failure is an audit row (`alert.sent`, `alert.send_failed`, actor `system`).
- The webhook is sealed under `kypulse:setting:alert_webhook`; `alert_webhook_status` is plaintext and holds no secret.
- Messages link to `AppURL/#/apps/<id>`; step 2c makes that route exist.
- Due is computed in Go from `ListTargets`, not in dialect-specific SQL.

## Verification
- `go test -race ./internal/monitor/`

## Child DOX Index
None.
