# kyPulse

Unified health and logging for the Busnes.app Ky suite. kyPulse polls every Ky app's
`/healthz`, pulls container state and logs from KyYard, accepts logs from paired senders, and
sends an outbound webhook when an app goes down or degrades. It is read-only: it watches and
reports, and never changes the apps it watches.

Design: `docs/superpowers/specs/2026-09-26-kypulse-design.md`.

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

## Backup and restore

kyPulse backs up to KyRecovery like every suite product. `docs/RESTORE.md` is the restore
runbook.

## Develop

```sh
make ci      # tidy, gofmt, vet, race tests, frontend tests, smoke test
```
