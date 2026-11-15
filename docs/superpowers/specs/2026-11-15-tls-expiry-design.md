# TLS certificate expiry checks

Date: 2026-11-15. Status: owner-approved feature (2026-11-15); design by worker f-pulse.

## Goal

Warn before a watched certificate expires, and fail loudly when it is expired, names the
wrong host or does not chain to a trusted root. Cover every `https://` health target and
standalone TLS endpoints KyPulse does not otherwise poll, such as the edge proxy.

## Threat model

- **The watched endpoint is untrusted input.** Its certificate's issuer and names are
  attacker-chosen text: they are capped (issuer 200 bytes), rendered as text only, never
  logged and never put in a webhook. The webhook carries the app name, a level and a fixed
  cause code, as today.
- **An administrator's TLS target is an outbound request built from user input** (SSRF).
  It dials through the same `internal/egress` address policy as health polls: every resolved
  address is checked at dial time, so a DNS rebind cannot pass. Loopback and link-local are
  refused unless the administrator ticks "Allow a local address" on that target. Even then
  the cloud metadata addresses (`169.254.169.254`, `169.254.170.2`, `fd00:ec2::254`,
  `100.100.100.200`), unspecified, multicast and reserved ranges stay refused. A TLS probe
  sends no application bytes: it completes a handshake and closes, so it cannot be used to
  talk to a plain-text service behind the guard.
- **No second connection.** For an `https://` app the certificate is read from the health
  poll's own handshake (`http.Response.TLS`, or the unverified chain inside
  `tls.CertificateVerificationError` when verification fails). Nothing extra is dialled.
- **Viewers** may read certificate facts (they already see app detail); only administrators
  add TLS targets or change thresholds. Both writes are audited.

## Data stored

Migration 15 adds three columns to `targets`:

| Column | Meaning |
|---|---|
| `sni` | Optional server name for a TLS target; empty means the host |
| `allow_local` | Administrator's explicit loopback/link-local permission (TLS targets only) |
| `cert_track` | The certificate's `alerts.Track` JSON, owned by `internal/monitor` |

The last observed certificate (`not_after`, `issuer`, `name_match`, and `problem` when the
handshake refused it) rides in the existing `last_result` JSON as `cert`. A standalone TLS
target is a normal target row whose URL is `tls://host:port`; it shares create/edit/delete,
silence, Alerts and the detail page. One server setting, `cert_thresholds`
(`"<warn>,<critical>"`, default 21 and 7 days), holds both thresholds, so a write is atomic.
Certificate transitions are `target_events` rows whose states are `cert_ok|cert_warning|cert_critical|cert_failed`, so the uptime measure, which
reads only health states, ignores them.

## Levels and the state machine

Each poll that produced a certificate classifies it, worst first:

| Condition | Level | Cause |
|---|---|---|
| `now ≥ NotAfter` | failed | `cert_expired` (`cert_not_yet_valid` before `NotBefore`) |
| leaf does not cover the host (or SNI) | failed | `cert_name_mismatch` |
| chain does not verify | failed | `cert_untrusted` |
| days left ≤ critical | critical | `cert_expiring` |
| days left ≤ warning | warning | `cert_expiring` |
| otherwise | ok | |

The certificate track uses `alerts.Decide` unchanged, so reminders are hourly and the
target's timed or until-fixed silence applies. A level change needs two consecutive polls
(one from a fresh target), like health recovery, so one odd answer during a proxy reload
does not alert. A recovery to ok ends an until-fixed silence on the target, exactly as a
health recovery does. A handshake that produced no certificate (refused, timeout) leaves
the certificate track as it was; the health track reports the outage.

A TLS target's health is reachability: ok once a handshake returned a certificate, down
otherwise. Its certificate problems are the certificate track's alone, so one problem is
one alert. An `https://` app whose certificate fails verification is also down on health,
because KyPulse cannot read its health through it; that is two alerts with two meanings.

## Routes

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/api/certificates` | session | every target with a certificate, soonest `not_after` first, plus the thresholds |
| PUT | `/api/certificates/thresholds` | admin | `{warn_days, critical_days}`; 1 ≤ critical < warn ≤ 365; audited `admin.cert_thresholds` |
| POST/PUT | `/api/targets`, `/api/targets/{id}` | admin | accept `tls://host:port`, `sni`, `allow_local` |

## UI

- App detail: a Certificate fact row with days left, expiry date, issuer and the level badge.
- One Certificates page in the left navigation, sorted by soonest expiry, with the thresholds
  form for administrators.
- The add/edit dialog offers "Health URL" or "TLS only" (`host:port`, optional SNI, "Allow a
  local address").

## Not built

- No OCSP/CRL revocation checks, no chain-of-intermediates display, no CT log lookups.
- No per-target thresholds: one pair for the deployment is what was asked for.
- No STARTTLS (SMTP/IMAP) probing; a TLS target is direct TLS only.
- No certificate checks for plain `http://` targets.
- No separate certificate silence: the target's silence covers both of its tracks.
- Certificate levels do not change an app's health state, the Status counts or the alert
  bar; they show on Certificates, app detail and Alerts, and go out as webhook messages.
