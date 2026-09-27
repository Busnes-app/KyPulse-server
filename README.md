# kyPulse

Unified health and logging for the Busnes.app Ky suite. kyPulse polls every Ky app's
`/healthz`, pulls container state and logs from KyYard, accepts logs from paired senders, and
sends an outbound webhook when an app goes down or degrades. It is read-only: it watches and
reports, and never changes the apps it watches.

Design: `docs/superpowers/specs/2026-09-26-kypulse-design.md`.

## Screens

Status shows every watched app as a grid, broken apps first. Alerts lists state transitions,
deliveries and silences. Each app has a detail page with its current state and history, and
buttons to silence for 1h, 8h or until fixed. An alert bar surfaces active problems above
every page. Settings & DB holds the alert webhook form (admin) alongside database info.
Admins get every screen and control; viewers get Status, Alerts, app detail and a read-only
Settings & DB.

## Run

```sh
docker compose up -d
```

The published image is `ghcr.io/busnes-app/kypulse-server`; `docker-compose.yml` explains how
to pin a commit by digest and verify its attestation. Every setting is a `KYPULSE_*` variable
(see `internal/config/AGENTS.md`); `KY_LOG_LEVEL` sets the log level and is shared across the
suite. The first start creates an `admin` user and prints its password to stderr unless
`KYPULSE_ADMIN_PASSWORD` is set.

Watch kyPulse's own `GET /healthz` from outside; kyPulse does not monitor itself.

Watched apps and the alert webhook are set in the UI (admins) or through `/api/targets` and
`/api/alerts/webhook`; `KYPULSE_ALERT_ALLOW_HTTP` and `KYPULSE_POLL_WORKERS` are the only
alerting variables.

## Compose overlays

Append an overlay's filename to `COMPOSE_FILE` in `.env` (never `-f`, which replaces the list
outright); each file's header comment has a copy-pasteable one-liner that does this for you.

**Source build** (`docker-compose.build.yml`). Builds and runs the local image (`kypulse:local`)
instead of the published one, for every command including restore. Revert by removing it from
`COMPOSE_FILE`.

**LAN DNS and private KyRecovery** (`docker-compose.lan-dns.yml`). Points the container's DNS
at `KYPULSE_DNS`, a LAN resolver, so a KyRecovery reachable only there resolves; it also sets
`KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY=true`, admitting RFC1918/CGNAT KyRecovery destinations
(loopback and other reserved ranges stay refused; HTTPS is still required). Revert by removing
the overlay from `COMPOSE_FILE` and unsetting `KYPULSE_DNS` and
`KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY` in `.env`.

**Static IP** (`docker-compose.static-ip.yml`). Pins the container to `KYPULSE_CONTAINER_IP` on
a network with subnet `KYPULSE_NETWORK_SUBNET`, both required once the overlay is in the chain.

## Backup and restore

kyPulse backs up to KyRecovery like every suite product. `docs/RESTORE.md` is the restore
runbook.

kyPulse's own audit trail is a keyed hash chain; `kypulse audit-verify` checks it and
`/healthz` reports `audit` down when it cannot be appended to.

## Develop

```sh
make ci      # tidy, gofmt, vet, race tests, frontend tests, smoke test
```
