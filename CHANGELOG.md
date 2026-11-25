# Changelog

## 0.6.1 — pending

- The bundled PostgreSQL (`--profile postgres`) moves from 17 to 18 (`18.7`, pinned by digest), in a new `postgres` volume. The next `up` dumps the 17 `pgdata` volume once and restores it into 18; `pgdata` is kept for rollback, and removing it is the operator's choice. It needs about twice the 17 data in free disk. Read [PostgreSQL 17 to 18](docs/CONFIGURATION.md#postgresql-17-to-18) before upgrading.

- After a rollback to 17 that ran 17 again, the next `up` refuses to start the older 18 and says how to move 17 again or keep 18 (shared upgrade script, kit v4). The check still holds after `upgrade-from-17` is removed.

- Keep SQLite `v1.62.0` with its upstream-required libc `v1.80.1` and memory `v1.12.1`; defer independent updates of those runtime dependencies.

- Prepare application version `0.6.1` (from `0.6.0`) for the compatible dependency refresh. Rebuild or install the updated application; this version bump requires no additional data, account, key or configuration migration.
