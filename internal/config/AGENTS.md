# Config

## Purpose
Manages environment and file-based configuration loading, defaults, and type conversions for kypulse.

## Ownership
Owns environment variable parsing, configuration validation, default fallbacks, and security key generations.

## Local Contracts
- `LoadFromEnv() (*Config, error)` must supply safe, valid defaults for all subsystems.
- Never log plaintext secrets or sensitive tokens.
- `KYPULSE_TRUSTED_PROXIES` is a comma-separated list of reverse-proxy IPs or CIDRs, empty by default, parsed once at startup into `[]netip.Prefix`; an unparsable entry fails startup. Only a request whose peer address is in the list may speak for another client through `X-Forwarded-For`. `0.0.0.0/0` and `::/0` are refused at startup; list only the proxy's own address or subnet.
- Production startup requires an explicit, durable `KYPULSE_SESSION_SECRET`. The encryption key comes from `KYPULSE_ENCRYPTION_KEY` when set, otherwise from the keyfile at `<DataDir>/encryption.key`, which `keyfile.LoadOrCreate` mints on first start; either is a valid production configuration. The audit chain key comes from `KYPULSE_AUDIT_KEY` (32 bytes, hex or base64) or `<DataDir>/audit.key`, minted on first start; it is never the encryption key.

- `KYPULSE_BACKUP_DEPOSIT_INTERVAL` is a Go duration (default `24h`), only the default for the schedule the admin screen stores; `0` is off, anything else below `MinDepositInterval` (15m) or negative fails startup. `KYPULSE_BACKUP_DIR` (default empty, off) is the sealed local-copy directory and `KYPULSE_BACKUP_KEEP` (default 7) how many to retain; below 1 fails startup because the lib refuses it at write time. `KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY` (default false) admits RFC1918 and CGNAT KyRecovery destinations only.

- `KYPULSE_ALERT_ALLOW_HTTP` (default false) admits a plain-http webhook URL; health targets may always be plain http. `KYPULSE_POLL_WORKERS` (default 4, 1..32) bounds concurrent polls. `KYPULSE_KYYARD_ALLOW_HTTP` (default false) admits a plain-http KyYard URL; the pairing itself is a sealed setting, not env config.
- `KYPULSE_LOG_MAX_BYTES` is decimal logical retained log and activity bytes, default 1 GiB, minimum 128 KiB. Malformed, empty, overflow, zero and negative values fail startup.

- `KY_LOG_LEVEL` (not `KYPULSE_`) is read by `ky-primitives/logging` and sets the level for every line; it is the one suite-wide variable. Output is JSON lines on stderr; there is no log file.

## Verification
- `go test -v ./internal/config/...`
- `go test -v ./internal/auth/ -run TestClientIP` (the helper that consumes the allowlist)

## Child DOX Index
None.
