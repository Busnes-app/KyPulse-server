# kyPulse design

Date: 2026-09-26. Status: approved in conversation, awaiting spec review.

## Goal

One small, read-only screen set for the whole Ky suite that tells an admin what is going on,
what needs attention, and what people are doing, and pushes an alert when an app goes down.

kyPulse polls each app's health endpoint, pulls container state and logs from KyYard, accepts
logs from paired senders for hosts KyYard does not cover, and sends outage alerts to a webhook.
It never changes anything in the apps it watches.

## Decisions

| Topic | Decision |
|---|---|
| Health | One `ky.health/1` JSON shape on `GET /healthz` in every app; kyPulse still reads apps that return less |
| Log sources | KyYard first; an ingest endpoint plus `kypulse-send` for everything else |
| App logging | Every app writes to stderr only |
| Watched apps | Admin-maintained list; KyYard containers are offered as suggestions, never polled unapproved |
| Alerts | Outbound webhook with ntfy, Gotify, Discord and generic presets |
| Alert triggers | App down, app degraded |
| Alert noise | One message per state change and recovery, hourly reminder, per-app silence |
| Retention | Logs and activity 7 days, plus a size cap |
| Sender inputs | Docker container stderr, files, stdin |
| KyYard access | New read-only service token in KyYard, obtained by pairing code |
| Sender access | Per-source token obtained by pairing code |
| Network | Private and LAN targets allowed; loopback, link-local and redirects refused |
| Roles | `admin` sees everything; `viewer` sees Status and Alerts only |
| kyPulse self-monitoring | None in kyPulse; docs tell the operator to watch kyPulse's `/healthz` from outside |
| Architecture | One kyPulse binary on the ky_server_base scaffold, plus a separate `kypulse-send` binary |

### Relation to the suite logging design

`ky-primitives/docs/superpowers/specs/2026-09-02-logging-design.md` lists a Ky log server as a
non-goal and routes logs to an off-the-shelf collector. kyPulse changes that for the suite: it
is the collector's destination for short-term, admin-facing triage. It stays small by
design. It keeps 7 days under a hard size cap, reads KyYard before accepting pushes, exposes
no query API beyond its own screens, and leaves long-term archive to external platforms.
Operators who run Loki or Graylog keep doing so; stderr output is unchanged.

## Screens

Top-level tabs: **Status**, **Alerts**, **Activity**, **Logs**. Activity and Logs are admin-only.

- **Alert bar.** It sits above the tabs on every page. It is red when any watched app is down
  or degraded, with one line per problem and a link to Alerts. When all apps are healthy it is
  green, showing the app count and the time of the last check. It also shows "alerts not
  being delivered" when the webhook keeps failing, and marks KyYard data stale when KyYard is
  unreachable.
- **Status.** A grid of watched apps, broken ones first, with a coloured dot for `ok`,
  `degraded`, `down` or `basic`.
- **Alerts.** Open and recent state changes, delivery results and silences.
- **Activity.** One timeline of audit events across apps, filterable by app, user and outcome.
  Bursts of failed sign-ins are highlighted. Apps with no audit events show "no audit events:
  not on shared logging".
- **Logs.** Stored lines filtered by app, level, time range and text.
- **App detail** (click a tile). It shows:
  - A banner with the current state, the time since the last change and the exact failing
    request.
  - Checks from the last good response.
  - KyYard container facts: state, exit code, image tag, restarts in the last hour, and memory
    against its limit.
  - Alert history and silence controls: 1 h, 8 h or until fixed.
  - Recent log lines for the app.

## 1. Health contract (`ky-primitives/health`)

Each app serves `GET /healthz` with no auth. It returns 200 when the status is `ok` or
`degraded`, and 503 when `down`. Those codes suit a readiness probe or a monitor. Liveness
probes stay on a route that checks nothing: restarting an app does not fix its dependency.

```json
{
  "schema": "ky.health/1",
  "service": "kyvault",
  "status": "degraded",
  "time": "2026-09-26T10:41:00Z",
  "checks": [
    {"name": "database", "status": "ok"},
    {"name": "audit", "status": "degraded", "reason": "append_disabled"}
  ]
}
```

- `status` is the worst check status. A service with no checks is `ok`.
- A check is a function returning `error`, with a per-check deadline defaulting to 2 s and
  capped at 4 s (`MaxTimeout`), so a slow check can never hold `/healthz` past kyPulse's
  own 5 s request timeout.
  - A timeout is `down` with reason `timeout`.
  - A check still running from a previous poll is not started again; it reports `timeout`.
  - A panicking check is `down`.
- One evaluation answers every request for 5 s, and concurrent requests share it. The route
  is public, so without this each request would run every check against the app's
  dependencies.
- The package never copies error text into the response, because error text can hold DSNs,
  paths or hostnames.
  - A failed check is reported as status `down` with no reason.
  - A check reports `degraded`, or adds a `reason`, only by returning `health.Degrade(r)` or
    `health.Fail(r)` with a `health.Reason`.
  - A `health.Reason` comes from `health.DeclareReason`, called at package level. Its code
    must match `^[a-z0-9_]{1,64}$`, and an invalid code panics at startup.
  - Each non-ok check writes a `health_check_failed` line to the app's stderr through
    ky-primitives `logging`, with the check name, reason and `logging.Err` error kind.
- No version, build or uptime fields. On a public route those help attackers fingerprint
  releases and reveal restart timing. kyPulse takes the version from the KyYard image tag and
  restarts from KyYard.
- `service` matches `[A-Za-z0-9][A-Za-z0-9_.-]{0,63}`.

Adoption per app:

| App | Change |
|---|---|
| KyIdentity | `/healthz` moves from `{"status":"alive"}` to the contract; `/readyz` unchanged |
| kydns, kynotes, kybookmarks, KyVault | Serve `/healthz` on the contract; existing health paths kept as aliases for one release |
| kyrecovery, KyForge | Add a public `/healthz` (none today) |
| KyYard | Add `/healthz`; `/health/live` and `/health/ready` unchanged |
| kypost | Add `/healthz`; `/api/health` unchanged for its clients |
| kyPulse | Serves `/healthz` from the start |

### Reading responses in kyPulse

The poller normalises every response, in order:

1. A 2xx with `schema: "ky.health/1"` is used as-is.
2. A 2xx JSON body with a known status field maps to `ok` or `degraded`:
   - `status` of `ok`, `alive`, `ready` or `healthy` means `ok`; `degraded` means `degraded`.
   - A boolean `healthy` means `ok` when true and `degraded` when false.
3. Any other 2xx is `ok`, displayed as `basic` ("basic health, no checks").
4. A non-2xx status, timeout, TLS failure, DNS failure or refused connection is `down`. The
   specific cause is recorded for the detail page.

Limits: the response body is capped at 64 KiB, the request times out after 5 s, and redirects
are not followed. The fixtures are the real current responses of every app, including
kybookmarks' always-200 `degraded` body and kydns' plain `ok`.

## 2. kyPulse server

Built on the existing ky_server_base scaffold in this repo.

### Scaffold changes

- **Rename.**
  - Module `github.com/Busnes-app/kypulse-server`.
  - Env prefix `KYPULSE_` (`KY_*` names from the scaffold are renamed).
  - Binaries `kypulse` and `kypulse-send`.
- **Remove** device QR pairing (`/api/devices/*`, `internal/devices`) and SCIM
  (`/scim/v2`, `internal/scim`).
- **Keep** local login with MFA, SSO including KyIdentity, sessions, CAPTCHA, settings, theme
  and kyrecovery backup.
- **Backup scope.** The capsule contains settings, watched apps, sealed tokens, webhook
  configuration and users. Logs and activity are excluded: they are re-creatable, may hold
  personal data, and would bloat the capsule.
- **Roles.** Users are `admin` or `viewer`, and new users default to `viewer`. Every route
  enforces the role server-side.
- **Own logs.** kyPulse writes to stderr through ky-primitives `logging`. Its own audit trail
  uses `auditchain`.

### Packages

| Package | Responsibility |
|---|---|
| `internal/targets` | Watched-app CRUD and validation |
| `internal/egress` | Shared outbound HTTP client and the address guard |
| `internal/poller` | Scheduled health polls and response normalisation |
| `internal/alerts` | Per-app state machine, reminders, silences |
| `internal/notify` | Webhook presets, delivery, retries |
| `internal/kyyard` | KyYard pairing, inventory and log pulls |
| `internal/ingest` | Source pairing and the ingest endpoint |
| `internal/logstore` | Log and activity storage, retention, queries |
| `cmd/kypulse-send` | The sender binary |

Business rules (normalisation, state transitions, retention selection) are pure functions;
HTTP handlers and the scheduler call them.

### Outbound network guard

One `internal/egress` client, modelled on `ky-primitives/recoveryclient`'s guard, serves
health polls, KyYard and webhooks.

- Addresses are checked at connect time, so a DNS rebind cannot slip past a check at
  resolution.
- Private and LAN addresses are allowed. Loopback, link-local (including cloud metadata),
  unspecified and multicast addresses are always refused.
- Redirects are refused.
- HTTPS is required, with two exceptions:
  - Health targets may be plain HTTP, since they are public, unauthenticated GETs.
  - A webhook may be plain HTTP only with `KYPULSE_ALERT_ALLOW_HTTP=true`, which is off by
    default. Enabling it is recorded in the audit trail.
- KyYard is always HTTPS.

### Watched apps and polling

A target has a name, health URL, interval (default 30 s, minimum 10 s), enabled flag, an
optional KyYard container link, and a `silenced_until` time.

- A bounded worker pool polls due targets, so a slow app cannot delay others.
- KyYard suggestions only prefill the add form. Nothing is polled until an admin saves it.
- Stored per target: the last normalised result, plus a list of state transitions (time,
  from, to, reason). Individual polls are not stored.

### State machine

States are `ok`, `degraded` and `down`. `basic` counts as `ok` for alerting.

| Transition | Condition |
|---|---|
| to `down` | 3 consecutive down polls |
| to `degraded` | 2 consecutive degraded polls |
| to `ok` | 2 consecutive ok polls |

- Every transition creates an alert and one webhook message.
- While a target stays `down` or `degraded`, a reminder goes out every hour.
- Silence (1 h, 8 h or until fixed) suppresses webhook messages only. The alert bar and the
  Alerts tab still show the problem. "Until fixed" ends on the next transition to `ok`.
- Transitions come from pure functions tested over poll sequences.

### Webhook

- **Configured in the UI by admins.**
  - The URL and any token are sealed with the server encryption key.
  - The token is write-only in the UI: it is never displayed after it is saved.
- **Presets:**
  - ntfy: text body, with title and priority headers.
  - Gotify: the token goes in the `X-Gotify-Key` header, never in the URL.
  - Discord: a JSON `content` body.
  - Generic: `{"app","state","previous","reason","time","url"}`.
- **Message content:** app name, transition, reason code, time and a link to the kyPulse app
  page. Messages never contain log lines, user names or IPs, because hosted ntfy and Discord
  are third parties.
- **Delivery:**
  - A "Send test" button sends a sample message.
  - Each message is retried 3 times with exponential backoff.
  - Continued failure raises "alerts not being delivered" in the alert bar.
  - Every send and failure is audited.

## 3. KyYard integration

### KyYard changes (KyYard-Server repo)

- **Service tokens.**
  - An organization admin opens Settings, Service tokens, Pair kyPulse, which generates a
    6-digit code: 15-minute TTL, single use, bound to that organization.
  - `POST /api/service-tokens/claim` takes `{pairing_code, service_name}` and returns
    `{token, organization}`.
  - The claim route is unauthenticated, so it is rate-limited to 5 attempts per minute per IP
    and 30 per minute globally. That keeps guessing a 6-digit code within its TTL impractical.
  - The token is 256-bit random. KyYard stores only its SHA-256 hash.
  - Tokens are listed and revocable in the same screen.
- **`pulse_reader` role.**

  | Allowed | Refused |
  |---|---|
  | Read organizations, environments, endpoints, inventory and samples | Every mutating route |
  | Container logs, without `follow` | Exec, shells, deploys, settings, users, tokens |
  | The organization audit feed | Other organizations |

  Refused routes return 403.
- **Audit volume.**
  - Service-token reads do not write an audit row per request. The token records
    `last_used_at` and the last IP, and KyYard writes one audit row per token per hour
    summarising reads.
  - Claim, revoke and every refused request are audited individually.
- **Health.** Add `/healthz` on the contract (section 1).

### kyPulse side

- **Pairing.**
  - An admin enters the KyYard URL and pairing code. kyPulse claims with
    `service_name: "kypulse"` and seals the token.
  - The token is never logged.
  - Pairing and unpairing are audited.
- **Unpair** (admin, audited) deletes the URL and token in kyPulse. The KyYard revoke is a
  separate step by a KyYard admin, and each UI names the other side's step.
- **Pulls:**
  - Every 60 s: endpoints and inventory, giving container state, exit code, image tag,
    restart count, Docker health and memory against its limit.
  - Logs only for containers linked to a watched app. Each pull asks for lines `since` the
    last seen timestamp, capped at 1,000 lines per container per pull.
  - Audit feed rows since the last seen ID go into activity.
  - Resource samples are fetched on demand for the app detail page and not stored.
- **Degradation.** If KyYard is unreachable, KyYard-derived data is shown as stale with its
  age, and health polling continues independently.

## 4. Logs, senders and activity

### Ingest endpoint

`POST /api/ingest/logs` takes a `Bearer` source token, with `Content-Type:
application/x-ndjson`.

- **Limits.** At most 1 MiB per request and 16 KiB per line. Longer lines are truncated and
  marked. Each token may send 60 requests per minute; beyond that the endpoint returns 429
  with `Retry-After`.
- **Token scope.** A source token authorises this route only. It cannot read anything.
- **Source attribution.** The source name comes from the token. Any source field in the body
  is ignored.
- **Response.** A 2xx means the whole batch is stored, and the sender advances its offset only
  then.

### Source pairing

1. An admin clicks Logs, Add source, which shows a 6-digit code: 15-minute TTL, single use.
2. The operator runs `kypulse-send pair --url <kypulse> --code <code> --name <source>`.
3. The claim returns a token, which the sender writes to its state file with owner-only
   permissions (`keyfile`).

The claim route has the same rate limits as KyYard's: 5 attempts per minute per IP and 30 per
minute globally.

Sources are listed and revocable in the UI. Pairing, revocation and refused requests are
audited.

### `kypulse-send`

A single static binary with three input modes:

- `docker --container a,b`: reads container stderr and stdout through the Docker API. The docs
  state that socket access is root-equivalent, and recommend a read-only socket proxy.
- `file --path <p>`: follows a file across rotation and truncation.
- `stdin`, for example `app 2>&1 | kypulse-send stdin`.

Behaviour:

- **Batching.** A batch is sent every 2 s or at 500 lines, whichever comes first.
- **Resume.** After each acknowledged batch, the sender persists its position: a file offset
  and inode, or the Docker timestamp. On restart it resumes from there. Delivery is
  at-least-once, so a crash may re-send a batch but never skips one.
- **Outage buffer.** While kyPulse is unreachable, up to 16 MiB of lines are held in memory.
  Past that the oldest lines are dropped, and a `dropped N lines` marker line is sent when
  delivery resumes.

### Storage and retention

- **Log lines.** Lines from KyYard and from ingest share one table: time, received time,
  source, app, level, event, message, and the raw line capped at 16 KiB.
- **Parsing.**
  - JSON lines in the ky-primitives `logging` shape are parsed into fields.
  - Other JSON is kept raw with a best-effort level.
  - Non-JSON lines are stored as the message.
- **Retention.** An hourly job deletes rows older than 7 days, then deletes oldest-first until
  total stored size is under `KYPULSE_LOG_MAX_BYTES`, which defaults to 1 GiB. Activity uses
  the same retention.
- **Display.** Stored text is untrusted. The UI renders it as plain text, with control
  characters and ANSI escapes stripped.

### Activity

- **Sources.** Audit events are extracted into an activity table: time, app, actor, action,
  target, outcome and IP. They come from ky-primitives `logging` audit lines (from KyYard or
  ingest) and from KyYard's audit feed.
- **Chain verification.** kyPulse does not verify hash chains, because that needs each app's
  chain key. `kyauditverify` remains the tool for that.
- **Access.** Logs and Activity are admin-only, because they can contain personal data.

## Error handling

- **Health polls.** Each failure records its specific cause: timeout, TLS, DNS, refused,
  status code or body too large. The detail page shows it.
- **KyYard failures** mark only KyYard-derived data stale.
- **Ingest.** It returns 400 for a malformed batch (nothing stored), 401 for an unknown or
  revoked token, 413 over the size cap and 429 over the rate limit.
- **Webhook failures** are retried, then surfaced in the alert bar and audited.
- **Background jobs.** Every job is idempotent and resumes from stored positions (last seen
  timestamps and IDs, sender offsets). A crash mid-run repeats work at most once and never
  corrupts state.

## Testing

- **`ky-primitives/health`.**
  - Error text never appears in the response.
  - Reason codes are validated.
  - Status is the worst check status.
  - Deadlines are enforced.
  - The HTTP status maps correctly.
- **Normaliser.** Table tests over saved real responses from every current app.
- **State machine.** Tests over poll sequences cover every transition, the reminder cadence,
  and silence behaviour (still visible, not sent, ends on recovery).
- **Egress guard.** Loopback, link-local and metadata addresses, DNS rebind at connect time,
  redirects, the 64 KiB cap, the timeout and the HTTP rules.
- **Webhook.** Each preset's request shape, retries and failure surfacing, tested against a
  fake server.
- **Ingest.** Size, line and rate limits; a source token calling a read route; a forged source
  in the body; partial-batch rejection.
- **`kypulse-send`.** Resume after restart, file rotation and truncation, the buffer cap and
  drop marker, and pairing file permissions.
- **KyYard.**
  - kyPulse side: tested against a fake KyYard.
  - KyYard side: `pulse_reader` is refused on every mutating route, is refused on other
    organizations, is refused `follow`, and the hourly audit summary is written.
- **Retention.** Age and size deletion, oldest first.
- **Browser (Playwright).** Every tab, the alert bar in red, green and delivery-failing
  states, the app detail page, and a viewer denied Activity and Logs.
- **CI.** The existing CI workflow gates all of the above.

## Build order

Each step gets its own implementation plan and PR.

1. **`ky-primitives/health`.** The contract, handler and tests, in a new ky-primitives
   release.
2. **kyPulse core.**
   - Rename and scaffold cleanup.
   - Roles, egress guard, watched apps, poller, state machine and webhook.
   - The Status and Alerts tabs, the alert bar, the app detail page without KyYard or log
     sections, and kyPulse's own `/healthz`.
   - Useful against today's apps through the fallback rules.
3. **KyYard.** Service tokens and `pulse_reader` in KyYard-Server, then pairing, inventory,
   log and audit pulls in kyPulse, and the KyYard sections of the app detail page.
4. **Logs.** The ingest endpoint, source pairing, `kypulse-send`, log storage and retention,
   and the Logs and Activity tabs.
5. **App adoption.** One small PR per app moving to `/healthz` on the contract. This can run
   alongside steps 2 to 4.

## Out of scope

- kyPulse monitoring itself or sending heartbeats. Operators watch kyPulse's `/healthz` from
  outside.
- Alerts on container restarts or security events. They appear on screen only.
- Email, KyPost push or other alert channels beyond the webhook.
- Journald input for `kypulse-send`.
- Hash-chain verification of ingested audit lines.
- Long-term log archive, full-text indexing, metrics history and dashboards beyond the four
  tabs.
- Any action on watched apps: restart, repair, deploy.
