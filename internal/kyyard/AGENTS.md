# KyYard

## Purpose
Pairs kyPulse to one KyYard organization and reads what a `pulse_reader` may: endpoints, container inventory, resource samples, linked-container logs and organization audit.

## Ownership
`Pairing` (sealed URL, organization and token in `server_settings.kyyard_enc`), `Client` (bearer reads over the egress guard), `Claim` (pairing-code exchange), and the independent `Service` inventory and collection loops.

## Local Contracts
- The token and the pairing code never reach a log line, an audit row or an HTTP response; Only fixed reason codes are stored for failures; remote failures use `Reason(err)`, local collection failures use `unreadable`, `storage`, `cursor_invalid` or `links_unresolved`.
- `Claim` posts `{pairing_code, service_name:"kypulse"}`; 403 is `ErrPairingRefused` (reason `pairing_refused`), 429 `ErrRateLimited` (`rate_limited`); a code that is not six digits is refused before any request.
- Reads carry `Authorization: Bearer`; 401 is `ErrUnauthorized` (`unauthorized`), other non-2xx `status_NNN`, transport failures the egress vocabulary. `cmd/server` keeps inventory/audit at 2 MiB and 5 s; log pulls use a separate guarded client capped at 32 MiB and 30 s. A body over either cap fails without advancing a cursor.
- Inventory, samples and restart history stay in memory. Logs, imported activity and cursors are persisted through `LogStore.AppendImported`; the shared log budget counts every attributed copy.
- `Service.Open` at startup adopts the stored pairing (paired, no snapshot) before the loop starts.
- `Service.PullNow` reads the first 200 endpoints (`?limit=200`), then inventory and latest samples of each Docker endpoint in state `approved`, `active` or `offline`. A 404 on an endpoint's inventory or samples skips that endpoint; any other failure fails the pull. A successful pull replaces the snapshot atomically; a failed one keeps the snapshot and pairing and records `Reason`. An unreadable pairing logs `kyyard_pull_failed` with reason `unreadable`.
- `PullNow` single-flights on `pullMu`; request paths never call it inline, they `Kick` (buffered-1, non-blocking wake of `Run`).
- Stale: paired and no successful pull within `StaleAfter` (3 min), or last error `unauthorized`. A fact from an `offline` endpoint is `endpoint_offline` and stale. `Facts` is false for a link absent from the latest inventory.
- Memory and restart fields are meaningful only when `has_sample` (`sample_at` is the sample time; KyYard samples running containers only). Restart history is kept in memory for one hour; `restarts_last_hour` spans `history_minutes` (capped at 60).
- Lifecycle reset forgets the in-memory snapshot/status and bumps a generation; a pull's commit, failure and unpaired-clear are discarded when the generation moved since the pull started.
- Production pairing changes use `Service.Replace`/`Unpair` under the service lifecycle lock, then `Kick` inventory after replacement. Lock order: service, pairing, database. Import holds these locks through persisted-generation validation and the row/cursor transaction; network calls hold neither lock. `Pairing.Save` also serializes against import and always generates a random generation; legacy pairings acquire a stable generation once on load. Failed replacement leaves the old pairing intact.
- `CollectNow` single-flights independently of inventory. Its owned 60 s worker is canceled and joined before store close. It groups resolved links by endpoint/container, fetches each once and attributes copies to every linked target ID, including health-disabled targets. Missing links set `links_unresolved`; one failed container does not prevent other containers or audit collection.
- Logs request `tail=1000&timestamps=1&follow=0` and the saved inclusive `since` timestamp. `parseLogs` walks bounded text/plain bytes, stores at most 1,000 application lines capped at 16 KiB, recognizes only notices bearing `X-KyYard-Notice-Token`, and retains discontinuity notices without interpreting them as audit. A full tail records “history may be incomplete: pull reached 1000 lines”. Equal-timestamp replay duplicates are intentional; tail polling cannot guarantee complete high-volume history.
- Audit requests `after_id` and `limit=200`, at most five pages per tick. IDs must strictly increase and `next_after_id` must equal the last row (or the request on an empty page). Each page commits before the next request. Old array responses return `audit_cursor_unsupported` and preserve the cursor. Activity maps actor/action/target/outcome/IP explicitly, uses app `kyyard` and external key `generation:organization:remote-ID`; no remote chain verification.
- Status retains independent inventory/log/audit `last_success`, fixed `error` and `stale` fields. Inventory success never clears collection errors; last-success ages reset on restart/pairing and become stale after three minutes or immediately on unauthorized. Audit last-success records committed partial progress even if the next page fails.

## Verification
- `go test -race ./internal/kyyard/`

## Child DOX Index
None.
