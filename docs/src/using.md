# Everyday tasks

## Health and alerts

Start with Status and Alerts, then open the affected application's details. Distinguish a failed health probe from stale container facts. kyPulse reports observations; use the owning product or KyYard to make a deliberate repair.

## Logs and activity

Pair a log source before ingestion. Linux hosts can run `kypulse-send` using the documented source credential and endpoint. Use Logs and Activity for investigation and configure retention according to the data you intend to keep. Avoid shipping secrets in application logs.

## Notifications

Keep the alert webhook reachable and test the destination when it changes. A healthy monitoring screen does not prove that an external receiver accepted an alert.

## Backups

Configure a recovery key, destination and schedule. Preserve the audit key and follow the restore guide; copies on the same host do not protect against losing that host.
