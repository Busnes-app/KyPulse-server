# Notify

## Purpose
Renders one alert message for one webhook preset (ntfy, Gotify, Discord, generic JSON) and delivers it with retries.

## Ownership
Owns `Config`, `Message`, `Build`, `Notifier`. Knows nothing about why a message is sent.

## Local Contracts
- Tokens ride in headers (`Authorization: Bearer`, `X-Gotify-Key`), never in the URL or body.
- Messages carry app name, state, previous state, reason code, time and a kyPulse link — never log lines, user names or IPs.
- Retries on transport errors, 429 and 5xx; a 4xx is `ErrRejected` and is not retried. Four attempts over `DefaultBackoff`.

## Verification
- `go test ./internal/notify/`

## Child DOX Index
None.
