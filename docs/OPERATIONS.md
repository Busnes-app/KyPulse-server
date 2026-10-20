# Operations

[Product overview](../README.md) · [Standalone installation](../INSTALL.md)

Commands and unlinked file paths below are relative to the repository root.

## Backup and restore

kyPulse backs up to KyRecovery like every suite product, and to a local directory when
`KYPULSE_BACKUP_DIR` is set. Backups are sealed capsules that only the suite's recovery
custodians can open together. [`docs/RESTORE.md`](RESTORE.md) is the restore runbook.

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
