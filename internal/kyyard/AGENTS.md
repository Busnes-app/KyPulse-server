# KyYard

## Purpose
Pairs kyPulse to one KyYard organization and reads what a `pulse_reader` may: endpoints, container inventory and resource samples.

## Ownership
`Pairing` (sealed URL, organization and token in `server_settings.kyyard_enc`), `Client` (bearer reads over the egress guard), `Claim` (pairing-code exchange), and the `Service` snapshot loop.

## Local Contracts
- The token and the pairing code never reach a log line, an audit row or an HTTP response; `Reason(err)` is the only text stored about a failure.
- `Claim` posts `{pairing_code, service_name:"kypulse"}`; 403 is `ErrPairingRefused`, 429 `ErrRateLimited`; a code that is not six digits is refused before any request.
- Reads carry `Authorization: Bearer`; 401 is `ErrUnauthorized` (reason `unauthorized`), other non-2xx `status_NNN`, transport failures the egress vocabulary.
- Nothing KyYard-derived is written to the database.
- `Service.PullNow` pulls endpoints (Docker runtime only), each endpoint's inventory and latest samples, and replaces the snapshot atomically; a failed pull keeps the last snapshot and records `Reason`. `PullNow` single-flights on its own `pullMu`, so the loop's own tick and any other caller never interleave -- callers on a request path must never call `PullNow` inline, since it can queue behind a slow loop pull (one HTTP round trip per endpoint) well past an HTTP write timeout; `Kick` is the non-blocking way to ask for one sooner. `StaleAfter` 3 min without a successful pull marks everything stale; `Facts` is false for a link absent from the latest inventory. Restart history per container is kept in memory for one hour for `restarts_last_hour`, merged into the same locked commit as the facts. `Clear` after unpair forgets everything and bumps a generation counter, so a pull already in flight cannot resurrect what it committed after; the generation is read under `s.mu` before the pairing is even loaded, not after, so an unpair landing during the load is caught too; nothing is written to the database. `PullNow`'s own "no pairing" branch (`Pairing.Load` found nothing) uses `clearIfCurrent(gen)`, not `Clear`, so a `Load` that raced a concurrent re-pair (whose `Clear`+`Adopt` already landed a newer generation) cannot un-adopt it -- clearing there is conditional on `gen` still being current, exactly like the commit path below it.
- `Kick` sends on a buffered-1 wake channel (`Run`'s `select` also holds this case) without blocking; a wake already pending collapses into one pull, since `PullNow` always reads the latest pairing regardless of how many `Kick`s asked for it. `Adopt(cfg)` marks a pairing paired in memory with no snapshot (`facts`/`fetchedAt` nil) -- `Status` then reads `paired:true, stale:true`, no `fetched_at` -- for a caller (the pair handler) that commits `cfg` without waiting for a pull. `Adopt` does not touch `gen`; callers must run it after the new `cfg` is durably saved to disk and after `Clear` (which invalidates any pull in flight under the previous pairing), in that order -- `Clear` must never run before the save that might still fail, or a failed re-pair would report the still-good old pairing as unpaired until the next pull.

## Verification
- `go test -race ./internal/kyyard/`

## Child DOX Index
None.
