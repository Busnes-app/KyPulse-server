# Poller

## Purpose
Polls every watched app's health URL on its interval and normalises the answer into `ok`, `degraded` or `down`.

## Ownership
Owns `Normalize` (the only place the spec's four reading rules live) and `Poller` (bounded worker pool, no persistence).

## Local Contracts
- `Normalize` order: `ky.health/1` document, known `status` field or boolean `healthy`, any other 2xx as `Basic`, everything else `Down` with a cause (`egress.Cause` or `status_<code>`). The fixtures in `normalize_test.go` are the suite's real answers on 2026-09-26; add a fixture when an app changes.
- `CertOf` reads the leaf from the poll's own handshake (`Response.TLS`, or the unverified chain in `tls.CertificateVerificationError`), records `not_after`, a 200-byte-capped issuer, `name_match` and `problem` (`cert_expired`, `cert_not_yet_valid`, `cert_name_mismatch`, `cert_untrusted`, in Go's verification order) into `Result.Cert`. A `tls://host:port` target is probed with `Handshake`; its health is reachability (ok, `Basic`, once a certificate came back, whatever is wrong with it).
- A target already in flight is skipped. `Tick` blocks on a free worker, so every due target is polled each tick and a slow app delays only the targets behind it. A failed `Due` goes to `OnError` (nil-safe).
- `Run` returns only after in-flight polls finish, so the store may be closed after it.

## Verification
- `go test -race ./internal/poller/`

## Child DOX Index
None.
