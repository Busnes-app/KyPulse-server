# SSO

## Purpose
Provides unified Single Sign-On federation for KySignOn, Generic OpenID Connect (Google, Microsoft Entra ID, Okta, Keycloak), and SAML 2.0 Service Provider (SP).

## Ownership
Owns the application adapters around OAuth/OIDC login, KySignOn HMAC-SHA256 signed directory sync webhooks, and SAML metadata publication.

## Local Contracts
- `KySignOnClient.HandleSyncWebhook` refuses signing keys below
  `crypto.MinHMACKeyBytes` and verifies signatures before modifying local user state.
- A `v1=` signature on `/api/sso/kysignon/sync` is a KyIdentity syncauth delivery and is handled by the API's installer directory route, not this client.
- PKCE with `S256` is enforced on all OAuth/OIDC authorization requests.
- ID tokens require provider signature, issuer, audience, expiry, and one-time nonce verification before claims are trusted.
- The private JWKS transport bounds input, refuses redirects and checks every
  RSA modulus at 2048 bits with verifier-matched envelope/case-sensitive JWK
  parsing. Initialize the provider/verifier under the discovery mutex and keep
  upstream cache/rotation and claim verification.
- `GODEBUG=fips140=on` requires direct HTTPS OIDC issuer/endpoints without URL
  credentials and refuses discovery/token redirects. Preserve caller trust
  roots; `docs/STRONG-CRYPTO.md` owns process/edge configuration and migration.
- OAuth discovery, authorization URLs, PKCE parameters, code exchange, and token verification are delegated to `golang.org/x/oauth2` and `coreos/go-oidc`; application code only maps verified claims.
- SAML assertion parsing is not implemented locally; metadata XML uses `encoding/xml` and no ACS route is exposed until a maintained SAML service-provider library is configured.
- Directory webhook timestamps are accepted only within five minutes; status or role changes revoke the user's sessions. The payload has no event ID, so a SHA-256 of the signed body is remembered until its timestamp window ends and a repeat is refused; a delivery that fails to apply is forgotten so the provider can retry. The set is in-memory and bounded; when full of live entries it refuses rather than forgets.
- `VerifyLogoutToken` (`backchannel.go`) reuses the ID-token verifier, then requires
  `iat`, `sub` or `sid`, the back-channel logout event as a JSON object and no `nonce`;
  a present `typ` must be `logout+jwt`. Every failure is `ErrInvalidLogoutToken`.
  `IdentityClaims.SessionID` and `IssuedAt` carry the ID token `sid` and `iat` so sessions can be ended by them.
  Only KySignOn has a login route, so `sid` values come from one issuer.
- Provider-asserted roles pass through `RoleFor`: exactly `admin` grants admin, everything else is a viewer.

## Verification
- `go test -v ./internal/sso/...` (`TestVerifyLogoutToken` holds one rejection case per logout-token rule)
- Repeat in configured mode: real key-size/casing regressions exercise both
  adapters with strong-key, cache and wrong-nonce controls.

## Child DOX Index
None.
