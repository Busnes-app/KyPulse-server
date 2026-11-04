# Operations

[Product overview](../README.md) · [Standalone installation](../INSTALL.md)

Commands and unlinked file paths below are relative to the repository root.

## Backup and restore

KyPulse backs up to KyRecovery like every suite product, and to a local directory when
`KYPULSE_BACKUP_DIR` is set. Backups are sealed capsules that only the suite's recovery
custodians can open together. [`docs/RESTORE.md`](RESTORE.md) is the restore runbook.

KyPulse's own audit trail (sign-ins, admin changes, alert deliveries, pairings) is a keyed
hash chain. `kypulse audit-verify` walks it and exits 1 if a record was altered or removed;
every start logs the chain's count and head (`audit_chain_placed`), which is your copy
outside the database. `/healthz` reports `audit` degraded when the chain cannot take the
next record, and a log that cannot be placed refuses to start with the remedy in the error.

## Passwords

A local account changes its own password under **Settings > Account > Password** by giving the
current one. Every session of that account, the current one included, is signed out, so it signs
in again with the new password. Accounts that sign in through KyIdentity change it there.

## KyIdentity administrator roles before upgrading

Before upgrading, assign `kypulse.admin` (or `admin`) to every dedicated KyPulse
administrator in the KyPulse application's KyIdentity role assignments. A global
KyIdentity administrator role alone does not grant application administrator access.
Verify the application's ID token contains that role in its `roles` array before
upgrading; keep a working local recovery administrator available during the change.

At each sign-in, a present per-app `roles` array is authoritative: an empty array,
or one without either administrator role, makes the account a viewer and ends its
other sessions when the role changes. If `roles` is absent, the legacy `role` claim
is used when present; with neither claim an existing role stays unchanged.
Installer-owned identities remain governed by their directory assignments.

## Two-factor authentication

MFA is a KyIdentity feature. KyPulse has no second factor of its own: for one, sign in with
KyIdentity and turn MFA on there.

**Removed: local two-factor sign-in.** Earlier versions let a local account turn on TOTP under
Settings. The Settings card, the code and recovery-code step at sign-in and the
`/api/auth/mfa/*` routes are gone. Admins who want MFA should move their people to KyIdentity
sign-in and enable it there. A local account that had TOTP on now signs in with its password
alone, so nobody is locked out by a lost factor. The stored TOTP data (sealed secrets,
recovery-code hashes) is kept in the database, unused; nothing is cleared.

## Command line

```sh
kypulse                                   # run the server
kypulse init-admin -password <pw> [-username admin] [-promote]   # create an admin, or reset its password; -promote makes an existing non-admin local account an admin
kypulse audit-verify                      # verify the audit chain
kypulse backup-drill                      # seal and reopen a capsule with a throwaway key; exits 1 when the drill fails
kypulse export-capsule [-out <file>]      # write a sealed capsule to a file
kypulse deposit                           # run one backup now (for cron)
kypulse restore -capsule <f> -to <dir>    # restore a capsule; shares are read from stdin
kypulse version
```

Under Compose, prefix with `docker compose exec app /app/kypulse`.
