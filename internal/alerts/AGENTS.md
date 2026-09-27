# Alerts

## Purpose
The per-target state machine: consecutive-poll thresholds, transitions, hourly reminders and silences. Pure functions; no I/O.

## Ownership
Owns `Track` (persisted by the store as JSON), `Next` and `Decide`.

## Local Contracts
- `down` after 3 consecutive down polls, `degraded` after 2, back to `ok` after 2; a fresh target is `pending` and becomes `ok` on its first good poll without a notification.
- One notification per transition into a problem; a reminder every `ReminderEvery` while not ok, counted from the last notification. A recovery is sent only when the problem was announced (`Track.Announced`): a problem silenced from start to end produces no webhook at all.
- Silence stops webhooks only: transitions are still recorded. "Until fixed" ends on a recovery from a problem, not on a fresh target's first ok; that recovery is not sent, because the problem never was.

## Verification
- `go test ./internal/alerts/`

## Child DOX Index
None.
