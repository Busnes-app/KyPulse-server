# Getting started

## Start kyPulse

Prepare `data`, `backups` and `secrets` with ownership for container UID 10001, or use the documented named-volume alternative. Start Compose and read the bootstrap password privately with `docker compose exec app cat /app/data/initial-admin-password`. The password is not logged. Replace it at first sign-in and follow the installation guide's instruction to remove the bootstrap file.

The default host port is loopback 8080. For access beyond the host, configure an HTTPS proxy, `KYPULSE_APP_URL` and the actual trusted proxy address.

## Add a watched application

Configure the app's health endpoint and verify that Status reflects a real probe. Pair KyYard separately for container facts. Configure an alert webhook and test delivery before depending on it. Protect the durable audit key as part of recovery.
