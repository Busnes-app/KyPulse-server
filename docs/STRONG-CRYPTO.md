# Supported key-strength and TLS configuration

[Running](RUNNING.md) · [Restore](RESTORE.md) · [Assurance case](ASSURANCE-CASE.md)

Use this configuration with the matching source/image revision. It covers the
server and `kypulse-send`; check the actual endpoints and independent HTTPS edge
before rollout. It is not a FIPS certification or a claim about every deployment.

## Preserve existing data, identities and custody

Keep the database, original `session.key`, `encryption.key`, audit key and its
configured path, environment, protected installer credentials, paired-source
identities/tokens, KyYard binding, recovery pin/token, capsules, receipts and
custodian cards. Keep each sender's original 0700 state directory, 0600 token and
checkpoint files. Enabling TLS policy does not re-pair a sender or reset a cursor.

The audit key is independent of the encryption key. Retain its actual file:
`KYPULSE_AUDIT_KEY_FILE` may name a separate secrets volume, while an existing
legacy `data/audit.key` remains the fallback until deliberately migrated. Never
mint a replacement to make audit verification pass. Preserve every data, backup
and secrets mount plus the existing image/build, DNS and network overlays.

Primitives source `bae5c14dba4c` (`v0.10.2-0.20261026050236-bae5c14dba4c`)
adds the shared TOTP key floor and carries merged capsule tar-budget, Shamir
sampling and setup-refusal repairs. The key/share/container formats are unchanged.
Keep historical capsules and their original custody; test a known restore before
upgrading an installation.

## Key rules in every mode

| Boundary | Rule |
| --- | --- |
| Session and PoW signing | A supplied `KYPULSE_SESSION_SECRET` needs at least 16 literal bytes. Production otherwise loads/creates the original 32-byte `session.key` and hex-encodes it; development generates a throwaway strong value. Prefer independent random material, such as `openssl rand -hex 32`, when explicitly configuring a secret. Length does not establish entropy. |
| Directory webhook | Nonempty `KYPULSE_KYSIGNON_HMAC_SECRET` requires at least 16 literal bytes at startup and before legacy webhook use. Installer directory credentials meet the same floor before active configuration changes. Syncauth delivery retains its shared signature/replay rules and account binding. Coordinate a replacement with the sender; empty leaves legacy webhooks unconfigured. |
| Data and audit keys | Encryption and independent audit keys each require exactly 32 decoded bytes, whether supplied or retained in keyfiles. AES-256-GCM records and the HMAC-SHA256 audit chain retain their original keys. |
| OIDC | Every fetched RSA JWKS member needs at least 2048 actual modulus bits. Envelope/member parsing matches go-oidc/go-jose, including member casing; input is bounded and redirects refused before caching. Maintained libraries retain algorithm, signature, issuer/audience/expiry and nonce checks, with supported strong EC/Ed25519 keys. `RoleFor` retains admin/viewer mapping. |
| TOTP | Generate twenty random decoded bytes. Authentication requires at least sixteen through the shared RFC 4226/6238 implementation. Existing sealed records remain readable but a short key cannot authenticate. Use an enrolled recovery method or deliberate administrator reset to re-enroll; preserve unrelated credentials. |
| Source/sender credentials | Source tokens are 32 random bytes with existing format/hash, privilege and target/source binding. Sender state/token/checkpoints remain separate private files. The Docker reader uses an absolute local Unix socket, not a remote TLS key. |
| Recovery | Fixed-suite `kycap/3` containers seal to the pinned suite public key. The product server never receives custodial shares; offline restore retains the recorded digest and original custody procedure. Collected log/activity payloads remain excluded under the existing retention/backup contract; this does not create an independent log archive. |

Short signing inputs fail without rewriting existing keyfiles or identity state.
Changing a session signing secret invalidates existing PoW challenges. A weak
IdP signing key requires provider rotation, not a verification bypass. Restart
after upgrade to replace the in-memory verifier/cache with the repaired policy.

## Enable the Go TLS profile

Use the toolchain in `go.mod` and the pinned Docker builder. Set
`GODEBUG=fips140=on` before every server, sender or remote Go CLI process starts.
Preserve other needed GODEBUG entries with `fips140` set to `on`, then restart.
Use `on`, not the diagnostic `only` mode: Argon2id, HMAC-SHA1 TOTP and fixed-suite
recovery remain available.

The profile requires direct HTTPS OIDC issuer/authorization/token/JWKS URLs
without URL credentials and refuses discovery/token redirects. Normal TLS chain
and hostname checks, PKCE, signatures, claims, nonce and key rotation remain.
Go filters the complete verified certificate chain, including explicitly trusted
roots, to RSA-2048+ and supported strong EC/Ed25519 keys. A private CA does not
waive the floor.

This applies to Go HTTPS for health polling, alert webhooks, KyYard, sender
pairing/delivery, OIDC, KyRecovery and verified PostgreSQL TLS. Monitoring,
KyYard, webhook and sender network requests keep the existing `egress` address,
redirect and body limits. OIDC and recovery retain their maintained libraries'
transport integration and additional policy. Local Docker Unix-socket reads do
not cross a TLS boundary.

Review every stored destination, not just environment defaults. Use HTTPS for
health targets, alert webhooks, KyYard and sender destinations; retain
`KYPULSE_ALERT_ALLOW_HTTP=false` and `KYPULSE_KYYARD_ALLOW_HTTP=false`. Legacy
HTTP health URLs remain supported elsewhere but are outside this HTTPS profile.
TLS settings do not upgrade a saved HTTP URL or authorize verification bypasses.

For Compose, append `docker-compose.strong-crypto.yml` to the existing protected
`.env` `COMPOSE_FILE` chain, preserving base/build/LAN-DNS/static-IP files. The
overlay adds only GODEBUG and does not forward unrelated `.env` variables. If
another overlay sets GODEBUG, retain its needed entries with `fips140=on` in the
final resolved value. Inspect resolved configuration privately: it has secrets.
`python3 docs/check-strong-crypto-compose.py` compares four complete configurations
with synthetic inputs and starts no containers.

Use production mode, an HTTPS `KYPULSE_APP_URL`, secure cookies and the proxy's
exact trusted address. The local-data profile uses SQLite and original data paths.
For native processes, explicitly export the existing `KYPULSE_PORT` and
`KYPULSE_DB_DRIVER=sqlite`; the existing restore drill requires both. Compose
supplies them. Remote PostgreSQL uses `sslmode=verify-full` and the correct
CA/hostname; its separate operator backup procedure remains unchanged.

## Independent HTTPS edge and rollout

The service serves private backend HTTP. Limit it to the intended TLS proxy and
keep `KYPULSE_TRUSTED_PROXIES` narrow. Use TLS 1.2/1.3 with a verified RSA-2048+
or P-256+ certificate chain. For a host Nginx/OpenSSL proxy, relevant policy is:

```nginx
ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers 'DEFAULT@SECLEVEL=2';
```

Set actual certificate/key paths and routes, then verify the deployed edge.
Go's setting does not configure Nginx, browsers or other products. SAML metadata
publication is not an inbound assertion verifier.

Run complete ordinary/configured race suites on SQLite/PostgreSQL, frontend
build/tests and browser regressions, module/vulnerability checks, Compose
preservation, repeatable binaries, static sender build and Docker auth/health,
key/audit/source continuity and recovery-drill checks. Then verify real sign-in,
TOTP/recovery access, directory revocation, polling, alert delivery, KyYard and
sender acknowledgments, audit verification, backup receipts and a custody-correct
offline restore. Local checks do not establish live operational readiness.

References: [RFC 4226](https://www.rfc-editor.org/rfc/rfc4226#section-4),
[RFC 6238](https://www.rfc-editor.org/rfc/rfc6238),
[Go FIPS runtime mode and limits](https://go.dev/doc/security/fips140).
