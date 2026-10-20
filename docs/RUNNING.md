# Running

[Product overview](../README.md) · [Standalone installation](../INSTALL.md)

Commands and unlinked file paths below are relative to the repository root.

## Quick start

```sh
mkdir -p data backups secrets && sudo chown 10001:10001 data backups secrets
docker compose up -d
docker compose logs app | grep 'Initial bootstrap'   # the generated admin password
```

The container runs as uid 10001 on a read-only root filesystem with every capability
dropped, so the three directories must belong to that uid; Docker would otherwise create them
owned by root and the server refuses to start with `permission denied`. Without root, use
named volumes instead (below).

Open <http://localhost:8080>, sign in as `admin` with that password, and replace it when
asked. Set `KYPULSE_ADMIN_PASSWORD` in `.env` before the first start to choose it instead; it
is read only while the database has no users and stays visible in `docker inspect`, so remove
it afterwards. Compose runs in `KYPULSE_ENV=production`, so session cookies are `Secure`: a
browser on the same machine can use `http://localhost`, anyone else needs kyPulse behind a TLS
proxy with `KYPULSE_APP_URL=https://...` and `KYPULSE_TRUSTED_PROXIES` (the proxy's own
address) in `.env`.

Compose publishes port 8080 on `127.0.0.1` only. A proxy on the same host reaches it there;
set `KYPULSE_BIND` in `.env` to publish on another address (`0.0.0.0` for every interface).

### Without root: named volumes

Compose reads `docker-compose.override.yml` beside it automatically (unless `COMPOSE_FILE` is
set in `.env`; then add it to that chain). This one replaces the three bind mounts:

```yaml
services:
  app:
    volumes:
      - kypulse-data:/app/data
      - kypulse-backups:/app/backups
      - kypulse-secrets:/app/secrets
volumes:
  kypulse-data:
  kypulse-backups:
  kypulse-secrets:
```

A new named volume takes its owner (uid 10001, mode 700) from the image, so skip the
`mkdir`/`chown` line and run `docker compose up -d`. Back up the volumes, not `./data`;
[docs/RESTORE.md](RESTORE.md) is written for the bind mount, so adapt its host-path steps
to the volume.

### Upgrading: port 8080 now on loopback

Compose used to publish `8080` on every interface. It now binds `127.0.0.1` unless
`KYPULSE_BIND` is set. If a proxy on another host, or clients on the LAN, reached kyPulse
directly on `:8080`, set `KYPULSE_BIND=0.0.0.0` in `.env` to restore the old behaviour, or
better, put a TLS proxy in front.

### Upgrading to a non-root container

Images before this change ran as root, so an existing `./data` and `./backups` are root-owned
and the new image cannot open them. Once, before `docker compose up -d`:

```sh
docker compose down
mkdir -p secrets && sudo chown -R 10001:10001 data backups secrets
```

Then move the audit key out of the database directory (next section).

### Audit key

The audit key signs the audit hash chain. Next to `kypulse.db`, anyone who can rewrite the
database can also re-sign the chain, so Compose keeps it in `./secrets`
(`KYPULSE_AUDIT_KEY_FILE=/app/secrets/audit.key`). An existing `data/audit.key` is still used
while `secrets/audit.key` does not exist, and every start then logs `audit_key_beside_db`.
Move it once, with the server stopped:

```sh
docker compose down && sudo mv -n data/audit.key secrets/audit.key && docker compose up -d
```

After the first start you may mount `./secrets` read-only (`./secrets:/app/secrets:ro`); a
fresh install needs it writable once to mint the key. Back `./secrets` up with `./data`.

Watch kyPulse's own `GET /healthz` from outside. kyPulse does not monitor itself.
