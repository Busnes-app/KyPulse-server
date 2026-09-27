# Collected Log Parsing

## Purpose
Normalize untrusted application lines into bounded storage rows and remove terminal controls for display.

## Local Contracts
- `Parse` limits the stored raw line and extracted fields to 16 KiB at UTF-8 boundaries. It falls back to transport time, then receive time, when application timestamps are missing or invalid.
- A positive `seq`, non-empty `hash`, valid `fields` array and non-empty `action` identify an audit-shaped record for the imported Activity table. This is indexing, not chain verification.
- `Display` removes CSI/OSC terminal escapes and controls while preserving angle brackets. API handlers apply it to every visible field; React renders the result as text.

## Verification
- `go test ./internal/logstore`
