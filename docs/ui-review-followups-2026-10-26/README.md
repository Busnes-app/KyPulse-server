# Review follow-up R2 evidence: kyPulse (2026-10-26)

Finding R2 of the independent refinement runtime review (ky-ui
`reviews/2026-10-26-fleet/refinement-runtime-review.md`): the flat `.dr-facts` rows from
#50 put every fact label in a fixed 8rem column and dropped the label's flex row. Backup
renders `<div class="dr-fact-label"><span/><Badge/></div>`, so at 1280px its None, Not paired
and 0 of 7 badges wrapped under their labels while Schedule Daily stayed inline. This record
claims no score; the root reviewer confirms independently.

## Fix

`web/src/styles/theme.css`, one shared rule, all four consumers checked (app detail,
`KyYardFacts`, `KyYardCard`, Backup):

- `.dr-facts` is a grid whose label column is `minmax(8rem, max-content)`; each `.dr-fact`
  spans it as a `subgrid`, so values keep one left edge across rows.
- `.dr-fact-label` is a wrapping flex row again (gap 4px 8px), so a badge sits beside its label.
- Phones keep one column. No palette, font, status/danger colour, markup, behaviour, auth or
  CSP change. Plain-label consumers keep the 8rem column (app detail values stay at 914px).

## Stages

| Stage | Source | What it shows |
| --- | --- | --- |
| `00-before` | clean `c306af5` (merged origin/master, #50) | 6/7 checks: desktop Backup badges None, Not paired, 0 of 7 wrapped |
| `01-after` | clean `b538323` (the fix) | 7/7 checks; no defect found in the one inspection round, so no confirmation recapture |

`backup` and `app-detail` (a down synthetic app; the representative plain-label consumer),
OS light and dark × 1280×900 and 390×844, full page, device scale 1, service workers
blocked, Busnes (no saved choice). Manifests record the source commit, the uncommitted-source
hash (null for both), the `.browser/server` SHA-256, Playwright 1.63.0 / Chromium 153,
Space Grotesk load state, fixture and per-image SHA-256. `results.mjs` writes
`captures.csv` and `behaviour-checks.csv`.

Measured (light, from the manifests): desktop Backup label column 128 → 163px, values
416 → 451px, all four badges on their label's line; desktop app detail label column 128px
and values at 914px before and after; phones unchanged (one column, badges inline before and
after). Before/after app-detail pixel diffs are dominated by live timestamps, so the
geometry checks are the evidence for "unchanged" there, plus visual inspection.

KyYardFacts and the Settings KyYard card were not rendered: they need a paired KyYard. They
use plain text labels, the same case as app detail.

## Reviewer harness rerun

`reviewer-verify/pulse.mjs` is the reviewer's `harness/pulse.mjs` (SHA-256
`cf61e5cf…ac441`) with only its output directory, scratch directory and binary paths changed,
so the reviewer's own evidence is untouched. Base binary: built from `git archive` of
origin/master `c306af5` (SHA-256 `6293004676bc…ffdc`); head binary: this worktree's
`.browser/server` at `b538323` (`fecb1eaf7cd2…0da`). The capture driver's stage-00 binary
(`5c42e32a7083…`) is the same source built in the worktree path, so its bytes differ.

- base desktop: `badgeSameLineAsLabel` false for None, Not paired, 0 of 7; true for Daily (R2 reproduced)
- head desktop: true for all four; phones: true for all four at base and head
- head checks 5/5: admin header holds only Edit; Delete is the filled danger in Stop watching;
  its confirmation dismissed sends no DELETE (desktop and phone); a mocked non-admin sees no
  Edit, Stop watching or danger button

Crops: `reviewer-verify/pulse-{base,head}-backup-facts-{desktop,phone}.png`.

## Reproduce

From the repository root after `cd web && npm ci && npm run build` and
`go build -o .browser/server ./cmd/server`:
`node docs/ui-review-followups-2026-10-26/capture.mjs 01 after`, then `results.mjs`.
Synthetic data only, port 5391 (the driver refuses a busy port), removed afterwards.
