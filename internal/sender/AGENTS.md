# Sender

## Purpose

Pairs a source and delivers bounded NDJSON batches with acknowledged input positions.

## Ownership

`pair.go` claims a source from an HTTPS origin without query/fragment delimiters.
`state.go` owns owner-only state, token and Linux lifetime lock.
`sender.go` owns queueing, overflow markers, retry and checkpoint transitions. `file.go`,
`stdin.go` and `docker.go` feed `Item` values into `Run`.

## Local Contracts

- `PositionKey` identifies one configured stream. Readers clip lines to 16 KiB while
  draining any remainder, then give `Run` a complete position for each line.
- File checkpoints use absolute path, device, inode, consumed byte offset and a hash of
  the last 64 consumed bytes. A restart validates that hash before seeking and searches
  siblings for a matching renamed file; missing or mismatched history replays from zero
  with a gap marker. Truncation resets to zero. Identical suffixes and a truncate and
  regrow wholly between polls remain indistinguishable to this observer.
- Docker resolves each configured name to an immutable container ID. Checkpoints key by
  ID and stdout/stderr stream, with an inclusive timestamp and recorded equal-time ordinal
  for each stream. Boundary lines replay even when acknowledged, because Docker cannot
  verify the historical prefix after rotation. A missing checkpoint timestamp emits a
  gap marker once a newer line appears. TTY logs are a single raw stdout stream. A clean
  follow EOF completes that reader; the sender exits after all configured readers finish.
- Stdin and the outage queue live in memory and cannot survive a crash. File and Docker
  checkpoints replay only while upstream history exists. Overflow deliberately drops
  records and emits a marker, so delivery has no unconditional no-loss guarantee.
- `Run` freezes one batch per request. A 2xx acknowledges its records and marker; only then
  does it atomically save positions. A failed save stops delivery, allowing replay.
  Every exit cancels and joins the active delivery worker before releasing the state lock.
- The queue and in-flight batch share a 16 MiB budget. Overflow drops oldest queued
  records and sends a marker with the latest position of each discarded stream.
- Production HTTP uses `egress.Client` with HTTPS, guarded destinations, no redirects and
  an explicit request timeout. The `HTTP` interface exists for deterministic transport tests.
- Each sender process owns one 0700 state directory and its Linux flock. `token` is a
  keyfile-managed 0600 hex secret; `positions.json` is a separate 0600 atomic checkpoint.

## Verification

`go test -race ./internal/sender` and `go build ./cmd/kypulse-send`.

## Child DOX Index

None.
