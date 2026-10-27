# Auth

## Purpose
Manages user authentication, password policies, Multi-Factor Authentication (RFC 6238 TOTP), recovery codes, session token lifecycle, and anti-abuse CAPTCHA (Proof-of-Work default).

## Ownership
Owns session issuance and verification, TOTP token generation/validation, recovery code redemption, and client-side PoW verification.

## Local Contracts
- `RequestNetwork` exposes the same `ClientIP` result, actual peer trust and TLS/trusted nearest-proxy scheme for G11; untrusted forwarding headers cannot change them.
- Sessions for `must_change_password` users authenticate only GET `/api/auth/me` and POST `/api/auth/change-password` or `/api/auth/logout`; private settings and product actions fail closed. Session issuance is bound to the verified password hash and serialized with replacement.
- Passwords require at least 12 characters (`ValidatePassword`).
- Active sessions are stored with SHA-256 hashed tokens and verified against secure HttpOnly / SameSite cookies or Bearer headers.
- `IssueSSOSession` stores the provider's `sid` on the session so back-channel logout can end it, and fails with `store.ErrSSOSessionEnded` when that login was already ended.
- Session authentication rejects inactive accounts and deletes their presented session.
- PoW solutions must carry unexpired server-signed challenge metadata, and `PoWGuard` accepts each challenge salt once until it expires (bounded set; eviction only reopens a replay after a fresh solve).
- Password verification creates a five-minute opaque MFA transaction; MFA endpoints never accept a user ID and consume the transaction once.
- Single-use recovery codes are invalidated immediately upon redemption.
- `GenerateTOTPSecret`/`GenerateTOTPURL` serve enrolment (`internal/api/mfa_handlers.go`); every accepted `ValidateTOTP` counter, at sign-in, enrolment confirm or disable, is spent through `SpendTOTPCounter` before it is trusted.
- Shared TOTP authentication requires sixteen decoded key bytes; generation
  remains twenty random bytes. Preserve readable sealed legacy records and
  recovery methods while refusing short-key authentication.
- `ClientIP` is the one client-address helper: the rate limiter and session binding both use it. `X-Forwarded-For` is honoured only when the peer is in `KYPULSE_TRUSTED_PROXIES`, walking the chain from the right past trusted hops; `X-Real-IP` is ignored. Each entry may be a bare IP or `ip:port` (Azure App Service/Application Gateway append the port); anything else ends the walk and falls back to the peer.

## Verification
- `go test -v ./internal/auth/...`

## Child DOX Index
None.
