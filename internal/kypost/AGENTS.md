# KyPost

## Purpose
KyPulse's client for alert mail through a member's KyPost mailbox: the signed
`kypulse.alert.send` request and the KyIdentity pairing behind it. Contract: `docs/KYPOST.md`
(byte-identical with KyPost `docs/KYPULSE.md`).

## Ownership
Owns `SendRequest`, `Link` (pairing state, `Run`, `Send`, `Status`), `Refused`, `ErrRetry`,
`ErrNotPaired` and `Reason`.

## Local Contracts
- Pairing is KyIdentity only (`suitedir.Peer(ctx, "kypost")` with KyPulse's KySignOn client
  credentials): at once, every 15 minutes, and at most once a minute after KyPost refused the
  signature; failures retry from 30 s doubling to 15 minutes. KyIdentity unreachable keeps a
  working pairing; `no_peer`, refused credentials or a peer URL that is not a bare https
  origin unpair at once. The secret is held in memory only.
- The body is exactly the five contract fields. 2xx is sent; 429, 5xx and network errors are
  `ErrRetry`; anything else is `Refused{Code}` keeping only a `[a-z0-9_]{1,64}` code from
  KyPost, else `status_<n>`. A refused signature asks for a fresh pairing and is retryable.
- Requests go through the caller's `egress` client (HTTPS only, no redirects, address guard).
- `Reason` is the only form of an error that is stored, shown or audited.

## Verification
- `go test -race ./internal/kypost/`

## Child DOX Index
None.
