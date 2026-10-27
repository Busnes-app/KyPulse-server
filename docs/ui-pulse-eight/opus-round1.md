# KyPulse visual8: Opus 5.5, round 1 (baseline)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. I did not read `sol-round1.*`.
- Rubric: `ky-suite-v2` (`/home/yoshi/.codex-cpa/skills/ky-suite-ui/references/rubric.md`). Weights: hierarchy 0.20, typography 0.15, density_layout 0.20, color 0.15, visible_affordances 0.15, coherence 0.15. Each file total is rounded half-up to 1 decimal. Each cohort mean is taken over the unique rounded file totals.
- Machine record: `opus-round1.json` (schema `ky-ui-council-scores/2`). It holds all 32 files with sha256, six scores, the total, one observation per dimension, state, theme, confidence, caveat, `duplicate_of: null` and `ky_fit`.

## Verdict

| Cohort | n | Mean | Min | Max | Ky fit mean |
|---|---:|---:|---:|---:|---:|
| admin-console | 20 | **6.3** | 5.2 | 7.6 | 6.9 |
| settings | 12 | **7.0** | 6.2 | 8.0 | 7.2 |

**Not satisfied.** Neither covered cohort reaches 8.0. These are the actual scores, not projected targets. No pooled score or cross-cohort rank was computed.

Scope: visual quality of 32 initial-viewport captures at product source `fef7b67`, synthetic admin only. This round is not behavioural acceptance.

## Provenance checks (scripted)

- Worktree HEAD is `d3e823db6e6ace4f081f9eb3a089c27d8d6ad24d`. `git diff fef7b67 HEAD -- web/src web/dist` is 0 bytes. The only non-evidence change is the root `AGENTS.md` child-index line, which is docs only.
- All 32 PNGs have sha256, bytes and dimensions equal to both `inventory-v1.json` and `manifest-00-before.json`. All 32 hashes are unique.
- Every manifest `overflow` value is 0.
- Near-duplicate check: 0 pairs. The closest pair is activity vs logs, desktop light: 4.73% of pixels differ, max channel delta 224.
- The 9 manifest `source_files` pins match both `fef7b67` and the worktree. ky-ui `VERSION` is 0.4.1.
- The live fixture `GET /` hashes to `d45283df…`, equal to `served_index_sha256`.
- The KyNotes brand references (4 files at `1ab04e6`) match their pinned hashes. They are not quality inputs.

## Per-file totals (H, T, D, C, A, Co → total; ky_fit)

| Surface | desktop light | desktop dark | mobile light | mobile dark | Ky fit |
|---|---|---|---|---|---:|
| status | 8,8,8,7,7,7 → **7.6** | 8,8,8,6,7,7 → **7.4** | 8,8,7,7,7,7 → **7.4** | 8,8,7,6,7,7 → **7.2** | 7.8 |
| app-detail | 8,7,7,8,8,7 → **7.5** | → **7.5** | 7,7,6,7,8,7 → **7.0** | → **7.0** | 7.6 |
| alerts | 7,6,7,5,6,5 → **6.1** | → **6.1** | 6,6,4,5,5,5 → **5.2** | → **5.2** | 6.6 |
| activity | 6,6,6,6,6,5 → **5.9** | → **5.9** | 6,6,5,6,6,5 → **5.7** | → **5.7** | 6.4 |
| logs | 5,6,5,5,6,5 → **5.3** | → **5.3** | → **5.3** | → **5.3** | 6.2 |
| appearance | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,7,8 → **7.9** | 8,8,7,8,8,8 → **7.8** | 8,8,7,8,7,8 → **7.7** | 8.2 |
| backup | 8,7,8,6,6,7 → **7.1** | → **7.1** | 7,7,5,6,6,7 → **6.3** | → **6.3** | 7.2 |
| settings | 6,7,7,5,7,6 → **6.4** | → **6.4** | 6,7,6,5,7,6 → **6.2** | → **6.2** | 6.2 |

Surface means are within-cohort diagnostics, not rankings:

- admin-console: Status 7.4, App detail 7.3, Activity 5.8, Alerts 5.7, Logs 5.3.
- settings: Appearance 7.9, Backup 6.7, Settings 6.3.

Ky fit uses five dimensions: palette, type/geometry, shell, accent, cohesion. Suite cohesion is judged only against the captured KyNotes references, two products, not the fleet. The shell is consistent everywhere: one 240px left region on desktop and horizontal primary navigation on phone. Accent discipline is where it falls (Logs 4, Settings 4).

## Findings, severity-ranked

Each finding is labelled as a factual defect (F) or taste (T). Live values come from my own incognito tab against the fixture (native Chromium; the scored captures are Chromium 153/scale 1).

### High

1. **F, Logs/Activity: four or two equal filled-accent buttons, plus an off-scale title.**
   - Logs fills Apply, Clear, Add source and Refresh sources with the accent; the live computed fill is the same on all four. Activity fills both Apply and Clear. No single primary remains.
   - The h1 on both pages renders at the browser default 30px/700. Every other page uses 24px/600. Cause: `pages/Logs.tsx:21` and `pages/Activity.tsx:21` render a bare `<h1>` outside `.dr-header` (`styles/theme.css:304`).
   - The filter row orphans "To (UTC)" onto a second desktop row. On phones the inputs stay about 196px wide.
   - This finding drives the Logs 5.3 and Activity 5.8 surface means.
2. **F, Alerts: browser-default links and a second table vocabulary.**
   - App links render the user-agent colours rgb(0,0,238) in light and rgb(158,158,255) in dark (live computed). The Settings intro "Appearance" link has the same problem.
   - `.events th` is 12px uppercase with 0.04em tracking (`theme.css:506`). The design contract forbids that label style, and Status uses sentence case. App detail's Alert history uses the same `.events` table.
   - At 390px the 421px table scrolls inside a 356px panel with no cue, so the Delivery column reads as clipped. It is reachable, not lost. Change cells wrap to three lines.
3. **F, phone AlertBar consumes the first viewport.**
   - Two problems take 134px (live measured), and each full timestamp wraps.
   - On the app-detail page the global AlertBar and the page's Down banner state the same failure. Red status chrome ends at y=544 of 844 before any control.
   - Keep the AlertBar's meaning (every live problem, cause, duration, Alerts link) and the page banner's detail (exact request, last tried). Compact only the phone presentation of the AlertBar.

### Medium

4. **F, Backup: disabled primaries are faded accent.**
   - `button:disabled { opacity: .5 }` (`theme.css:197`) leaves "Back up now" and "Save" as pale orange fills. The label samples at about 2.2:1 from pixels.
   - The contract requires disabled to look distinct, not like a faded primary.
5. **F, Backup phone: the panel header collapses.**
   - "Back up now" squeezes to a 52×72px three-line heading beside the action stack (live measured).
   - `.panel-header` (`theme.css:164`) has no wrap.
6. **F, Degraded colour is hard-coded and fails on dark.**
   - `.state-degraded { color:#b45309 }` and `.dot-degraded { #f59e0b }` (`theme.css:491,493`) bypass `--warning`.
   - Degraded text on the dark panel measures 2.9:1. This is a sampled and computed value, not a contrast gate.
   - The AlertBar also paints the degraded item in danger red: `alert-bar-down` wraps all problems (`components/AlertBar.tsx:45`).
7. **T backed by contract, Settings: three filled primaries and decorative accent icons.**
   - Every panel heading icon is accent-coloured inline (`WebhookForm.tsx:82`, `KyYardCard.tsx:62`, `TwoFactorCard.tsx:81`).
   - Set up two-factor, Save webhook and Pair are all filled.
   - "Off" is bare bold text (`TwoFactorCard.tsx:83`, unstyled `badge`).

### Low

8. **F, duplicate navigation icons.** Status and Activity both use `Activity`; Logs and Backup both use `Archive` (`components/AppHeader.tsx:13-17`).
9. **T, App detail empty-note sizes differ.** "No retained logs for this app." is a 15px `<p>` (`LogTimeline.tsx:49`); the other empty notes are 13px hints.
10. **T, Backup badges.** Green success chips "0 of 7" and "Daily" sit under "No backups are being made". The tone reflects configured, not working. This is optional, and changing it changes meaning, so it needs owner intent.

Not findings: there is one left navigation region (desktop: one `nav` at x=16, w=207 inside the 240px header; phone: horizontal primary). There are no adjacent duplicate panels. Appearance already matches the KyNotes reference.

## Smallest coherent fix batch for 8 in both cohorts

These are product-scoped changes only, preserving routes, roles, CSP/CSRF, themes and the AlertBar/banner meanings:

1. Wrap the Logs/Activity titles in `.dr-header`. Make Clear, Refresh sources and Hide code `btn-secondary`, and Revoke a danger outline. Let filter fields share one row and go full-width on phones.
2. Add one product link rule: `a { color: var(--ink-strong); text-underline-offset: 3px }`, matching `.dr-hint a`. Change `.events th` to sentence-case 12px like `.status-list-heading`. Below 480px, stack `.events` rows as labelled blocks, as Status already does.
3. Phone AlertBar (720px and below): one line per problem with relative duration and cause, with the full timestamp kept in `title` or on desktop, and the Alerts link inline. Give degraded lines a warning tone rather than danger, without changing `role="alert"`.
4. Point `.state-degraded`, `.dot-degraded` and `.banner-degraded` at `--warning` / ky-ui tokens.
5. Replace the opacity disabled style with a neutral disabled treatment (surface fill, muted ink, no accent). Add `flex-wrap: wrap` to `.panel-header`.
6. Settings: ink-coloured heading icons, muted chip for "Off", and one filled primary per page view. Set up two-factor and Pair become secondary.
7. Give Activity and Backup distinct nav icons.

Items 1–3 are required for admin-console. Items 4–6 lift settings. I make no score projection; re-score after recapture.

## Drift (read only after blind scoring)

- HANDOFF.md records KyPulse at 5.6 before refinement and 6.9 for its latest scored views (2026-10-26).
- Those sets covered app-detail/backup full-page captures at other commits, with saved-palette variants and service workers blocked. No file hash overlaps this round.
- Per-file drift is therefore N/A, and differences are not controlled product change.
- The refinement record's fixed items are visible here: flat `.dr-facts`, the Delete section moved, and an ink back link.

## Gates and limitations

- keyboard, focus, contrast, states and responsive are all **untested**. Overflow 0 and visible nav at 390/1280 are not a gate pass.
- Coverage is admin only; the viewer role and existing role tests were not rerun.
- Captures are the initial viewport only.
- Logs/Activity are empty/unpaired. Settings/Backup are unconfigured synthetic defaults.
- Not covered: named-theme contrast, WebKit, soft keyboard, dense logs and multi-team data.
- Live timestamps differ between captures.
- Native tab actions: login, hash navigation, emulated light/dark and resize only. No writes, no theme saved. The tab was closed.
