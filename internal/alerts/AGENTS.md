# Alerts

## Purpose
The per-target state machine: consecutive-poll thresholds, transitions, hourly reminders and silences. Pure functions; no I/O.

## Ownership
Owns `Track` (persisted by the store as JSON), `Next`, `Decide` and `Measure`.

## Local Contracts
- `down` after 3 consecutive down polls, `degraded` after 2, back to `ok` after 2; a fresh target is `pending` and becomes `ok` on its first good poll without a notification.
- One notification per transition into a problem; a reminder every `ReminderEvery` while not ok, counted from the last notification. A recovery is sent only when the problem was announced (`Track.Announced`): a problem silenced from start to end produces no webhook at all.
- Flapping: `FlapAfter` (6) transitions within `FlapWindow` (1 h) replace per-transition webhooks with one `Decision.Notice` (`<from> → flapping`), an hourly reminder (`flapping → flapping`) and, once no transition has happened for `StableAfter` (15 min), one notice for the settled state (`flapping → <state>`; to `ok` only if announced). Transitions are still recorded. `Track.Changes`/`Track.Flapping` persist it in the track JSON.
- `Announced` means handed to the sender, not received: a recovery after a failed problem webhook is still sent.
- `Measure` reads a target's past day from its transitions: uptime is the share of classified (not pending) time spent ok, incidents count moves from ok or pending into degraded or down. Time while KyPulse is stopped keeps the last recorded state.
- Silence stops webhooks only: transitions are still recorded. "Until fixed" ends on a recovery from a problem, not on a fresh target's first ok; that recovery is not sent, because the problem never was.

## Verification
- `go test ./internal/alerts/`

## Child DOX Index
None.
