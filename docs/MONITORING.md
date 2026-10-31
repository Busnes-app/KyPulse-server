# Monitoring

[Product overview](../README.md) · [Standalone installation](../INSTALL.md)

Commands and unlinked file paths below are relative to the repository root.

## Watching apps

An admin adds an app with a name, its health URL and an interval (default 30 s, 10 s to 1 h).
KyPulse reads any health endpoint, in this order:

1. A 2xx with `"schema": "ky.health/1"` is used as-is, checks included
   ([`ky-primitives/health`](https://github.com/Busnes-app/ky-primitives) serves it).
2. A 2xx JSON body with `status` of `ok`, `alive`, `ready` or `healthy` is ok; `degraded`
   is degraded; a boolean `healthy` is ok or degraded.
3. Any other 2xx is ok, shown as **basic** (no checks).
4. A non-2xx answer, timeout, TLS or DNS failure, or refused connection is down, with the
   cause recorded.

A request times out after 5 s, reads at most 64 KiB and never follows redirects. Private and
LAN addresses are allowed; loopback, link-local and cloud metadata addresses are refused,
also when a name resolves to one at connect time.

An app turns **down** after 3 down polls in a row, **degraded** after 2, and **ok** after 2.
Each change sends one webhook message and is recorded under Alerts. While an app stays down
or degraded, a reminder goes out every hour. Silencing (1 h, 8 h or until fixed) stops
webhook messages only; the alert bar and Alerts keep showing the problem, and "until fixed"
ends at the next recovery.

## Alert webhook

Set in Settings by an admin, sealed at rest with the deployment key. Presets:

| Preset | Sends |
|---|---|
| ntfy | text body with title and priority headers; optional access token |
| Gotify | required app token in `X-Gotify-Key`, never in the URL |
| Discord | JSON `content` |
| Generic | `{"app","state","previous","reason","time","url"}`; optional bearer token |

The token is write-only: it is never shown again. Leaving the field empty keeps it only
when the preset, URL scheme and host (including port) stay the same. A Gotify edit that
would leave no app token is refused and preserves the previous configuration.
**Send test** makes one attempt and shows the answer. Real alerts retry 3 times with backoff;
continued failure raises "Alerts not being delivered" in the alert bar. Messages carry the
app name, the transition, a reason code, the time and a link, never log lines, user names or
IPs. Webhook URLs must be https unless `KYPULSE_ALERT_ALLOW_HTTP=true`.

## KyYard

KyPulse can read one KyYard organization with a read-only service token.

1. A KyYard organization administrator opens **Members → Service tokens → Pair kyPulse** and
   gets a six-digit code, valid 15 minutes.
2. A KyPulse admin enters the KyYard URL and that code on the KyYard card in Settings.

The token is sealed at rest, never logged and never shown. KyPulse then pulls every 60 s:
Docker endpoints that are approved, active or offline (the first 200), their container
inventory and latest resource samples. Inventory and samples live in memory and a restart
re-pulls them. A separate 60-second worker collects linked-container logs and organization
audit activity, with durable cursors committed atomically with each imported batch.

Link a watched app to a container (`endpoint/name`; admins get suggestions in the add and
edit form) and its detail page shows state and exit code, Docker health, image, memory
against its limit and restarts with the sample's age, restarts in the last hour, and when it
was observed. Memory and restarts appear only once KyYard has a sample (it samples running
containers only); a container on an offline endpoint is marked and shown stale.

Right after pairing or a restart the card and the bar say **first pull pending**. KyYard data
is shown **stale** when no pull has succeeded for 3 minutes, and at once when KyYard refuses
the token (it was revoked: unpair and pair again). Admin Logs, Activity and Settings show
independent inventory, container-log and audit-feed last-success ages and errors. A failed
collector does not stop health polling or clear another collector's error.

Only resolved linked containers are collected, including links on apps with health polling
paused. Several watched apps sharing one container cause one fetch and an attributed copy
per app. Log requests use the saved inclusive timestamp, so equal-timestamp duplicates are
expected. Each pull keeps at most 1,000 application lines, 16 KiB per line, under a 32 MiB
response cap and 30-second request timeout. Matching KyYard notices and local truncation
notices are retained as collector warnings. A full tail says **history may be incomplete:
pull reached 1000 lines**: Docker's newest-N tail is not lossless pagination. For complete
high-volume collection use file/Docker sender input or an existing external collector.

The audit feed requires KyYard's `after_id` cursor API (`items`, `next_after_id`), reads 200
ascending rows per page and at most five pages per tick, and commits each page before the
next request. An older KyYard's array response produces `audit_cursor_unsupported` without
advancing its cursor; update KyYard to enable this collector. Container logs and health
continue independently. Remote audit chain verification is not performed. Re-pairing uses
a fresh generation and starts new cursors; already imported history remains until retention.

Unpairing takes two steps, one on each side: **Unpair** here deletes the URL and token in
KyPulse only; a KyYard administrator must also revoke the token on the Members page. The
same applies when you pair again: the previous token stays valid in KyYard until revoked
there. KyYard URLs must be https unless `KYPULSE_KYYARD_ALLOW_HTTP=true`.
