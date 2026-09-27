# KyYard

## Purpose
Pairs kyPulse to one KyYard organization and reads what a `pulse_reader` may: endpoints, container inventory and resource samples.

## Ownership
`Pairing` (sealed URL, organization and token in `server_settings.kyyard_enc`), `Client` (bearer reads over the egress guard), `Claim` (pairing-code exchange), and the `Service` snapshot loop.

## Local Contracts
- The token and the pairing code never reach a log line, an audit row or an HTTP response; `Reason(err)` is the only text stored about a failure.
- `Claim` posts `{pairing_code, service_name:"kypulse"}`; 403 is `ErrPairingRefused` (reason `pairing_refused`), 429 `ErrRateLimited` (`rate_limited`); a code that is not six digits is refused before any request.
- Reads carry `Authorization: Bearer`; 401 is `ErrUnauthorized` (`unauthorized`), other non-2xx `status_NNN`, transport failures the egress vocabulary. `cmd/server` builds the egress client with a 2 MiB body cap (KyYard snapshots are at most 1 MiB).
- Nothing KyYard-derived is written to the database.
- `Service.Open` at startup adopts the stored pairing (paired, no snapshot) before the loop starts.
- `Service.PullNow` reads the first 200 endpoints (`?limit=200`), then inventory and latest samples of each Docker endpoint in state `approved`, `active` or `offline`. A 404 on an endpoint's inventory or samples skips that endpoint; any other failure fails the pull. A successful pull replaces the snapshot atomically; a failed one keeps the snapshot and pairing and records `Reason`. An unreadable pairing logs `kyyard_pull_failed` with reason `unreadable`.
- `PullNow` single-flights on `pullMu`; request paths never call it inline, they `Kick` (buffered-1, non-blocking wake of `Run`).
- Stale: paired and no successful pull within `StaleAfter` (3 min), or last error `unauthorized`. A fact from an `offline` endpoint is `endpoint_offline` and stale. `Facts` is false for a link absent from the latest inventory.
- Memory and restart fields are meaningful only when `has_sample` (`sample_at` is the sample time; KyYard samples running containers only). Restart history is kept in memory for one hour; `restarts_last_hour` spans `history_minutes` (capped at 60).
- `Clear` (unpair) forgets everything and bumps a generation; a pull's commit, failure and unpaired-clear are discarded when the generation moved since the pull started.
- Re-pair order: `Pairing.Save`, then `Clear`, then `Adopt(cfg)`, then `Kick`. A failed save leaves the old pairing's in-memory state untouched.

## Verification
- `go test -race ./internal/kyyard/`

## Child DOX Index
None.
