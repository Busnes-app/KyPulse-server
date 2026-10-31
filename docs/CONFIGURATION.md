# Configuration

[Product overview](../README.md) · [Standalone installation](../INSTALL.md)

Commands and unlinked file paths below are relative to the repository root.

## Configuration

Every setting is an environment variable. `KY_LOG_LEVEL` (`debug`, `info`, `warn`, `error`)
is the one suite-wide variable; the rest are `KYPULSE_*`. Every log line is JSON on stderr;
there is no log file.

| Variable | Default | Purpose |
|---|---|---|
| `KYPULSE_PORT`, `KYPULSE_HOST` | `8080`, `127.0.0.1` (`0.0.0.0` in the image) | listen address |
| `KYPULSE_APP_URL` | `http://localhost:<port>` | public URL; CORS origin and the link in webhook messages |
| `KYPULSE_APP_NAME` | `kyPulse` | service name: recovery capsules are sealed under it, so keep it across restores. The UI shows it, with the default spelled KyPulse |
| `KYPULSE_ENV` | `development` (`production` in the image and Compose) | `development` or `production`. Development listens on loopback only, mints a session secret per start and sends cookies without `Secure` |
| `KYPULSE_ALLOW_INSECURE_DEV` | `false` | let development listen on a non-loopback `KYPULSE_HOST` |
| `KYPULSE_SESSION_SECRET` | `<data dir>/session.key` in production, random per start in development | signs the proof-of-work login challenge |
| `KYPULSE_ENCRYPTION_KEY` | `<data dir>/encryption.key` | 32-byte key sealing the webhook, KyYard token, TOTP secrets and KyRecovery token |
| `KYPULSE_AUDIT_KEY` | read from `KYPULSE_AUDIT_KEY_FILE` | 32-byte key of the audit hash chain; must differ from the encryption key |
| `KYPULSE_AUDIT_KEY_FILE` | `<data dir>/audit.key` (`/app/secrets/audit.key` in Compose) | audit keyfile, minted on first start; an existing `<data dir>/audit.key` wins while this file does not exist |
| `KYPULSE_ADMIN_PASSWORD` | generated into `<data dir>/initial-admin-password` (0600; never logged) | first admin's password on an empty database |
| `KYPULSE_BIND` | `127.0.0.1` | Compose only: host address the `8080` port is published on |
| `KYPULSE_DATA_DIR` | `./data` (`/app/data` in Compose) | database, keyfiles |
| `KYPULSE_DB_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `KYPULSE_DB_DSN` | SQLite file in the data dir; required for `postgres` | connection string |
| `KYPULSE_DB_MAX_OPEN_CONNS`, `KYPULSE_DB_MAX_IDLE_CONNS` | `25`, `5` | PostgreSQL pool |
| `KYPULSE_TRUSTED_PROXIES` | none | proxy IPs or CIDRs allowed to set `X-Forwarded-For`; `0.0.0.0/0` is refused |
| `KYPULSE_COOKIE_SECURE`, `KYPULSE_COOKIE_DOMAIN` | on in production or with an `https` app URL, empty | session cookie `Secure` flag (also gates HSTS) |
| `KYPULSE_CAPTCHA_PROVIDER` | `pow` | `pow`, `turnstile`, `friendly` or `none` |
| `KYPULSE_CAPTCHA_SITE_KEY`, `KYPULSE_CAPTCHA_SECRET_KEY` | empty | for turnstile or friendly |
| `KYPULSE_CAPTCHA_POW_DIFFICULTY` | `50000` | proof-of-work search space (hashes tried at most) |
| `KYPULSE_SSO_ENABLED`, `KYPULSE_SSO_AUTO_PROVISION` | `true`, `true` | single sign-on |
| `KYPULSE_KYSIGNON_ISSUER`, `_CLIENT_ID`, `_SECRET`, `_HMAC_SECRET` | empty | KySignOn; register `<app URL>/api/sso/kysignon/backchannel-logout` as the client's back-channel logout URI so signing out there ends the session here |
| `KYPULSE_OIDC_ISSUER`, `_CLIENT_ID`, `_SECRET` | empty | generic OIDC |
| `KYPULSE_SAML_ENTITY_ID`, `KYPULSE_SAML_METADATA_URL` | empty | SAML service provider |
| `KYPULSE_POLL_WORKERS` | `4` | concurrent health polls, 1 to 32 |
| `KYPULSE_ALERT_ALLOW_HTTP` | `false` | admit a plain-http webhook URL |
| `KYPULSE_KYYARD_ALLOW_HTTP` | `false` | admit a plain-http KyYard URL |
| `KYPULSE_LOG_MAX_BYTES` | `1073741824` (1 GiB) | maximum retained log and activity payload bytes; minimum 131072 |
| `KYPULSE_BACKUP_DIR` | empty (`/app/backups` in Compose) | sealed local backup copies; empty keeps none |
| `KYPULSE_BACKUP_KEEP` | `7` | local copies kept |
| `KYPULSE_BACKUP_DEPOSIT_INTERVAL` | `24h` | default backup schedule; `0` off, at least `15m` |
| `KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY` | `false` | admit a KyRecovery on a private address (HTTPS still required) |

Integer and boolean variables must parse (`true`/`false`, `1`/`0`, `yes`/`no`, `on`/`off`);
a typo or an empty value refuses to start rather than reading as the default.

Back up `encryption.key` and `audit.key` with the database: without them the sealed settings
cannot be opened and the audit trail cannot be verified. Backups carry both.

Logs and activity share a seven-day retention period and the byte limit above. Age uses
receive time, so a sender's future timestamp cannot extend retention. The limit is
enforced during each append and checked again at startup and hourly; oldest received rows
go first. It measures stored UTF-8 field bytes plus a fixed row allowance, including
derived activity separately, rather than database, index or WAL file size. A committed
batch may be evicted by retention. SQLite may reuse freed pages after deletion without
immediately shrinking its file. SQLite recovery capsules exclude collected log lines,
imported activity, collection cursors and pending pairing codes, including residual text in
free pages; they keep source identities and token hashes. PostgreSQL capsules are unsupported.

## Compose overlays

Append an overlay's filename to `COMPOSE_FILE` in `.env` (never `-f`, which replaces the list
outright); each file's header comment has a copy-pasteable one-liner that does this for you.

**Source build** (`docker-compose.build.yml`). Builds and runs the local image
(`kypulse:local`) instead of the published one, for every command including restore. Revert
by removing it from `COMPOSE_FILE`.

**LAN DNS and private KyRecovery** (`docker-compose.lan-dns.yml`). Points the container's DNS
at `KYPULSE_DNS`, a LAN resolver, so a KyRecovery reachable only there resolves; it also sets
`KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY=true`, admitting RFC1918/CGNAT KyRecovery destinations
(loopback and other reserved ranges stay refused; HTTPS is still required). Revert by
removing the overlay from `COMPOSE_FILE` and unsetting `KYPULSE_DNS` and
`KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY` in `.env`.

**Static IP** (`docker-compose.static-ip.yml`). Pins the container to `KYPULSE_CONTAINER_IP`
on a network with subnet `KYPULSE_NETWORK_SUBNET`, both required once the overlay is in the
chain.

**PostgreSQL**. Set `POSTGRES_PASSWORD` in `.env` (`openssl rand -hex 32`), then
`docker compose --profile postgres up -d` starts the bundled database, reachable only on the
compose network. Point KyPulse at it with `KYPULSE_DB_DRIVER=postgres` and
`KYPULSE_DB_DSN=postgres://postgres:<password>@postgres:5432/kypulse?sslmode=disable`. Backups
snapshot SQLite only, so a PostgreSQL deployment backs up its database separately.

The published image is `ghcr.io/busnes-app/kypulse-server`. `docker-compose.yml` explains how
to pin a commit by digest (`KYPULSE_IMAGE`) and verify its build attestation first.
