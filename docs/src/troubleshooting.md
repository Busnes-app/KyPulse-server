# Troubleshooting

- **Startup permission denied:** check UID 10001 ownership of data/backups/secrets, or use the named-volume setup.
- **Cannot open it from another machine:** the default binding is loopback. Configure HTTPS and the documented bind/proxy settings.
- **App marked down:** inspect endpoint, DNS, TLS and the app's actual health response before changing alert thresholds.
- **Container facts stale:** investigate the KyYard connection separately from application polling.
- **No webhook alert:** verify the receiver, endpoint configuration and a controlled delivery test.
- **No logs:** check source pairing, sender credentials and ingestion connectivity.

When reporting a problem, include the product version, platform, failed task and a redacted error. Exclude pairing links, device secrets, recovery codes, custodian shares and private content.
