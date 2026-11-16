# Email alerts through KyPost

Date: 2026-11-15. Status: owner-approved feature (2026-11-15); design by worker f-pulse.
Wire contract: `docs/KYPOST.md` (byte-identical with KyPost `docs/KYPULSE.md`).

## Threat model

- **KyPulse must not become an open mail relay.** KyPost, the resource owner, decides
  everything: the owner's switch (off by default), the recipient list, confirmation of extra
  recipients, the per-mailbox rate limit, the fixed template. KyPulse names a member and
  recipients; a compromised KyPulse can at most mail confirmed recipients of members who
  opted in, at 30 an hour, under a `[KyPulse]` subject.
- **Forged requests.** Signed with `syncauth` v1 under the KyIdentity pair secret; event type
  checked first; replay guard. The secret lives in memory only.
- **Personal data.** Recipient addresses are sealed at rest (`alert_kypost_enc`, the
  webhook's sealer pattern with its own label), never returned to viewers, never logged or
  audited (audits carry the owner ID and a count). Only KyPost's reason code is stored and
  shown.
- **Egress.** KyPulse reaches KyPost through `internal/egress` (HTTPS only, no redirects,
  loopback/link-local/metadata refused, private LAN allowed as for all KyPulse egress).
- **Same host.** If KyPost shares a host with the failing app, the alert dies with it; the UI
  says so and keeps the off-host webhook as the recommended path.

## Data

| Where | What |
|---|---|
| KyPulse `alert_kypost_enc` | sealed `{owner_id, recipients[]}` |
| KyPulse `alert_kypost_status` | last outcome `{at, ok, error code}` |
| KyPost per-user settings | switch, extra recipients with confirmation state |

## Routes (KyPulse)

| Method | Path | Auth |
|---|---|---|
| GET | `/api/alerts/kypost` | admin: availability, pairing, eligible owners, config, last outcome |
| PUT | `/api/alerts/kypost` | admin: `{owner_id, recipients}`; audited `admin.kypost_mail_set` |
| DELETE | `/api/alerts/kypost` | admin; audited |
| POST | `/api/alerts/kypost/test` | admin: one attempt; 502 `{error: code}` on refusal |

The KyPost route `POST /api/suite/kypulse/alert` and its settings routes are in the contract.

## Defaults and choices

- Pairing through KyIdentity only (no environment pairing): the owner is named by
  KyIdentity subject and issuer, so the channel cannot work otherwise. Without KyIdentity
  sign-in the channel is hidden behind a note.
- The owner is picked from KyPulse accounts that sign in through KyIdentity (they have a
  subject KyPulse knows). Owner question 1 asks whether a suite-wide picker is wanted.
- Every alert the webhook would get also goes to KyPost; retries reuse the webhook backoff;
  the event ID is `kypulse-alert-<event id>` so a retry is a KyPost duplicate, not a second
  mail.

## Not built

- No HTML, attachments, Reply-To or per-alert recipient choice.
- No environment pairing, no KyPost→KyPulse callbacks.
- KyPost refusals do not raise the alert bar's "alerts not delivered"; they show beside the
  channel.
