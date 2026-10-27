# Opus round12 review — KyPulse stage12-after

Claude Opus 5.5 (`claude-opus-5-5`), Claude Code harness, no subagents or Gemini, peer round12 not opened. Rubric ky-suite-v2 `ad351662…2c41` and contract `392afb88…44d3`, both from ky-ui `639e5882`. Runtime `68bf09f8d3c417bc578665ec9affb7b0405ce05a`, evidence `63df86c56e963fb9c74dbad581036000b5ad31d8`. JSON: [opus-round12.json](opus-round12.json).

## Verdict

**Not satisfied.** Admin-console 223.6/28 = 7.9857 (<8). Settings 96.8/12 = 8.0667 (≥8). I did not pool the cohorts or round the admin mean up to 8.

R1 and R2 did what they claimed, in the captures and natively. The empty-state guidance now sits under the empty message on all 8 collapsed Logs/Activity views, and Log sources uses 8px/16px spacing. Those removed every R11 reason I had for scoring admin below 8.

The admin shortfall now comes from one call. I gave typography 7 on the four desktop Logs captures (collapsed and expanded, light and dark) because the 13px Log sources prose runs 976px wide with no max-width. That width has not changed since R11, where I scored it 8. So this is a criterion shift on unchanged pixels, the same pattern my round8 calibration advisory warned about. I disclose it rather than revise it, because the frozen judgment was not a misreading and the measure is real. If those four files were typography 8, admin would be 224.0/28 = 8.0 exactly.

## Inputs, freeze and blinding

- 40/40 originals: sha256, bytes and IHDR dimensions match the manifest, inventory and git blobs at 63df86c. The disk set equals the manifest set. Overflow is 0. Cohorts: 28 admin-console and 12 settings. My own Pillow pass found no exact or near duplicates; the closest pair differs in 1.878% of pixels.
- All 21 source pins match the 68bf09f blobs and the worktree bytes. `uncommitted_source_sha256` is null. `check-vendor.mjs` reports ky-ui 0.4.3 verified. 9 vendored files byte-match producer 58626b7 objects; 3 generated files are verified only by their VERSION pins. The contract changes from 639e588 to 58626b7 only by adding themes-browser.js.
- The four KyNotes brand references (kynotes-server 1ab04e64) match their hashes and dimensions. I viewed them before brand scoring and used them for brand only.
- I viewed all 40 originals one at a time at original resolution before opening any prior per-file report. I made no contact sheet.
- Priming: the parent brief (aggregates, R1/R2 intent, taste list) and the COUNCIL.md/README cohort table, read during orientation. I also read source CSS before freezing to check two type facts. The "Down" banner heading is Space Grotesk bold, not monospace (confirmed by a crop), and the Logs intro is `.dr-hint`.
- Freeze: 2026-10-27T09:08:53Z. Payload sha256 `300498f81f1a7e7e9dda7c6141c21342962f74e8e4af1a38bb5b9f2a054877dc` covers the keys listed in `freeze.hashed_keys`. An earlier generation at 09:08:39Z (`6e20b9a7…`) had a surface-key bug: the 8 expanded-filter rows had no observations and fell back to default scores. I regenerated before opening any history and disclose it here and in the JSON.

## Scores

| File | Cohort | H | T | D | C | A | Co | Total | R11 | Δ | Ky fit |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| activity-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.6 |
| activity-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.6 |
| activity-filters-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| activity-filters-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| activity-filters-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| activity-filters-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| activity-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.4 |
| activity-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.4 |
| alerts-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| alerts-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| alerts-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| alerts-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| app-detail-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| app-detail-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| app-detail-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.2 |
| app-detail-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.2 |
| logs-desktop-dark | admin-console | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 7.8 | +0.1 | 8.4 |
| logs-desktop-light | admin-console | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 7.8 | +0.1 | 8.4 |
| logs-filters-desktop-dark | admin-console | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 8.0 | -0.1 | 8.4 |
| logs-filters-desktop-light | admin-console | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 8.0 | -0.1 | 8.4 |
| logs-filters-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.2 |
| logs-filters-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.2 |
| logs-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.6 | +0.4 | 8.2 |
| logs-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.6 | +0.4 | 8.2 |
| status-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| status-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| status-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| status-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| appearance-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 9 | 8.2 | 8.2 | +0.0 | 8.8 |
| appearance-desktop-light | settings | 8 | 8 | 8 | 8 | 9 | 9 | 8.3 | 8.2 | +0.1 | 8.8 |
| appearance-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 9 | 8.2 | 8.2 | +0.0 | 8.6 |
| appearance-mobile-light | settings | 8 | 8 | 8 | 8 | 9 | 9 | 8.3 | 8.2 | +0.1 | 8.6 |
| backup-desktop-dark | settings | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 8.0 | -0.1 | 8.6 |
| backup-desktop-light | settings | 8 | 7 | 8 | 8 | 8 | 8 | 7.9 | 8.0 | -0.1 | 8.6 |
| backup-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| backup-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.4 |
| settings-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.9 | +0.1 | 8.6 |
| settings-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.9 | +0.1 | 8.6 |
| settings-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.9 | +0.1 | 8.4 |
| settings-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.9 | +0.1 | 8.4 |

Exact cohort arithmetic uses the rounded unique totals. Admin is 24 × 8.0 + 4 × 7.9 = 223.6/28 = 7.985714… Settings is 2 × 8.3 + 2 × 8.2 + 6 × 8.0 + 2 × 7.9 = 96.8/12 = 8.066666… Ky fit is reported separately and never added to a total (five equal weights, mean 8.46 over 40 files). The JSON holds per-dimension observations, brand observations, confidence (high for all 40) and caveats.

## Drift against my R11 (same surface, theme, viewport and disclosure)

Max |Δ| is 0.4, so no file reaches 0.5.

**Admin, 222.0 → 223.6:**
- Product effect, +2.0 weighted:
  - R2 raised hierarchy 7→8 on all 8 collapsed Logs/Activity files.
  - R1 raised density 7→8 on phone Logs light/dark.
- Reviewer variation, −0.6: typography 8→7 on four desktop Logs files, a measure that did not change.
- Rounding: 7.85 rounds to 7.9, which adds +0.2.

**Settings, 96.4 → 96.8:** all reviewer variation on retained pixels, with no product change.
- Settings coherence 7→8 on 4 files: I now read the first-level cards as one vocabulary.
- Appearance light affordances 8→9 on 2 files.
- Backup desktop typography 8→7 on 2 files: the same 976px, 13px measure.

None of the settings changes is a product gain or a regression.

## Native supplement (after freeze)

I used my own Incognito tab, opened with reuseExistingTab:false and closed afterwards. I did not touch the parent tab. Browser: Chrome 154 at DPR 2, while the scored originals are Chromium 153 at DPR 1. OS light only. No writes: I only changed routes, resized the viewport and opened the Activity Filters disclosure.

- **Logs at 1280×900:**
  - The Log sources h2 is 17px with 8px to the first paragraph, and the paragraphs are 16px apart.
  - The paragraphs are 13px Space Grotesk, 976px wide, with `max-width: none`. A 130-character paragraph fits on one line.
  - The page intro is also 13px and 976px wide. `.status-intro` is `max-width: 65ch; font-size: 14px`.
  - The guidance directly follows `.log-empty`, 12px below it. One Primary nav, overflow 0.
- **Backup at 1280×900:** the "One sealed capsule…" paragraph is 13px, 976px wide, with no max-width; 239 characters wrap to 2 lines.
- **Activity at 390×844, expanded:**
  - The Filters summary is 40px.
  - The empty message spans 728.5–773.5 and the guidance 785.5–824.5, so the group ends inside 844 at pageY 0. Overflow 0.
  - This matches the parent's native-round12.json.
- **Logs at 390×844, collapsed:** heading gap 8px, paragraph gap 16px, paragraph width 358px, guidance 12px under the empty message.

Post-freeze factual note: my Status observation says "15px intro", but the computed intro is 14px. That was a wording error and no score depended on it. The 12px gap between the empty message and the guidance is off the 8px grid, but the two still read as one group; I did not change any score for it.

## Findings

Required, concrete (it is the only item behind my sub-8 admin mean):
- **R12-1:** On the four desktop Logs captures, the Log sources prose and the page intro run 976px wide at 13px (about 130–150 characters per line). The Status intro is already capped at 65ch.
  - Fix: give `.log-sources > .dr-hint` and the Logs/Activity `.dr-page > .dr-hint` intros a readable max-width such as 65–72ch. Change no copy, order, handler or subscription.
  - Done when: at 1280, those paragraphs are at most about 75 characters per line.
  - Caveat: I raised this criterion only in R12, on unchanged pixels. The parent decides whether it justifies a change.

Optional or taste, not required:
- O12-1: The Backup desktop "Back up now" paragraph has the same measure. The same max-width rule would cover it, and settings already passes.
- O12-2: The Logs/Activity intro is hint-size (13px), while the Status intro is 14px and the Settings/Appearance intros are body-size.
- O12-3: The app-detail `.banner-down` and `.banner-ok` use hard-coded rgba tints rather than ky-ui status tokens. This affects the brand score only.
- O12-4: Unselected radios on dark Appearance are quiet dark discs.
- I am not reopening any earlier taste item: Status hierarchy, the ragged theme row, full-width dates, 8px phone filter gaps, natural Backup heights.

## Gates and checks

All behaviour gates (keyboard, focus, contrast, states, responsive) are **untested**. Screenshots and my scoped native geometry cannot pass them.

The parent supplied these results; I did not execute them:
- At source 68bf09f, build, typecheck, vendor and make ci passed.
- The new full browser suite at /tmp/kypulse-r12-browser.log has no exit record yet. I did not read or rerun it.
- The R11 browser results are historical relative to this source.
- Remote CI and exact-head security review are pending.

I make no claim of full-product 8/9, consensus, CI, security or deployment. I posted nothing to MySlop; it returns 401.

## Limits

- Initial viewport only, with a synthetic admin.
- Logs/Activity are empty and KyYard is unpaired; Backup is unconfigured.
- The viewer role and the populated, loading and error states are not covered.
- The native check was Chrome 154 at DPR 2, OS light only.
