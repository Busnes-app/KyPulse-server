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

## Verification
- `go test -race ./internal/kyyard/`

## Child DOX Index
None.
