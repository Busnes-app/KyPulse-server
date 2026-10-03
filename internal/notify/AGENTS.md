# Notify

## Purpose
Renders one alert message for one webhook preset (ntfy, Gotify, Discord, generic JSON) and delivers it with retries.

## Ownership
Owns `Config`, `Message`, `Build`, `Notifier`, `Reason`. Knows nothing about why a message is sent.

## Local Contracts
- Tokens ride in headers (`Authorization: Bearer`, `X-Gotify-Key`), never in the URL or body.
- `Validate` requires a nonblank Gotify app token. ntfy and generic tokens remain optional; Discord authenticates through its webhook URL.
- Messages carry app name, state, previous state, reason code, time and a kyPulse link — never log lines, user names or IPs.
- Retries on transport errors, 429 and 5xx (`ReceiverError` once exhausted); a 4xx is a `RejectedError` (matches `ErrRejected`) and is not retried. Four attempts over `DefaultBackoff`.
- A send error's text can carry the webhook URL, which for Discord and ntfy is the credential. Only `Reason(err)` is stored, audited or returned: `egress.Cause` codes, `rejected_<code>`, `receiver_<code>`, `cancelled`, `unreadable` (`ErrUnreadable`), else `network`.

## Verification
- `go test ./internal/notify/`

## Child DOX Index
None.
