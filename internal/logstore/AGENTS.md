# Collected Log Parsing

## Purpose
Normalize untrusted application lines into bounded storage rows, remove terminal controls for display, and define the audit sign-in burst policy.

## Local Contracts
- `Parse` limits the stored raw line and extracted fields to 16 KiB at UTF-8 boundaries. It falls back to transport time, then receive time, when application timestamps are missing or invalid.
- `Parse` replaces NUL with U+FFFD before applying byte caps, for storage compatibility across SQLite and PostgreSQL.
- A positive `seq`, non-empty `hash`, valid `fields` array and non-empty `action` identify an audit-shaped record for the imported Activity table. This is indexing, not chain verification.
- `Display` removes CSI/OSC terminal escapes and controls while preserving angle brackets. API handlers apply it to every visible field; React renders the result as text.


- `activity.go` defines the failed-sign-in policy: exact `auth.login` or `sign-in` action,
  `failure` outcome, at least five events in an inclusive five-minute window. SQL receives
  this policy from the API; matching never searches message text. The summary is a triage
  hint, not imported-chain verification or a webhook trigger.

## Verification
- `go test ./internal/logstore`
