# KyPulse visual8: Opus 5.5, round 8 (stage 08-after)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. No subagents, no Gemini.
- Rubric `ky-suite-v2`. Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15.
  - File totals use Decimal half-up rounding to one decimal.
  - Cohort means are exact means of the rounded totals. They are never rounded into 8.
  - Ky fit is reported only and never enters a total.
- Machine record: `opus-round8.json` (`ky-ui-council-scores/2`, round 8). It holds 40 rows, inventory, drift, the native supplement and the verdict. The build script asserted both freeze hashes, recomputed every total and Ky fit, and matched the file list to `inventory-v8.json`.
- Scope: the visual8 progression gate only. This is not full-product 8, not 9/10 Accept, and makes no consensus or deployment claim. Every behaviour gate is untested.

## Verdict: not satisfied in either cohort

| Cohort | n | Sum | Exact mean | Fraction | Min / max | Ky fit mean | ≥ 8.0 | Verdict |
|---|---:|---:|---:|---|---|---:|---|---|
| admin-console (Status, Alerts, app detail, Logs, Activity; collapsed and expanded filters) | 28 | 222.0 | **7.928571…** | 111/14 | 7.7 / 8.2 | 8.3 | no | **not satisfied** |
| settings (Settings, Appearance, Backup) | 12 | 95.0 | **7.916666…** | 95/12 | 7.6 / 8.2 | 8.3666… | no | **not satisfied** |

- There is no pooled, cross-reviewer or cross-cohort mean. There is no competitor, so the Ky gap is N/A. Gemini must wait.
- These are the frozen numbers, with no threshold repair.
- Distance to the threshold, for information only:
  - admin-console is 2.0 total points short (224 needed).
  - settings is 1.0 total point short (96 needed).

### Round-8 changes confirmed

- **E1 is resolved.**
  - The Backup notice border pixel is `#875b12` in light and `#e0b775` in dark, which equals `--ky-warning`.
  - The light fill is 14% warning over panel (`#eee8de`). The native computed border is `rgb(135,91,18)`, and accent is no longer used.
  - Colour on all four Backup files moved from 7 to 8.
- **The collapsed Filters summary now reads as a control.**
  - It is outlined, 40px tall with an 8px radius, and opens natively with Enter. Focus stays on it with a 2px accent ring at 2px offset.
  - All 8 collapsed files rise, but the Logs desktop hierarchy deduction J1 remains.
- **Phone filters:**
  - Rows are spaced at 8px and the dates are full width.
  - The result is now inside the fold: y≈779 in the original, and [16,780] native.
  - The round-7 complaint that the result was pushed toward or past the fold is gone, but density stays 7 for J2.

### Evidence-backed defects

- None at blocker level this round. Every remaining deduction below is a judgment call, with a visible observation behind it.

### Judgment deductions (visible, not blockers)

- **J1, admin-console, Logs desktop (2 collapsed and 2 expanded files): H7.**
  - Add source and Refresh sources are identical outlined buttons, so the create action does not stand out.
  - The native class is `btn-secondary` on both.
- **J2, admin-console, expanded phone filters (4 files): D7.**
  - Two half-width fields are followed by a lone full-width third field, so the grid reads 2+1+1+1.
  - The vertical rhythm itself is an even 8px. This deduction is borderline and close to taste.
- **J3, admin-console, app detail (4 files): Co7, plus D7.**
  - One view mixes three treatments: a filled danger banner, flat fact rows and a bordered rounded history table.
  - Checks puts its heading inline while its sibling sections stack theirs.
- **J4, settings, Backup:**
  - **Desktop: H7 and D7.** Seven buttons are visible, and the "Back up now" heading repeats its button label. The two columns end unevenly; this overlaps the owner's unequal-heights taste item, so I disclose it rather than unscore it.
  - **Phone: D7.** Section gaps are about 16px in one place and 48px in another.
- **J5, settings, Appearance dark (2 files): A7.**
  - Unselected radios are dark filled discs with faint rings. This is the shared ky-ui component.

### Taste, not scored down

- The persistent incident strip.
- Wrapped phone diagnostic chronology.
- Unequal Backup column heights beyond the J4 disclosure.
- Settings keeps section cards while Backup is flat.
- **Danger next to accent (Ky fit only, an upstream token matter):** danger `#b42318` sits very close in hue to accent `#bf3f18` in light. Danger `#ff8176` sits close to accent `#f5865f` in dark.
- **Glyph check:** "Down" is bold proportional Space Grotesk. The GET diagnostic line and the Health URL are fixed-width. I checked this on a 4× crop.

## Per-file scores (H,T,D,C,A,Co → total; Ky fit)

All 40 rows are high confidence with no artifacts. Light and dark score the same except Appearance dark, where A is 7.

| Surface | Desktop | Mobile |
|---|---|---|
| status | **9**,8,8,8,8,8 → **8.2**; 8.4 | **9**,8,8,8,8,8 → **8.2**; 8.2 |
| alerts | 8,8,8,8,8,8 → **8.0**; 8.4 | 8,8,8,8,8,8 → **8.0**; 8.2 |
| app-detail | 8,8,**7**,8,8,**7** → **7.7**; 8.4 | 8,8,**7**,8,8,**7** → **7.7**; 8.2 |
| logs (collapsed) | **7**,8,8,8,8,8 → **7.8**; 8.4 | 8,8,8,8,8,8 → **8.0**; 8.2 |
| logs-filters (expanded) | **7**,8,8,8,8,8 → **7.8**; 8.4 | 8,8,**7**,8,8,8 → **7.8**; 8.2 |
| activity (collapsed) | 8,8,8,8,8,8 → **8.0**; 8.4 | 8,8,8,8,8,8 → **8.0**; 8.2 |
| activity-filters (expanded) | 8,8,8,8,8,8 → **8.0**; 8.4 | 8,8,**7**,8,8,8 → **7.8**; 8.2 |
| settings | 8,8,8,8,8,8 → **8.0**; 8.4 | 8,8,8,8,8,8 → **8.0**; 8.2 |
| backup | **7**,8,**7**,8,8,8 → **7.6**; 8.4 | 8,8,**7**,8,8,8 → **7.8**; 8.2 |
| appearance light | 8,8,**9**,8,8,8 → **8.2**; 8.6 | 8,8,8,8,8,8 → **8.0**; 8.4 |
| appearance dark | 8,8,**9**,8,**7**,8 → **8.1**; 8.6 | 8,8,8,8,**7**,8 → **7.9**; 8.4 |

Ranking within each cohort follows this table. The JSON lists `rank_order` and holds one concrete observation per dimension per file.

**Ky fit:**

| Dimension | Score | Reason |
|---|---|---|
| palette_tokens | 9 | Sampled pixels equal the 0.4.3 tokens exactly. |
| type_geometry | 8 | Some panel padding is about 20px. |
| shell | 9 desktop, 8 phone | The phone layout shows no account name. |
| accent_discipline | 8 | Danger and accent hues sit close together. |
| suite_cohesion | 8, or 9 on Appearance | Compared against the 4 pinned KyNotes refs. Notes has a top-bar wordmark, while Pulse puts it in the sidebar. The Appearance cards are the same component. |

## Drift against my round 7, by matched state

| Group | n | Round 7 | Round 8 | Main cause |
|---|---:|---:|---:|---|
| admin collapsed Logs/Activity | 8 | 7.7 | 7.95 | Product: the outlined summary. These pixels changed only in the summary box. |
| admin expanded Logs/Activity | 8 | 7.9 | 7.85 | Variation: Logs desktop H 8→7 (J1 newly applied to unchanged buttons). Phone layout changed, score unchanged. |
| admin Status/Alerts/app detail | 12 | 7.9333… | 7.9666… | Variation only: these pixels changed only in chronology. Status H +1, Alerts phone H +1, app detail Co/D −1. |
| settings | 12 | 7.9333… | 7.9166… | Product: Backup C +1 on 4 files. Variation: Appearance D/Co/A on unchanged pixels. |

- **Coverage:** the same 40 originals and states as round 7. Rounds 1–6 used 32, and that coverage change is not counted as a gain.
- **Variation is not product change:** every retained-screen shift (Status, Alerts, app detail, Appearance, Logs-filters desktop H) is reviewer variation.
- **Product-attributable changes:**
  - Collapsed files: +0.3 on 6 and +0.1 on the 2 Logs desktop files, where J1 still holds H at 7.
  - +0.1 on 4 Backup files.
  - 0.0 on 4 phone-filter files.
- **Per-cohort effect:**
  - admin-console: the product gain is offset by variation.
  - settings: the Backup gain is outweighed by Appearance variation (−0.6 in sum).

## Provenance (scripted, read-only)

| Check | Result |
|---|---|
| Disk, inventory-v8 and manifest-08-after | Exact set equality, 40 files (28 admin-console, 12 settings) |
| sha256, bytes and width×height | 0 mismatches against inventory and manifest. All 40 equal the committed blobs at `4128723`. |
| Duplicates | 0 exact. The closest pair is activity-desktop-light vs activity-filters-desktop-light: 1.443% px, max delta 248, so not near. Matches duplicate-audit-v8 (empty). |
| Manifest | `source_commit` is `1fac233`. `uncommitted_source_sha256` is null. Overflow is 0 on all 40. Disclosure states: 8 collapsed and 8 expanded. |
| 21 source pins | All equal git objects at `1fac233` |
| Palette | VERSION 0.4.3 pins `tokens.css` 636fcf04…, which equals the ky-ui producer `58626b7` git object. I did not use the dirty root ky-ui working bytes. |
| KyNotes brand refs (`1ab04e6`) | All 4 match the disk bytes, the committed blob and their dimensions. I viewed them for brand only and did not rerate Notes. |

## Blinding and priming

- **Before the freeze I read:**
  - the AGENTS chain, the skills, the rubric and design contract
  - the inventory, manifest, duplicate audit and brand pins, none of which has score fields
  - the producer tokens
  - all 40 originals, one at a time at original resolution
  - a 4× crop of the "Down" glyph
  - Backup, palette and phone-geometry pixel samples
  - the 4 Notes refs
- **Freeze:** `2026-10-27T07:17:52Z`.
  - `frozen-round8.json` sha256 `f14c6387…66320`
  - `frozen.py` sha256 `2111458d…8436`
  - Both are in `/tmp/opus-r8-scratch/`.
- **After the freeze I read:**
  - my own `opus-round7.md/json`
  - 07→08 pixel diffs
  - `native-round8.json`
  - my own native tab
- **Priming:**
  - The brief gave history, E1, the CSS change description and check status before I saw any pixels.
  - `web/AGENTS.md` describes the changes.
  - That is priming, not a clean blind. Directory listings showed other reports' names, including `sol-round8.json`, and I opened none of them.
- **Post-freeze numeric changes:** none.

## Native supplement (own incognito tab; Chrome 154, DPR 2 ≠ scored Chromium 153, DPR 1)

- **Login and route check:** I used the snapshot input refs, waited for "Security check verified." and then for `role=navigation[name='Primary']`. There was 1 nav and 0 overflow on every route I checked.
- **Backup, 1280 wide:** `.dr-alert-warn` border `rgb(135,91,18)`, which is `--ky-warning`.
- **Logs, 1280 wide:**
  - The disclosure starts closed. The summary is 79.5×40 with a 1px field border, 8px radius and 14px/500 text.
  - Enter opened it. The outline is accent, 2px with a 2px offset.
  - The filter fields are plain `text` inputs (no datalist) plus `datetime-local`.
- **Activity, 390×844, expanded:**
  - Inputs are 171 and 171 wide, then 358, 358 and 358.
  - Apply sits at y724 and the result at [16,780,358,45].
  - The current tab is inside the nav.
- **Saved theme:** `ky_theme` was absent before and after, and I made no theme change. I closed my own tab only.
- **Not done:** no pair, pin, backup, drill, silence, edit, delete or configuration actions.

## Unverified

- Other roles and populated Logs/Activity.
- Lower page content.
- Named-theme rendered contrast.
- Behaviour acceptance.
- **The full local round-8 browser suite:** the parent reports it as running, with no aggregate pass yet.
- **Remote CI/security at the current head:** the parent reports this as pending. The authored security marker `c976d40` was stale at last inspection, and I make no security clearance claim.
