# Egress

## Purpose
The one outbound HTTP client: health polls, KyYard and webhooks. Private and LAN destinations are allowed; loopback, link-local (cloud metadata), unspecified, multicast and reserved ranges are refused at dial time; redirects are refused; bodies are capped.

## Ownership
Owns `Client`, `ValidateURL`, `Cause` and the address policy. No caller builds its own `http.Client`.

## Local Contracts
- `ValidateURL` is the save-time check; the dialer repeats the address check on every resolved IP, so a DNS rebind cannot pass.
- `AllowHTTP` is per client: on for health targets, off for webhooks unless `KYPULSE_ALERT_ALLOW_HTTP`, always off for KyYard.
- A body over `MaxBody` is `ErrBodyTooLarge`, never a truncated success.
- `Cause` is the fixed vocabulary the UI shows and tests pin.

## Verification
- `go test -race ./internal/egress/`

## Child DOX Index
None.
