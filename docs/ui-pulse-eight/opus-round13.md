# KyPulse visual8: Opus 5.5 round 13 (stage13-after)

Claude Opus5.5 (`claude-opus-5-5`), primary Council reviewer. Rubric `ky-suite-v2` (sha256 `ad351662…`), design contract `392afb88…`. Runtime `699d201`, evidence `a0a5558`. Machine record: `opus-round13.json`.

## Verdict

| Cohort | n | Sum | Fraction | Exact mean | At least 8 |
|---|---:|---:|---|---|---|
| admin-console | 28 | 224.0 | 224.0/28 = 8/1 | 8.0 | True |
| settings | 12 | 95.8 | 95.8/12 = 479/60 | 7.983333333333333333333333333 | False |

**Not satisfied.** The settings mean is 7.983, below 8; the displayed 8.0 does not clear it. Both settings shortfalls are the Backup desktop captures at 7.9. Admin-console is exactly 8.0. No pooling, no rounding into passage. Brand fit, kept separate: mean 8.01 over 40 files.

The 72ch change did what it claimed. Desktop supporting hints now wrap inside a 612px box (about 566px of rendered text), forms stay 976px, and phone hints stay 358px. No capture shows clipping, removed content or overflow.

## Findings

- **R13-1 (required, criterion shift):** Backup fact badges (None, Not paired, 0 of 7, Daily) render 12px/600 (.badge, theme.css:241), under the contract 13px meta/label size, beside 13px labels. Fix: Set .badge to the 13px label size, without changing badge text, colour or semantics. concrete and measured, but newly cited on pixels unchanged since R12 (criterion shift). Parent decides whether it warrants a change.
- O13-1 (optional): Short last lines: Log sources "destinations.", Status intro "controls.", Settings summary "Appearance." alone on line 2. text-wrap: pretty would balance them; taste.
- O13-2 (optional): Collapsed Logs has no filled action (Add source outlined). Calibration J1 advises against a second filled primary; I list it only as a scanning-emphasis note, not a required change.
- Not reopened (unselected taste): Settings first-level cards, the theme grid's last row, quiet native radios, natural Backup heights, phone filter gaps.

## Per-file scores

Columns: hierarchy, typography, density/layout, color, affordances, coherence; total; R12 total; delta; ky_fit. Confidence high for all 40.

| File | Cohort | H | T | D | C | A | Co | Total | R12 | Δ | ky_fit |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| activity-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| activity-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| activity-filters-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| activity-filters-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| activity-filters-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| activity-filters-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| activity-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| activity-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| alerts-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| alerts-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| alerts-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| alerts-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| app-detail-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| app-detail-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| app-detail-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| app-detail-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| logs-desktop-dark | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.9 | -0.1 | 8.0 |
| logs-desktop-light | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.9 | -0.1 | 8.0 |
| logs-filters-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.9 | 0.1 | 8.0 |
| logs-filters-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.9 | 0.1 | 8.0 |
| logs-filters-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| logs-filters-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| logs-mobile-dark | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.0 |
| logs-mobile-light | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.0 |
| status-desktop-dark | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.0 | 0.2 | 8.0 |
| status-desktop-light | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.0 | 0.2 | 8.0 |
| status-mobile-dark | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.0 | 0.2 | 8.0 |
| status-mobile-light | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.0 | 0.2 | 8.0 |
| appearance-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.2 |
| appearance-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.3 | -0.3 | 8.2 |
| appearance-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.2 |
| appearance-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.3 | -0.3 | 8.2 |
| backup-desktop-dark | settings | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 7.9 | 0.0 | 7.8 |
| backup-desktop-light | settings | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 7.9 | 0.0 | 7.8 |
| backup-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| backup-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| settings-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| settings-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| settings-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |
| settings-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 |

The JSON holds all six quality observations and five brand observations for every file, plus caveats.

## Drift against my R12 (same surface, theme, viewport and disclosure)

No file moved by 0.5 or more; the largest move is 0.3. Causes:

- **Status, 4 files, +0.2:** hierarchy 8→9. Reviewer variation; the Status pixels are retained.
- **Appearance, 4 files, −0.2/−0.3:** coherence, and on light also affordances, 9→8. Reviewer variation on the retained shared cards.
- **Logs desktop typography 7→8, 4 files:** an actual prose-wrap effect in the changed region. R12 deducted for the 976px measure, and the measure is now 72ch.
- **Logs collapsed hierarchy 8→7, desktop and phone:** reviewer variation on retained controls. Both R8 calibration advisories (J1) said a filled create action is not required. I keep the frozen number and list the point as optional only.
- **Backup desktop stays 7.9, for a different reason.** R12's typography 7 cited the 976px measure, which is now fixed. R13's 7 cites 12px badges that have not changed since R12. That is a criterion shift, not a regression.
- Sums: 223.6 -> 224.0 (+0.4): status +0.8 (hierarchy variation), logs-filters-desktop +0.2 (prose-wrap effect), logs-desktop -0.2 (prose-wrap +, hierarchy variation -), logs-mobile -0.4 (hierarchy variation); 96.8 -> 95.8 (-1.0): appearance -1.0 (variation); backup desktop unchanged at 7.9 with a changed reason.
- Brand: R12 brand mean 8.46 -> R13 8.0; mostly R13 holding palette/shell/accent at 8 where R12 gave 9 on retained pixels (reviewer variation). Logs type_geometry 7->8 on desktop follows the measure fix; on phone (358px both rounds) it is variation.

## Post-freeze notes

- Status observations say "Intro 15px"; R12 native measured .status-intro at 14px with max-width 65ch. Wording error only; no score depended on it.
- "No logs found." is also a .dr-hint at 15px, so its cap is 684px (72ch at 15px); it keeps its 15px size. Supporting 13px hints cap at 612px.
- Observations citing ~566px describe rendered line extent; the hint box is 612px (native).
- Backup "Back up now" paragraph now wraps to 3 lines, moving its action row lower so it meets the 900px fold. That is a prose-wrap effect, not a clipping defect, and no score cites it as one.

## Computer Use supplement (after freeze)

- Tab: own Incognito tab_2o_2e419d46-7b07-427f-bf30-1831131b4cba, opened reuseExistingTab:false, closed afterwards; parent tab tab_2f_2e419d46-7b07-427f-bf30-1831131b4cba untouched
- Browser: Chrome/154.0.0.0, DPR 2 (scored originals: Chromium 153, DPR 1); OS light, ky_theme null; fonts.check Space Grotesk true; service worker controller present
- Sign in: synthetic admin via snapshot refs; waited for "Security check verified" before Sign In, then role=navigation[name='Primary'] before routes
- Writes: none (route navigation, viewport resize, opening the Activity Filters disclosure, read-only DOM measurement)
- Agreement with native round13: Matches parent native-round13.json: desktop hints 612px / form 976px, phone 358px, expanded Activity guidance 824.5/844, one nav, zero overflow, OS light only.
- Limits: Scoped measurement in one theme and two viewports; not keyboard, focus, contrast or state acceptance.
- Logs 1280×900: hints 612px at 13px, empty status 684px at 15px, both forms 976px, one `Primary` nav, overflow 0.
- Activity 390×844 expanded: Apply/Clear end at 712.5, empty status at 773.5, guidance at 824.5 of 844; dates 358px; overflow 0; current tab fully visible.

## Provenance and limits

- 40 manifest shots = 40 inventory rows = 40 stage13 PNGs on disk = 40 blobs in a0a5558; sha256/bytes/PNG dimensions match; manifest overflow 0 for all; 40 distinct hashes; duplicate audit lists no near duplicates. Cohorts 28 admin-console / 12 settings.
- All 21 source pins match the blobs at 699d201 and the worktree. Vendor ky-ui 0.4.3 verified; tokens.css matches producer 58626b7. All 4 KyNotes brand references match their pinned hashes.
- Freeze: `1cca304eba854c0625e0d866f3d08e6b9027aad9904715ccc6fcace5559c1da8` at 2026-10-27T09:33:10+00:00, taken before any history, any audit, native-round13.json or Computer Use.
- All 40 originals and 4 KyNotes brand references viewed individually at original resolution before opening any prior per-file report, audit, calibration advisory or native-round13.json. Priming present: the parent brief (R12 aggregates, patch intent, prior findings, taste list) and the first rows of the COUNCIL.md score table (rounds 1-3 aggregates) glimpsed during orientation. Not fully blind.
- Peer round13 report not opened. Only this file was validated, with a standalone copy of the build-report.py per-report checks against inventory-v13.json; no peer reports were loaded.
- Gates are untested: keyboard, focus, contrast, states, responsive. These scores make no claim about full-product 8/9, peer consensus, CI, security or deployment.
- The parent supplied these checks; I did not run them: source 699d201 build/typecheck/vendor/make ci pass; the full browser run's exit record is pending; exact-head security is pending (last clearance b8f8c05); MySlop returns 401.
