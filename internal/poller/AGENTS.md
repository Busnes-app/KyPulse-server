# Poller

## Purpose
Polls every watched app's health URL on its interval and normalises the answer into `ok`, `degraded` or `down`.

## Ownership
Owns `Normalize` (the only place the spec's four reading rules live) and `Poller` (bounded worker pool, no persistence).

## Local Contracts
- `Normalize` order: `ky.health/1` document, known `status` field or boolean `healthy`, any other 2xx as `Basic`, everything else `Down` with a cause (`egress.Cause` or `status_<code>`). The fixtures in `normalize_test.go` are the suite's real answers on 2026-09-26; add a fixture when an app changes.
- `Tick` never blocks on a slow app: a target already in flight is skipped, a full pool leaves the rest for the next tick.
- `Run` returns only after in-flight polls finish, so the store may be closed after it.

## Verification
- `go test -race ./internal/poller/`

## Child DOX Index
None.
