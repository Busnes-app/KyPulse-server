# KyPulse visual8: Opus 5.5, round 6 (stage 06-after)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. I opened no Sol report, COUNCIL.md, README, workbook, CSV, audit, verification JSON, `checks/`, geometry or earlier native JSON. I also opened none of my own rounds 1–4.
- Rubric `ky-suite-v2`. Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15.
  - File totals use Decimal half-up rounding to one decimal.
  - Cohort means are exact means of the rounded file totals. They are never rounded into 8.
  - Ky fit is reported only.
- Machine record: `opus-round6.json` (`ky-ui-council-scores/2`, round 6). It holds 32 rows, drift, sensitivity, blinding, the native supplement and the verdict. A separate script recomputed every total and both means and checked the file list against `inventory-v6.json`: 0 mismatches.
- Scope: the visual8 progression gate only. This is not full-product 8, nor 9/10 Accept. Every behaviour gate is untested. I did not call Gemini or any subagent.

## Verdict: not satisfied in either cohort

| Cohort | n | Exact mean | My round 5 | Min / max | ≥ 8.0 | Verdict |
|---|---:|---:|---:|---|---|---|
| admin-console (Status, Alerts, app detail, Logs, Activity) | 20 | **7.96** | 7.975 | 7.8 / 8.0 | no | **not satisfied** |
| settings (Settings, Appearance, Backup) | 12 | **7.98333…** (479/60) | 8.03333… | 7.9 / 8.0 | no | **not satisfied** |

- There is no pooled, provider or cross-reviewer mean. There is no competitor, so the Ky gap is N/A. Gemini must wait.
- **No evidence-backed defect remains in either cohort.**
  - Round 5's only defect, the clipped selected Logs tab on phone light, is gone in both phone Logs originals. A 4× crop shows both rounded corners in each.
  - Natively, all 7 phone routes and a fresh `#/logs` reload kept the current item fully inside the nav.
- **The shortfall is entirely my judgment on retained layouts** (A1 and S1 below). In round 5 I scored those same layouts higher. The settings cohort fell below 8 only because of that judgment, not because of any product change: the Backup desktop and Appearance desktop pixels differ from stage 05 only in live timestamps and the temp path. I am reporting the frozen numbers and not repairing them toward the gate.

## Provenance (scripted, read-only)

| Check | Result |
|---|---|
| `kypulse-06-after-*` on disk, inventory-v6 and manifest-06-after | Exact set equality, 32 files |
| sha256, bytes and width×height | 0 mismatches against inventory, manifest and the committed blob at `c976d40` |
| Duplicates | duplicate-audit-v6 lists 0 near duplicates. Every `duplicate_of` is null. |
| Manifest `source_commit` | `447f25a`. `uncommitted_source_sha256` is null. Overflow is 0 on all 32 shots. |
| 21 `source_files` pins | All match `447f25a` |
| `447f25a` → `c976d40` | Changes only under `docs/ui-pulse-eight` |
| Palette authority | `check-vendor.mjs`: ky-ui 0.4.3 verified. Vendored `tokens.css` (636fcf04…) equals producer `58626b7` `src/tokens.css`, read as a git object. I did not use the root ky-ui working tree. |
| KyNotes brand refs (`1ab04e6`) | All 4 match the file and the committed blob. I viewed 2 for `suite_cohesion` only. None enters a mean. |

The round-6 source diff (`fb2da30..447f25a`, `web/src`) has four parts:
- AppHeader re-runs `scrollIntoView` on `document.fonts.ready`, with a cancel flag.
- Logs and Activity add `.log-empty` to the empty `<p>`.
- One CSS rule: `.log-empty { font-size: var(--ky-type-body); color: var(--ink-strong) }`.
- `capture.mjs` now waits until the current nav item is inside the nav box. It does not scroll.

No API, handler, role, route, CSP or CSRF code changed.

## Blinding and priming

- **Before the freeze I read:**
  - the AGENTS chain, the skill, the rubric, the design contract and the council skill
  - the inventory, manifest, duplicate audit and brand pins, none of which has score fields
  - all 32 originals, one at a time at full resolution
  - a 3× crop of app-detail "Down" (proportional Space Grotesk bold: the `w` is wider than the `o`; only the GET line is mono)
  - 4× crops of the phone Logs current tab in light and dark (complete)
- **Freeze:** 2026-10-27T06:26:21Z.
  - `frozen-round6.json` sha256 `e097a890…0010`
  - `frozen.py` sha256 `2c9dbcc1…0fce`
  - Both are in `/tmp/opus-r6-scratch/`. `build.py` asserts the hash and every total.
- **After the freeze I read:**
  - my own `opus-round5.md/json`
  - 05→06 pixel diffs of all 32
  - the source diff
  - `native-round6.json`
  - my own native tab
  - two KyNotes references
- **Priming:**
  - The brief gave me the prior objections, the round-6 change and the parent's native findings before I saw pixels. `web/AGENTS.md` also describes the fixes. That is priming, not a clean blind.
  - Directory listings showed the names of other reports. I opened none of them.
  - I assigned Ky fit after the freeze and after reading round 5. It is reported only.
- **Post-freeze numeric changes:** none.

## Per-file scores (H,T,D,C,A,Co → total)

- All 32 rows have high confidence, with no visible artifacts.
- Each caveat covers the synthetic fixture, live timestamps, initial viewport only, and the stated empty, unpaired or unconfigured state.
- Light and dark score identically.

| Surface | Desktop | Mobile |
|---|---|---|
| status | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** |
| alerts | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** |
| app-detail | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** |
| activity | 8,8,8,8,8,8 → **8.0** | 8,8,**7**,8,8,8 → **7.8** |
| logs | 8,8,8,8,8,8 → **8.0** | 8,8,**7**,8,8,8 → **7.8** |
| settings | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** |
| backup | 8,8,8,8,8,**7** → **7.9** | 8,8,8,8,8,8 → **8.0** |
| appearance | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** |

Representative observations (the JSON has one per dimension per file):

- **Status:** failure-first rows. The single orange Add app. Status words are paired with dots. Selected nav uses a tint plus the accent rail.
- **Alerts desktop:** the Time column fits its unbroken timestamp. Only `status_503` is mono. The Change column has slack, which is taste.
- **Alerts phone:** the Time, Reason and Delivery labels are proportional.
- **App detail:** "Down" is proportional bold. Phone Silence is an aligned 2+1 grid. The Checks heading carries its empty note inline, while KyYard and Recent logs stack theirs. That is a small variation, scored 8.
- **Logs and Activity desktop:** one filter grid with Apply as the single orange action. The empty result is now 15px in strong ink. "No paired sources." stays 13px muted as a sub-list note.
- **Logs and Activity phone:** the result is legible at body size, but the five-field filter stack puts it at the viewport bottom: density 7, item A1.
- **Backup desktop:** the setup cards no longer repeat summary values. Disabled buttons are neutral and distinct from Run restore drill. The stack runs card / flat fact rows / card: coherence 7, item S1.
- **Settings and Appearance:** one card vocabulary. The shared picker has an unambiguous Match system selection.

Ky fit (reported only) is 8.0 on every file, with each of the five dimensions at 8:
- palette reads as ky-ui 0.4.3
- 24px title, 40px controls, 8px radius
- one left navigation region, or one horizontal Primary nav on phone
- orange limited to the primary, the selection and focus
- coheres with the pinned KyNotes captures

## Remaining items: evidence vs judgment vs taste

### admin-console

No defect remains. Round 5's A1 tab clip is fixed in the originals. The small, low empty result is now body size.

**A1. Judgment on a retained layout: Logs and Activity phone filter stack.**
- **What the image shows:** five stacked fields and two notes push the only result to the bottom of the 844px viewport. Natively the Activity result sits at y 763.5–808.5 and the Logs result at 744–766.5. Log sources starts at the fold.
- **Why it is not a defect:** nothing clips or overlaps. In round 5 I scored this same layout density 8 on Logs and density 7 on Activity. Making both 7 now is consistency on my side, which is reviewer variation, not regression.
- **Not demanded.** One bounded option is From and To on one phone row, with no copy, route or handler change.

### settings

No defect remains.

**S1. Taste-leaning judgment on a retained composition: Backup desktop.**
- **What the image shows:** setup cards, then flat fact rows, then the Back up now card. That is a visible container change across three stacked bands.
- **Why it is not a defect:** I scored this same composition coherence 8 in round 5. It sits close to the owner-designated taste item, unequal card heights.
- **Not demanded.**

### Taste, not scored down

- The persistent incident strip, including the redundant Alerts link on the Alerts page.
- The phone chronology and the mono GET line wrap.
- Alerts desktop Change column slack.
- Unequal Backup setup-card heights.
- "No paired sources." at 13px muted beside the 15px primary result.
- The inline Checks note.
- The Settings intro wraps "Appearance." onto a second line.

## Sensitivity (not projections or acceptance)

| Change (my own alternative judgment) | admin-console | settings |
|---|---:|---:|
| as frozen | 7.96 | 7.98333 |
| Logs phone D 7→8, both themes (my round-5 judgment) | 7.98 | — |
| all four phone filter screens D 7→8 | 8.00 | — |
| Backup desktop Co 7→8, both themes (my round-5 judgment) | — | 8.00 |
| Appearance desktop D 8→9, both themes (my round-5 judgment) | — | 8.01667 |
| one more 1-point drop on any 8.0 admin file | 7.95 | — |

- Each cohort verdict hinges on single-point judgments of retained layouts.
- Backup desktop at Co 8 has a weighted sum of 8.0. At Co 7 the sum is 7.85, which half-up rounding makes 7.9.

## Drift against my round 5 (read after the freeze)

| File(s) | R5 → R6 | Dimension | Cause |
|---|---|---|---|
| logs phone light | 7.9 → 7.8 | A 7→8, D 8→7 | **A: artifact/product.** The tab is complete: AppHeader reveals again after fonts, and the driver waits for its bounds. **D: reviewer variation** on the retained filter stack. |
| logs phone dark | 8.0 → 7.8 | D 8→7 | Reviewer variation. The layout is retained and is now scored the same as Activity phone. |
| activity phone ×2 | 7.8 → 7.8 | none | The 15px strong result is a visible product change, but its position is unchanged, so D stays 7. No credit, no penalty. |
| appearance desktop ×2 | 8.2 → 8.0 | D 9→8 | Reviewer variation. The 05→06 diff covers only the banner rows (y 14–28), and round 5 already flagged my 9 as drift. |
| backup desktop ×2 | 8.0 → 7.9 | Co 8→7 | Reviewer variation. The diff is only live timestamps and the random temp path. |
| other 22 files | 0 | none | — |

- Mean drift: admin-console −0.015 (7.975 → 7.96); settings −0.05 (8.0333 → 7.9833).
- Only one delta has a cause other than reviewer variation: the Logs phone light tab, which is a real improvement. No file rose in score.
- Re-scoring retained pixels measures me, not the product. Do not read these drops as regressions, or the round-5 settings 8.03 as a product gain that was lost.

## Native supplement (Chrome 154, DPR 2: differs from the Chromium 153, DPR 1 originals)

I opened my own incognito tab (`reuseExistingTab:false`) against `127.0.0.1:18429`.
- **Sign-in:** filled the inputs from snapshot refs, waited for "Security check verified", submitted, then waited for `role=navigation[name='Primary']`.
- **Writes:** none. No theme was saved (`ky_theme` stayed null; `data-theme` was busnes-light from the OS).
- **Cleanup:** I closed the tab.

What I measured:
- **Phone 390×844:**
  - On Status, Alerts, Activity, Logs, Backup, Settings and Appearance: 1 `nav`, nav box [16, 374], the current item fully inside it, overflowX 0, scrollY 0.
  - Logs at [293.5, 374] after a fresh `#/logs` load, at 0, 150 and 600 ms. Fonts were already cached, so this cannot reproduce or disprove the font race.
  - Both empty results at 15px, `rgb(24, 35, 38)`.
- **Desktop Activity 1280×900:**
  - Unfiltered copy: "No audit events found in the retained 7-day window."
  - A typed but unapplied actor left it unchanged.
  - Apply switched it to "No audit events match these filters. Within the retained 7-day window; filters may exclude retained events."
  - Clear restored the unfiltered copy and emptied the inputs.
- **Console:** one 404 for `/api/targets/1`. My own probe of a nonexistent `#/apps/1` caused it (real IDs are `tgt_*`). It is not a product error.

This native check supplements the original review and does not replace it.

## Not verified by me

- The delayed-font regression failing before and passing after. That is the parent's claim; I did not rerun it.
- The aggregate of the full local browser suite, which is still pending.
- Remote browser, FIPS, Postgres, Docker, repeatable-build and security CI.
- Other roles, populated Logs and Activity, content below the fold, named-theme contrast, and every behaviour gate.

## Answer

- **admin-console: not satisfied.** The exact mean is 7.96. No defect remains, and the deduction is A1, my judgment on a retained layout.
- **settings: not satisfied.** The exact mean is 7.98333. No defect remains, and the deduction is S1, a taste-leaning judgment on a retained composition.
- Gemini must still wait.
- The parent decides whether a bounded A1 or S1 change is worth one more round. I do not demand either, and I do not claim that a change would reach any score.
