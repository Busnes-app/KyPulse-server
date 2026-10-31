# KyPulse standalone installation

## Before you start

Read [Key strength and TLS](docs/STRONG-CRYPTO.md) for supported settings,
signing-key floors and preserving audit, sender and recovery material.

These steps are for a **new, independent installation** from this repository. Use the
README’s upgrade/restore procedure for an existing instance. Keep its volumes, keys,
configuration and Compose overlays; do not replace its environment files with examples.

Use Linux with Docker Engine and Docker Compose 2.24 or newer, Git, and permission to use Docker.
Choose a release/revision and use its matching Compose files and documentation. Image tags
can move: follow the README/Compose attestation and digest-pinning procedure before relying
on a published image. Source builds use the separate build overlay where provided.

Clone this repository, enter its directory, then run the product steps below:

```sh
git clone https://github.com/Busnes-app/KyPulse-server.git
cd KyPulse-server
```

Each service needs its own directory and persistent storage. Several products use host
port 8080 by default; install them on separate hosts or configure distinct host ports and
matching public origins. For remote users, provide a TLS reverse proxy and a hostname you
control. Trust only the proxy’s actual peer address, and forward the original scheme/client
address according to the product’s proxy guide.

## Install monitoring

KyPulse runs with local operator accounts and SQLite without KyIdentity or KyYard. Health
sources and an alert webhook are added after installation. KyYard enables optional
container facts and log collection.

```sh
[ -e .env ] || (umask 077; touch .env)
chmod 600 .env
mkdir -p data backups secrets
sudo chown 10001:10001 data backups secrets
docker compose config --quiet
docker compose pull
docker compose up -d
```

The image runs as UID 10001 with a read-only root filesystem. Without permission to assign
these bind directories, use the [named-volume override](docs/RUNNING.md#without-root-named-volumes).
Read the generated bootstrap password privately with
`docker compose exec app cat /app/data/initial-admin-password` (the log names only this
0600 file, never the password), sign in as `admin` at `http://localhost:8080`, replace it and
enroll MFA under Settings > Two-factor authentication, then delete the file with
`docker compose exec app rm /app/data/initial-admin-password`. Alternatively provide a
unique `KYPULSE_ADMIN_PASSWORD` in the protected `.env` for first bootstrap, then remove it
after replacement.

The generated password is printed once, by the first container, and exists nowhere else.
Read it before any restart or `.env` change: a recreated container has a new log, and the
database already holds the admin, so it is not printed again. If it is lost, set a new one
with `docker compose exec app /app/kypulse init-admin -password '<at least 12 characters>'`;
sign-in then asks for a replacement.

For HTTPS users, set `KYPULSE_APP_URL=https://pulse.example.com` and
`KYPULSE_TRUSTED_PROXIES` to the proxy’s exact peer address in `.env`. The default bind is
loopback (`KYPULSE_BIND=127.0.0.1`). Restart to apply environment changes.

Check `/healthz`, add a compatible service endpoint, wait for a real poll, then verify a
controlled failure/recovery and the actual webhook receiver. Merely saving an alert URL
does not prove notification delivery. Keep `data`, `backups` and the separate `secrets`
audit key together in the documented recovery process; do not regenerate a lost key.

Source builds retain `docker-compose.build.yml` in `COMPOSE_FILE` and use
`docker compose up -d --build`. See the [README](README.md) for integrations, audit key
handling and recovery.

## Keep it recoverable

Preserve the database **and deployment keys**, not just the image. Configure recovery using
the linked restore runbook, retain an independent copy, and complete a scratch restore with
custodian cards before treating recovery as verified. A synthetic drill uses a throwaway
key and does not prove that the real cards are available or correct.

For upgrades, back up first, read the chosen release’s migration notes, and reuse the same
Compose project, volume mounts and overlay selection for every command. `docker compose
pull` followed by `docker compose up -d` updates a published-image installation; a digest pin
must be deliberately changed after verification. A source install must keep its build
overlay and rebuild. `docker compose down -v` deletes managed volumes: it is not an upgrade.

## What verification means

The startup checks above prove only the checks they perform. Use the README’s development
section and the repository CI workflow for source/build tests; a passing health response or
screenshot is not a substitute. Record the revision, commands, environment and skips when
reporting results. Design plans and dated evidence remain records of their stated scope.
