# Visual refinement evidence: kyPulse (2026-10-26)

Before/after captures and real-browser checks for the fleet visual-refinement batch
(baseline review gap 4: browser-default blue/violet back link, uppercase labels and a box
per setting, a filled Delete as the strongest control with no page primary). Ratings
belong to the independent reviewer; this record claims no score.

## Stages

| Stage | Source | What it shows |
| --- | --- | --- |
| `00-before` | clean `8aefea2` (origin/master) | baseline; 2/6 checks |
| `01-after` | clean `535a91a` | the change; 6/6 checks. No visual defect found, so no confirmation recapture |

One app page (the down "Backup agent") in three variants: `app-detail` (Busnes, no saved
choice) and the fleet review F7 variants `app-delete-sky` and `app-delete-forest` (saved
Sky and Forest), each light and dark OS scheme × 1280×900 and 390×844, full page. Saved
palettes override the OS scheme: the light/dark pairs differ only inside the live
duration text (pixel diff boxes 406,14–1164,206 desktop and 12,159–378,451 phone).
Manifests record source commit, uncommitted-source hash (null), OS, Playwright/Chromium,
font state, server, fixture, time, PNG SHA-256s and checks.

Fixture: `.browser/server` built from the tree (embedding the committed `web/dist`) on
127.0.0.1:5391 with temporary SQLite; admin account; three watched apps served by a
capture-owned fake health endpoint on a private bridge address (Billing ok, Mail relay
degraded, Backup agent down). Durations and timestamps are live.

## Changes

- The "← Status" link uses page ink with an underline instead of the browser's default
  blue/violet (`.dr-hint a`).
- `.dr-facts` are label/value rows on the page surface divided by hairlines, labels in
  13px sentence case; no box per field, no uppercase letter-spaced labels. This style is
  shared with the KyYard facts on app pages and the KyYard card in Settings (not captured).
- Edit stays in the page header as the only header action. Delete moves to a closing
  "Stop watching" section ("Its alert history is deleted too."), still the filled
  `.btn-danger` with the same `window.confirm` and admin-only rendering. No primary was added.
- Colours, the F1 status tint and the F7 suite-palette label rule are unchanged.

## Checks

Per stage in `behaviour-checks.csv`: the Status link colour under Sky, Forest and Busnes,
no filled destructive control in the page header (Delete keeps `btn-danger`), labels not
uppercase, and no console or CSP errors. `browser/danger-button.spec.mjs` (F7 pixel
contrast on the real Delete in all 15 suite palettes plus the legacy `--button-text`
assertions) ran against the new position; results are in the pull request.

## Known limits

- The Alert history table headings (`.events th`) are still uppercase and letter-spaced;
  the style is shared with the Alerts page, which this batch did not capture.
- The Status/Settings page introductions keep their 12px title gap (Base moved to 8px).

## Reproduce

From the repository root, after `cd web && npm ci && npm run build` and
`go build -o .browser/server ./cmd/server`:
`node docs/ui-visual-refinement-2026-10-26/capture.mjs 01 after`, then `results.mjs`.
