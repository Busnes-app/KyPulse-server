# Egress

## Purpose
The one outbound HTTP client: health polls, KyYard and webhooks. Private and LAN destinations are allowed; loopback, link-local and the known cloud metadata addresses (`fd00:ec2::254`, `100.100.100.200`), unspecified, multicast, RFC 5737 documentation and reserved ranges are refused at dial time; redirects are refused; bodies are capped.

## Ownership
Owns `Client`, `ValidateURL`, `Cause` and the address policy for monitoring,
KyYard, webhook and sender network callers. OIDC/recovery retain their library
transport integration; local Docker reads use a separate Unix-socket client.

## Local Contracts
- `ValidateURL` is the save-time check; the dialer repeats the address check on every resolved IP, so a DNS rebind cannot pass.
- `AllowHTTP` is per client: on for health targets, off for webhooks unless `KYPULSE_ALERT_ALLOW_HTTP`, always off for KyYard.
- A body over `MaxBody` is `ErrBodyTooLarge`, never a truncated success.
- `Cause` is the fixed vocabulary the UI shows and tests pin.
- The Go process TLS profile preserves these address/body/redirect rules and
  rejects undersized verified-chain keys, including explicitly trusted roots.

## Verification
- `go test -race ./internal/egress/`
- Repeat with `GODEBUG=fips140=on`; a strong-leaf control and fixture-only
  DNS/dial seams prove the actual client's weak-root TLS refusal.

## Child DOX Index
None.
