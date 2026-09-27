# kyPulse

Unified health and logging for the Busnes.app Ky suite. kyPulse polls every Ky app's
`/healthz`, reads container state from KyYard, shows what is broken first, and sends an
outbound webhook when an app goes down or degrades. It is read-only towards the apps it
watches: it observes and reports, and never changes them.

Design: [`docs/superpowers/specs/2026-09-26-kypulse-design.md`](docs/superpowers/specs/2026-09-26-kypulse-design.md).

## What works today

| Area | State |
|---|---|
| Health polling, alert state machine, webhook alerts | done |
| Status, Alerts, app detail, alert bar, webhook form | done |
| KyYard pairing, container facts, stale marking | done |
| kyPulse's own tamper-evident audit trail | done |
| Source pairing, log ingest, admin log and activity APIs, retention | done |
| `kypulse-send` Linux sender | done |
| Logs and Activity tabs, KyYard log and audit-feed pulls | planned (client build step) |

## Quick start

```sh
docker compose up -d
docker compose logs app | grep 'Initial bootstrap'   # the generated admin password
```

Open <http://localhost:8080>, sign in as `admin` with that password, and replace it when
asked. Set `KYPULSE_ADMIN_PASSWORD` before the first start to choose it instead. For anything
reachable beyond your own machine, set `KYPULSE_ENV=production`, a durable
`KYPULSE_SESSION_SECRET` and a matching `KYPULSE_APP_URL`, and put kyPulse behind a TLS
proxy (see `KYPULSE_TRUSTED_PROXIES`).

Watch kyPulse's own `GET /healthz` from outside. kyPulse does not monitor itself.

## Screens

- **Alert bar**, on every page. Red with one line per down or degraded app; green with the
  healthy count and the last check; a separate line when webhook delivery keeps failing or
  KyYard data is stale.
- **Status**, the home tab. Every watched app as a tile, broken first. Admins add apps here.
- **Alerts**. State changes and hourly reminders with their delivery result, and current
  silences.
- **App detail** (`#/apps/<id>`, the link webhook messages carry). Current state with the
  exact failing request, the checks from the last response, alert history, KyYard container
  facts, and, for admins, silence (1 h, 8 h, until fixed), edit and delete.
- **Settings & DB**. The alert webhook and KyYard pairing (admins), theme, database driver.
- **Backup** (admins). KyRecovery pairing, schedule, local copies, restore drill.

Roles are `admin` and `viewer`. A viewer sees Status, Alerts, app detail and a read-only
Settings & DB; every write and Backup are admin-only, and the server enforces it.
Collected Logs and imported Activity, including app-detail log queries, are admin-only.

## Sending logs

An admin creates a six-digit code with `POST /api/log-sources/pairing` and JSON
`{"target_id":"<watched target ID>"}`. Omit `target_id` for a source that appears only in
global log queries. The code expires after 15 minutes and can be claimed once. A sender
claims it without a user session using `POST /api/log-sources/claim` and
`{"pairing_code":"123456","name":"host-app"}`. Keep the returned `token` secret: it is
shown only on claim, and the database stores its hash. An admin can list source IDs with
`GET /api/log-sources` and revoke one with `DELETE /api/log-sources/<id>`; revocation stops
future ingestion and keeps historical rows. Browser session writes require CSRF; a claim
without a session does not.

Send UTF-8 NDJSON to `POST /api/ingest/logs` with
`Authorization: Bearer <source token>` and `Content-Type: application/x-ndjson`:

```json
{"line":"{\"timestamp\":\"2026-09-27T12:00:00Z\",\"app\":\"kyvault\",\"level\":\"INFO\",\"event\":\"started\",\"message\":\"service started\"}"}
```

Each record needs a string `line`, which may contain an original JSON line or plain text;
optional `time` is an RFC3339Nano transport timestamp and `truncated` is a boolean.
Unknown fields are accepted. Source identity and watched-target binding come from the
token, never the body; an application `app` field is an untrusted label. A bound source
appears in that target's detail queries even if its `app` label differs. An unbound source
appears in global queries. Logs shaped like audit events also create imported Activity
rows; they are indexed observations, not verified entries in kyPulse's own audit chain.

The complete request is limited to 1 MiB and 1,000 records. Decoded lines beyond 16 KiB
are cut at a UTF-8 boundary and marked truncated. Empty batches, blank records, malformed
NDJSON or times fail the whole batch with 400; oversized requests return 413 and content
types or encodings other than NDJSON and identity return 415. Invalid or revoked tokens
return 401. Each source may send 60 requests per minute; 429 includes `Retry-After`.
A 204 means the full batch committed, subject to normal retention. Pairing claims are
limited to five attempts per minute per client IP and 30 globally; 429 also includes
`Retry-After`. Admins can read `GET /api/logs` and `GET /api/activity` with descending ID
pages (`limit` defaults to 100, maximum 200; `before_id` is exclusive). Both return
`{"items":[...],"next_before_id":0}` when no next page exists; viewers get 403.

### Linux sender

`make build` produces `kypulse` and `kypulse-send`. Pair a source using the admin's
six-digit code, then run one input mode with the same owner-only state directory:

```sh
kypulse-send pair --url https://pulse.example.com --code 123456 --name host-app
kypulse-send file --path /var/log/app.log
app 2>&1 | kypulse-send stdin
kypulse-send docker --container app,worker --socket /var/run/docker.sock
```

The sender requires Linux. Its default state is `$XDG_STATE_HOME/kypulse-send` or
`~/.local/state/kypulse-send`; the directory is mode 0700, the token and checkpoint are
mode 0600, and one process holds its lock. Use `--state-dir` on each command for another
source. kyPulse can revoke a source by deleting it from the admin source list, after
which delivery gets 401. TLS uses the host's normal system root certificates.

One bounded queue combines all configured inputs. It sends every 2 s or at 500 lines,
splitting earlier at the encoded 1 MiB request limit. Up to 16 MiB is held in memory;
overflow discards oldest lines and sends a `dropped N lines` marker. Checkpoints advance
only after a successful request. A crash can replay acknowledged lines when the checkpoint
was not saved; stdin and the queue are lost on restart. File input follows rename rotation
and truncation, but deleted history and an indistinguishable inode reuse can leave gaps or
duplicates. Docker resolves names to immutable IDs and follows stdout and stderr separately,
up to 32 distinct containers. It requests history inclusively from the earliest saved
Unix-second timestamp. It replays every line at the checkpoint timestamp, including
acknowledged lines, because rotation can remove an earlier line and renumber equal-time
occurrences. This can produce duplicates after sender restart. If newer lines arrive
without the saved timestamp, the sender emits a gap marker. Docker cannot prove every
history loss; malformed or oversized frames produce errors or gap markers. A TTY container
has one merged raw stdout stream. A completed Docker follow response ends that reader;
restart the sender to watch a later restart of that container. Other configured readers
continue until their own streams complete.

Docker socket access is root-equivalent. Prefer a read-only socket proxy exposing only
`/version`, versioned container inspect and logs routes. Mounting a Docker socket with `:ro`
does not make its API read-only. The sender accepts only a local Unix socket, never a remote
Docker URL.

## Watching apps

An admin adds an app with a name, its health URL and an interval (default 30 s, 10 s to 1 h).
kyPulse reads any health endpoint, in this order:

1. A 2xx with `"schema": "ky.health/1"` is used as-is, checks included
   ([`ky-primitives/health`](https://github.com/Busnes-app/ky-primitives) serves it).
2. A 2xx JSON body with `status` of `ok`, `alive`, `ready` or `healthy` is ok; `degraded`
   is degraded; a boolean `healthy` is ok or degraded.
3. Any other 2xx is ok, shown as **basic** (no checks).
4. A non-2xx answer, timeout, TLS or DNS failure, or refused connection is down, with the
   cause recorded.

A request times out after 5 s, reads at most 64 KiB and never follows redirects. Private and
LAN addresses are allowed; loopback, link-local and cloud metadata addresses are refused,
also when a name resolves to one at connect time.

An app turns **down** after 3 down polls in a row, **degraded** after 2, and **ok** after 2.
Each change sends one webhook message and is recorded under Alerts. While an app stays down
or degraded, a reminder goes out every hour. Silencing (1 h, 8 h or until fixed) stops
webhook messages only; the alert bar and Alerts keep showing the problem, and "until fixed"
ends at the next recovery.

## Alert webhook

Set in Settings & DB by an admin, sealed at rest with the deployment key. Presets:

| Preset | Sends |
|---|---|
| ntfy | text body with title and priority headers; optional access token |
| Gotify | token in `X-Gotify-Key`, never in the URL |
| Discord | JSON `content` |
| Generic | `{"app","state","previous","reason","time","url"}`; optional bearer token |

The token is write-only: it is never shown again, and leaving the field empty keeps it.
**Send test** makes one attempt and shows the answer. Real alerts retry 3 times with backoff;
continued failure raises "Alerts not being delivered" in the alert bar. Messages carry the
app name, the transition, a reason code, the time and a link, never log lines, user names or
IPs. Webhook URLs must be https unless `KYPULSE_ALERT_ALLOW_HTTP=true`.

## KyYard

kyPulse can read one KyYard organization with a read-only service token.

1. A KyYard organization administrator opens **Members → Service tokens → Pair kyPulse** and
   gets a six-digit code, valid 15 minutes.
2. A kyPulse admin enters the KyYard URL and that code on the KyYard card in Settings & DB.

The token is sealed at rest, never logged and never shown. kyPulse then pulls every 60 s:
Docker endpoints that are approved, active or offline (the first 200), their container
inventory and latest resource samples. Nothing KyYard sends is stored; it lives in memory
and a restart re-pulls it.

Link a watched app to a container (`endpoint/name`; admins get suggestions in the add and
edit form) and its detail page shows state and exit code, Docker health, image, memory
against its limit and restarts with the sample's age, restarts in the last hour, and when it
was observed. Memory and restarts appear only once KyYard has a sample (it samples running
containers only); a container on an offline endpoint is marked and shown stale.

Right after pairing or a restart the card and the bar say **first pull pending**. KyYard data
is shown **stale** when no pull has succeeded for 3 minutes, and at once when KyYard refuses
the token (it was revoked: unpair and pair again).

Unpairing takes two steps, one on each side: **Unpair** here deletes the URL and token in
kyPulse only; a KyYard administrator must also revoke the token on the Members page. The
same applies when you pair again: the previous token stays valid in KyYard until revoked
there. KyYard URLs must be https unless `KYPULSE_KYYARD_ALLOW_HTTP=true`.

## Configuration

Every setting is an environment variable. `KY_LOG_LEVEL` (`debug`, `info`, `warn`, `error`)
is the one suite-wide variable; the rest are `KYPULSE_*`. Every log line is JSON on stderr;
there is no log file.

| Variable | Default | Purpose |
|---|---|---|
| `KYPULSE_PORT`, `KYPULSE_HOST` | `8080`, `0.0.0.0` | listen address |
| `KYPULSE_APP_URL` | `http://localhost:<port>` | public URL; CORS origin and the link in webhook messages |
| `KYPULSE_APP_NAME` | `kyPulse` | name shown in the UI |
| `KYPULSE_ENV` | `development` | `production` requires `KYPULSE_SESSION_SECRET` and turns on secure cookies |
| `KYPULSE_SESSION_SECRET` | random per start | session signing key; set a durable one in production |
| `KYPULSE_ENCRYPTION_KEY` | `<data dir>/encryption.key` | 32-byte key sealing the webhook, KyYard token, TOTP secrets and KyRecovery token |
| `KYPULSE_AUDIT_KEY` | `<data dir>/audit.key` | 32-byte key of the audit hash chain; must differ from the encryption key |
| `KYPULSE_ADMIN_PASSWORD` | generated and printed | first admin's password on an empty database |
| `KYPULSE_DATA_DIR` | `./data` (`/app/data` in Compose) | database, keyfiles |
| `KYPULSE_DB_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `KYPULSE_DB_DSN` | SQLite file in the data dir | connection string |
| `KYPULSE_DB_MAX_OPEN_CONNS`, `KYPULSE_DB_MAX_IDLE_CONNS` | `25`, `5` | PostgreSQL pool |
| `KYPULSE_TRUSTED_PROXIES` | none | proxy IPs or CIDRs allowed to set `X-Forwarded-For`; `0.0.0.0/0` is refused |
| `KYPULSE_COOKIE_SECURE`, `KYPULSE_COOKIE_DOMAIN` | on in production, empty | session cookie |
| `KYPULSE_CAPTCHA_PROVIDER` | `pow` | `pow`, `turnstile`, `friendly` or `none` |
| `KYPULSE_CAPTCHA_SITE_KEY`, `KYPULSE_CAPTCHA_SECRET_KEY` | empty | for turnstile or friendly |
| `KYPULSE_CAPTCHA_POW_DIFFICULTY` | `4` | proof-of-work difficulty |
| `KYPULSE_SSO_ENABLED`, `KYPULSE_SSO_AUTO_PROVISION` | `true`, `true` | single sign-on |
| `KYPULSE_KYSIGNON_ISSUER`, `_CLIENT_ID`, `_SECRET`, `_HMAC_SECRET` | empty | KySignOn |
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

Keep `encryption.key` and `audit.key` with the database: without them the sealed settings
cannot be opened and the audit trail cannot be verified. Backups carry both.

Logs and activity share a seven-day retention period and the byte limit above. Age uses
receive time, so a sender's future timestamp cannot extend retention. The limit is
enforced during each append and checked again at startup and hourly; oldest received rows
go first. It measures stored UTF-8 field bytes plus a fixed row allowance, including
derived activity separately, rather than database, index or WAL file size. A committed
batch may be evicted by retention. SQLite may reuse freed pages after deletion without
immediately shrinking its file. SQLite recovery capsules exclude collected log lines,
imported activity and pending pairing codes, including residual text in free pages; they
keep source identities and token hashes. PostgreSQL capsules are unsupported.

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

**PostgreSQL**. `docker compose --profile postgres up -d` starts the bundled database; point
kyPulse at it with `KYPULSE_DB_DRIVER=postgres` and `KYPULSE_DB_DSN`. Backups snapshot SQLite
only, so a PostgreSQL deployment backs up its database separately.

The published image is `ghcr.io/busnes-app/kypulse-server`. `docker-compose.yml` explains how
to pin a commit by digest (`KYPULSE_IMAGE`) and verify its build attestation first.

## Backup and restore

kyPulse backs up to KyRecovery like every suite product, and to a local directory when
`KYPULSE_BACKUP_DIR` is set. Backups are sealed capsules that only the suite's recovery
custodians can open together. [`docs/RESTORE.md`](docs/RESTORE.md) is the restore runbook.

kyPulse's own audit trail (sign-ins, admin changes, alert deliveries, pairings) is a keyed
hash chain. `kypulse audit-verify` walks it and exits 1 if a record was altered or removed;
every start logs the chain's count and head (`audit_chain_placed`), which is your copy
outside the database. `/healthz` reports `audit` degraded when the chain cannot take the
next record, and a log that cannot be placed refuses to start with the remedy in the error.

## Command line

```sh
kypulse                                   # run the server
kypulse init-admin -password <pw> [-username admin]   # create an admin, or reset its password
kypulse audit-verify                      # verify the audit chain
kypulse backup-drill                      # seal and reopen a capsule with a throwaway key
kypulse export-capsule [-out <file>]      # write a sealed capsule to a file
kypulse deposit                           # run one backup now (for cron)
kypulse restore -capsule <f> -to <dir>    # restore a capsule; shares are read from stdin
kypulse version
```

Under Compose, prefix with `docker compose exec app /app/kypulse`.

## API

Routes answer JSON, errors are `{"error": "..."}`, and state-changing browser requests need
the `ky_csrf` cookie echoed in `X-CSRF-Token`. [`internal/api/AGENTS.md`](internal/api/AGENTS.md)
has every request and response shape.

| Route | Who |
|---|---|
| `GET /healthz` | public, `ky.health/1` |
| `GET /api/status`, `/api/targets`, `/api/targets/{id}`, `/api/alerts`, `/api/kyyard` | any signed-in user |
| `POST/PUT/DELETE /api/targets…`, `POST /api/targets/{id}/silence` | admin |
| `GET/PUT/DELETE /api/alerts/webhook`, `POST /api/alerts/webhook/test` | admin |
| `POST /api/kyyard/pair`, `DELETE /api/kyyard`, `GET /api/kyyard/containers` | admin |
| `POST /api/log-sources/pairing`, `GET /api/log-sources`, `DELETE /api/log-sources/{id}` | admin |
| `POST /api/log-sources/claim` | public |
| `POST /api/ingest/logs` | source Bearer token |
| `GET /api/logs`, `GET /api/activity` | admin |
| `/api/backup/…` | admin |

## Develop

Go 1.26 and Node 22.

```sh
make ci             # tidy check, gofmt, vet, race tests, frontend tests, smoke test
make run            # build and run on :8080 with ./data
make test-postgres  # the Go suite against KYPULSE_TEST_POSTGRES_DSN
```

The frontend lives in `web/` and is embedded from `web/dist`, which is committed; rebuild it
with `cd web && npm ci && npm run build` after any frontend change, because CI diffs it.
Browser regressions run with `go build -o .browser/server ./cmd/server`, then
`cd web && npx playwright install chromium && npm run test:browser`; they need a private IPv4
interface for the fake watched app. Contributor rules live in [`AGENTS.md`](AGENTS.md) and the
`AGENTS.md` beside each package.

## License

See [`LICENSE.txt`](LICENSE.txt).
