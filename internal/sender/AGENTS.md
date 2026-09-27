# Sender

## Purpose

Pairs a source and delivers bounded NDJSON batches with acknowledged input positions.

## Ownership

`pair.go` claims a source. `state.go` owns owner-only state, token and Linux lifetime lock.
`sender.go` owns queueing, overflow markers, retry and checkpoint transitions. `file.go`
and `stdin.go` feed `Item` values into `Run`; Docker input is a later task.

## Local Contracts

- `PositionKey` identifies one configured stream. Readers clip lines to 16 KiB while
  draining any remainder, then give `Run` a complete position for each line.
- File checkpoints use absolute path, device, inode and consumed byte offset. The reader
  finishes a renamed open file before switching to its replacement; a restart searches
  siblings for the saved inode. Truncation resets to zero. A truncate and regrow wholly
  between polls can evade size/inode detection.
- Stdin and the outage queue live in memory and cannot survive a crash. File and Docker
  checkpoints replay only while upstream history exists. Overflow deliberately drops
  records and emits a marker, so delivery has no unconditional no-loss guarantee.
- `Run` freezes one batch per request. A 2xx acknowledges its records and marker; only then
  does it atomically save positions. A failed save stops delivery, allowing replay.
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
