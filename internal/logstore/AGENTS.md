# Collected Log Parsing

## Purpose
Normalize untrusted application lines into bounded storage rows, remove terminal controls for display, and define the audit sign-in burst policy.

## Local Contracts
- `Parse` limits the stored raw line and extracted fields to 16 KiB at UTF-8 boundaries. It falls back to transport time, then receive time, when application timestamps are missing or invalid. Validate application and transport timestamps after UTC normalization: only years 0000–9999 are storable. An invalid transport hint falls back to receive time.
- `Bounded` exposes the same field normalization to KyYard audit-feed imports.
- `Parse` replaces NUL with U+FFFD before applying byte caps, for storage compatibility across SQLite and PostgreSQL.
- A positive `seq`, non-empty `hash`, valid `fields` array and non-empty `action` identify an audit-shaped record for the imported Activity table. This is indexing, not chain verification.
- `ImportedApp` (`kyyard`) is reserved for KyYard's imported audit feed (rows with an external key). A line's self-declared app becomes the Activity app only when it is a plain ASCII name (`[A-Za-z0-9][A-Za-z0-9_.-]{0,63}`) other than `kyyard` in any case; otherwise it is unspecified. KyYard collection then overrides it with `container:<link>`. Sender rows keep the claimed app (the kyoffboard relay depends on it); their authenticated identity is `source_id`/`target_id`.
- `Display` removes CSI/OSC terminal escapes, controls and Unicode format (Cf) characters, including bidi overrides/isolates and zero-width characters, while preserving angle brackets. API handlers apply it to every visible field; React renders the result as text.


- `activity.go` defines the failed-sign-in policy: exact `auth.login` or `sign-in` action,
  `failure` outcome, at least five events in an inclusive five-minute window. SQL receives
  this policy from the API; matching never searches message text. The summary is a triage
  hint, not imported-chain verification or a webhook trigger.

## Verification
- `go test ./internal/logstore`
