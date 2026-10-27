# KyPulse visual8: Opus 5.5, round 7 (stage 07-after)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. No subagents, no Gemini.
- Rubric `ky-suite-v2` (sha256 `ad351662…`). Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15.
  - File totals use Decimal half-up rounding to one decimal.
  - Cohort means are exact means of the rounded totals. They are never rounded into 8.
  - Ky fit is reported only and never enters a total.
- Machine record: `opus-round7.json` (`ky-ui-council-scores/2`, round 7). It holds 40 rows, inventory, drift, sensitivity, blinding, the native supplement and the verdict. `build.py` asserted the freeze hash, recomputed every total and Ky fit, and checked the file list against `inventory-v7.json`: 0 mismatches.
- Scope: the visual8 progression gate only. This is not full-product 8, not 9/10 Accept, and makes no consensus or deployment claim. Every behaviour gate is untested.

## Verdict: not satisfied in either cohort

| Cohort | n | Exact mean | Fraction | Min / max | Ky fit mean | ≥ 8.0 | Verdict |
|---|---:|---:|---|---|---:|---|---|
| admin-console (Status, Alerts, app detail, Logs, Activity; collapsed and expanded filters) | 28 | **7.857142857…** | 55/7 | 7.7 / 8.0 | 8.0 | no | **not satisfied** |
| settings (Settings, Appearance, Backup) | 12 | **7.933333…** | 119/15 | 7.5 / 8.2 | 8.0666… | no | **not satisfied** |

- There is no pooled, cross-reviewer or cross-cohort mean. There is no competitor, so the Ky gap is N/A. Gemini must wait.
- These are the frozen numbers, with no threshold repair.

### Concrete blocker (evidence-backed)

**E1: the Backup warning notice uses the accent colour** (settings; all four Backup files).
- **Evidence:** the "No backups are being made…" notice is outlined in `--ky-accent` over accent-soft fill.
  - Pixels: border `#bf3f18` in light and `#f5865f` in dark.
  - Computed native border: `rgb(191, 63, 24)`.
  - `--ky-danger` is `#b42318` and `--ky-warning` is `#875b12`, so the border is neither.
  - Source: `.dr-alert-warn { … background: var(--accent-soft); border-color: var(--accent) }` at `web/src/styles/theme.css:326` (1657c1b).
- **Contract:** orange is reserved for the single primary action, selection and focus. Status uses `--ky-warning`/`--ky-danger` with text.
- **Fix:** move `.dr-alert-warn` to the existing warning (or danger) alias. No copy, route or behaviour change.
- **History:** this is retained from earlier rounds, not a round-7 regression. In round 6 I misread it as danger. The colour 8→7 drop on Backup is therefore a **correction**, not a product change.

### Judgment deductions (not blockers)

- **J1, admin-console, 8 collapsed Logs/Activity files: H7 and A7.**
  - The collapsed "Filters" control is a bare ▸ with 14px text. It has no control shape, and no filled action is in view.
  - Natively it is a 40px full-width row with a 2px accent focus ring, so this is visual affordance only.
- **J2, admin-console, 4 expanded phone filter files: D7.**
  - The 2 half-width + 3 full-width field stack pushes the result to the viewport bottom.
  - Activity's result continues below the fold. That is normal vertical scrolling and is scored as captured.
  - This is the same retained layout I scored D7 in round 6. I am not reopening the paired-date option.
- **J3, settings, Backup desktop: H7 and D7.**
  - Refresh, Pin key and a three-button row whose heading "Back up now" repeats its button label all compete with the Pair primary.
  - The unequal ends of the two setup columns overlap the owner-designated unequal-heights taste item. I disclose that overlap rather than unscoring it.
- **J4, reviewer variation on unchanged screens.**
  - Alerts phone H 8→7: the per-alert app name is small 13–14px text and carries less emphasis than its status change.
  - App detail desktop D 8→7: uneven section rhythm.
  - Appearance Co 8→9 in the other direction.
  - 06→07 pixels on these screens differ only in live chronology.

### Taste, not scored down

- The persistent incident band repeats the newest incidents.
- Phone chronology wraps.
- Settings keeps card panels while Backup is flat.
- The Status intro leaves "controls." as an orphan.
- Unequal Backup setup heights, beyond the J3 disclosure.

### Numerical limits (sensitivity, informational only, not repair)

| Change | admin-console | settings |
|---|---:|---:|
| as frozen | 7.857142… | 7.933333… |
| E1 fixed (C 7→8 on 4 Backup files) | 7.857142… | 7.966666… |
| Collapsed disclosure reads as a control (A 7→8 on 8 files) | 7.885714… | 7.933333… |
| Backup desktop D 7→8 (drop the taste-overlap deduction) | 7.857142… | 7.966666… |
| All three | 7.885714… | 8.0 |

- **admin-console:** cannot reach 8 through the disclosure affordance alone. Its gap is spread across J1, J2 and J4.
- **settings:** reaches exactly 8.0 only if E1 is fixed and a taste-overlapping deduction is also dropped. Fixing E1 alone is not enough.

## Provenance (scripted, read-only)

| Check | Result |
|---|---|
| `kypulse-07-after-*` on disk, inventory-v7 and manifest-07-after | Exact set equality, 40 files (28 admin-console, 12 settings) |
| sha256, bytes and width×height | 0 mismatches against inventory, manifest and the committed blobs at `54fc13e` |
| Duplicates | 0 exact. The pairwise near-duplicate scan's closest pair is activity collapsed vs expanded desktop light: 1.44% px, max delta 248, so not near. Every `duplicate_of` is null and matches duplicate-audit-v7 (empty). |
| Manifest `source_commit` | `1657c1b`. `uncommitted_source_sha256` is null. Overflow is 0 on all 40. Disclosure states: 8 collapsed and 8 expanded, as named. |
| 21 `source_files` pins | All equal git objects at `1657c1b`. `1657c1b..54fc13e` touches only `docs/ui-pulse-eight`. |
| Palette authority | Vendored VERSION 0.4.3 pins `tokens.css` 636fcf04…, which equals the ky-ui producer `58626b7` `src/tokens.css` git object. I did not use the dirty root ky-ui working bytes. |
| KyNotes brand refs (`1ab04e6`) | All 4 match the file and the committed blob, and I viewed all 4 for brand cohesion only. None enters a mean, and Notes is not rerated. |

## Blinding and priming

- **Before the freeze I read:**
  - the AGENTS chain, the skills, the fixed rubric and design contract
  - the inventory, manifest, duplicate audit and brand pins, none of which has score fields
  - the producer tokens
  - all 40 originals, one at a time at original resolution
  - 3× and 4× crops of the "Down" letterforms. "Down" is the bold proportional body face. The GET diagnostics line and Health URL are fixed width.
- **Freeze:** 2026-10-27T06:52:42Z.
  - `frozen-scores.json` sha256 `c47c11ef…afcb6`
  - `scores_src.py` sha256 `806f3cb0…3cc6`
  - Both are in `/tmp/opus-r7/`.
- **After the freeze I read:**
  - my own `opus-round6.json/md`
  - 06→07 pixel diffs of the 32 matched originals
  - one round-6 Backup image, for cause attribution
  - Backup notice pixels and source
  - `native-round7.json`
  - my own native tab
- **Priming:**
  - The brief supplied prior objections, the round-7 change description and the parent's check status before I saw any pixels.
  - `web/AGENTS.md` describes the disclosure and flat-Backup changes.
  - That is priming, not a clean blind. Directory listings showed other reports' names, and I opened none of them.
- **Post-freeze numeric changes:** none.

## Per-file scores (H,T,D,C,A,Co → total; Ky fit)

All 40 rows are high confidence with no visible artifacts. Light and dark score identically.

| Surface | Desktop | Mobile |
|---|---|---|
| status | 8,8,8,8,8,8 → **8.0**; 8.0 | 8,8,8,8,8,8 → **8.0**; 8.0 |
| alerts | 8,8,8,8,8,8 → **8.0**; 8.0 | **7**,8,8,8,8,8 → **7.8**; 8.0 |
| app-detail | 8,8,**7**,8,8,8 → **7.8**; 8.0 | 8,8,8,8,8,8 → **8.0**; 8.0 |
| logs (collapsed) | **7**,8,8,8,**7**,8 → **7.7**; 8.0 | **7**,8,8,8,**7**,8 → **7.7**; 8.0 |
| logs-filters (expanded) | 8,8,8,8,8,8 → **8.0**; 8.0 | 8,8,**7**,8,8,8 → **7.8**; 8.0 |
| activity (collapsed) | **7**,8,8,8,**7**,8 → **7.7**; 8.0 | **7**,8,8,8,**7**,8 → **7.7**; 8.0 |
| activity-filters (expanded) | 8,8,8,8,8,8 → **8.0**; 8.0 | 8,8,**7**,8,8,8 → **7.8**; 8.0 |
| settings | 8,8,8,8,8,8 → **8.0**; 8.0 | 8,8,8,8,8,8 → **8.0**; 8.0 |
| backup | **7**,8,**7**,**7**,8,8 → **7.5**; 7.8 | 8,8,**7**,**7**,8,8 → **7.7**; 7.8 |
| appearance | 8,8,8,8,8,**9** → **8.2**; 8.4 | 8,8,8,8,8,**9** → **8.2**; 8.4 |

Ranking within each cohort follows this table. The JSON lists `rank_order` and holds one concrete observation per dimension per file.

### Ky fit

- **Palette:** Busnes cream and charcoal with orange throughout.
- **Shell:** contract shell (240px sidebar, stamp wordmark, pinned account; phone tabs).
- **Suite cohesion:** the nav tint and rail, outline secondaries and Appearance cards match the KyNotes references.
- **Appearance:** accent and cohesion score 9 because the card picker is identical to Notes.
- **Backup:** accent discipline scores 7 (E1).

## Drift against my round 6, reported separately

- **Common state, matched files only:**
  - admin-console, n=20: 7.96 → **7.92** (−0.04). Expanded Logs/Activity are compared with round 6's always-visible filters, and all four pairs are unchanged at 8.0/8.0/7.8/7.8.
  - settings, n=12: 7.98333… → **7.93333…** (−0.05).
- **Coverage change:** +8 collapsed admin files, each 7.7, with no prior. Most of the raw admin difference (7.96 → 7.857) is this added coverage. It is not a patch-loss or patch-gain claim.
- **Per-file causes:**
  - Alerts phone (−0.2), app detail desktop (−0.2) and Appearance (+0.2): reviewer variation on unchanged pixels.
  - Backup desktop (−0.4): the product changed (wrappers removed, about 35% of pixels). Co 7→8 credits the flat surface. H and D −1 are judgment (J3). C −1 is a correction (E1).
  - Backup mobile (−0.3): the product changed (about 47% of pixels). D −1 is judgment. C −1 is the E1 correction.
- **Not a product gain:** retained-screen changes in either direction are reviewer variation.

## Native supplement (own incognito tab; Chrome 154, DPR 2 ≠ scored Chromium 153, DPR 1)

- **Backup, 1280 wide:**
  - `.dr-alert-warn` border `rgb(191,63,24)`, which is the accent colour (E1).
  - 0 `.panel` in main, 1 nav.
- **Logs, 1280 wide:**
  - The disclosure starts closed. The summary is 14px/500, 976×40.
  - Focus plus Enter opened it. The focus ring is accent, solid 2px with a 2px offset.
  - A typed draft left the summary as "Filters". After Apply it read "Filters Applied" (13px muted).
  - Close and reopen kept the values and the result. Clear restored "Filters".
- **390×844:**
  - Logs, Activity and Backup: 1 nav and 0 horizontal overflow. The Logs current tab sat inside the nav at 100, 500 and 1500 ms after the route change.
  - One reading taken immediately after resizing an open `#/logs` from 1280 to 390 put the Logs tab outside the nav. A second resize repro showed it inside.
  - That reading is **unproven**, outside the capture conditions, and neither scored nor claimed as a defect.
- **Saved theme:** null before and after. Appearance emulation was reset to system and the tab closed.
- **Not done:** no pair, pin, backup, silence, edit, delete or configuration actions.

## Unverified

- Other roles and populated Logs/Activity.
- Lower page content.
- Rendered contrast of the named themes.
- Full behaviour acceptance.
- The current full browser suite and remote CI/security for round 7. The parent reports these as running or pending, and I make no security clearance claim.
