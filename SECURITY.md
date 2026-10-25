# Security Policy

kyPulse aggregates health and logs from the suite's Ky apps. It centralizes operational data, which makes it a sensitive collection point.

## Reporting vulnerabilities

Report security vulnerabilities privately through GitHub Security Advisories:
<https://github.com/Busnes-app/KyPulse-server/security/advisories>. Do not open a public issue.
Include the affected area, reproduction steps, impact, and affected versions.
We aim to acknowledge reports within two business days and will coordinate a fix
and disclosure.

## Trust boundaries and limitations

- kyPulse concentrates logs and health data from many products; its data store is as sensitive as the most sensitive thing any product logs. Keep it private and access-controlled.
- It polls product endpoints over HTTPS with configured credentials; those credentials are handled with least privilege and never logged.
- Aggregated views are authorized per identity; there is no unauthenticated status page that leaks topology or errors.

## Deployment requirements

- Run behind HTTPS; never add a default that downgrades to cleartext.
- Keep the database and backups private; stop the server before copying the data directory.
- Set retention limits on stored logs and rate limits on authentication.

## Security-sensitive changes

Changes to authentication, what data is collected or displayed, poller credentials, or log retention require tests for both the expected path and the relevant
failure or attack path, plus a description of the trust boundary in the pull
request.

## Related documents

- [CONTRIBUTING.md](CONTRIBUTING.md) — contribution and review requirements
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — community expectations
- [README.md](README.md) — what this project is and how to run it

