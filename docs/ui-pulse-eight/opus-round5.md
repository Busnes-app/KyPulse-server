# KyPulse visual8: Opus 5.5, round 5 (stage 05-after)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. I did not open any Sol report, COUNCIL.md, README, the workbook, the CSVs, the audits, verification JSON, `checks/` or geometry-round4.
- Rubric: `ky-suite-v2`. Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15. Each file total uses Decimal half-up rounding to 1 decimal. Each cohort mean is the exact mean of the rounded file totals and is never rounded into 8. Ky fit is separate: five dimensions at .20 each, reported only.
- Machine record: `opus-round5.json` (schema `ky-ui-council-scores/2`, round 5). It holds all 32 rows, drift, sensitivity, blinding, native supplement and the verdict. A second script recomputed every total and mean and checked the rows against the frozen file and the inventory. No mismatches.
- Scope: visual8 progression gate only. This is not full-product 8 or 9/10 Accept. Behaviour gates are untested, and I make no Gemini request.

## Verdict: not satisfied overall

| Cohort | n | Exact mean | My round 4 | Min / max | ≥ 8.0 | Verdict |
|---|---:|---:|---:|---|---|---|
| admin-console (Status, Alerts, app detail, Logs, Activity) | 20 | **7.975** | 7.92 | 7.8 / 8.0 | no | **not satisfied** |
| settings (Settings, Appearance, Backup) | 12 | **8.0333** | 7.9667 | 8.0 / 8.2 | yes | **satisfied** |

There is no pooled, cross-cohort, provider or cross-reviewer mean. There is no competitor in the set, so the Ky gap is N/A.

- **settings clears 8.** Backup desktop now shows each setup state once: badges None, Not paired, 0 of 7 and Daily, and one path. I confirmed this natively. The density slip I scored in round 4 is gone. Settings would still be exactly 8.0 if I held Appearance desktop density at my round-4 judgment (see Drift).
- **admin-console misses by 0.025.** The three round-4 admin slips are fixed in the pixels: the Alerts Time column, the Activity copy and the phone Silence grid. Two items keep the cohort below 8:
  - **A1.** The logs-mobile-light capture clips the current tab.
  - **A2.** My blind density 7 for Activity phone, on a layout that has not changed since I scored it 8 in round 4.

  I did not move any frozen number toward the gate.

## Provenance (scripted, read-only)

| Check | Result |
|---|---|
| 32 `kypulse-05-after-*` on disk vs inventory-v5 vs manifest-05-after | Exact set equality |
| sha256, bytes and width×height vs inventory, manifest and the committed blob at `1c13c82` | 0 mismatches |
| Duplicates | duplicate-audit-v5 lists 0 near duplicates. Every inventory `duplicate_of` is null. |
| Manifest | `source_commit` fb2da30. `uncommitted_source_sha256` null. |
| 21 `source_files` pins | All match `fb2da30` |
| fb2da30 → 1c13c82 | No change outside `docs/ui-pulse-eight`. The worktree was clean before my writes. |
| Palette authority | `check-vendor.mjs`: ky-ui 0.4.3 verified. Vendored `tokens.css` (636fcf04…) equals producer `58626b7` `src/tokens.css`, read as a git object. The root ky-ui working tree was not used. |
| KyNotes brand refs (`1ab04e6`) | All 4 hashes match the committed blob and the file. I viewed 2 for `suite_cohesion` only; none enters a mean. |

Full input hashes are in the JSON `inputs`.

## Blinding

- **Before the freeze I read:**
  - the AGENTS chain, the ky-suite-ui skill, the rubric, the design contract and the council skill
  - inventory, manifest, duplicate audit and brand pin, none of which has score fields
  - all 32 originals, one at a time at full resolution
  - two 4× crops:
    - app-detail "Down": proportional Space Grotesk bold, not mono
    - Logs phone nav, light vs dark
  - 2 KyNotes references
- **Freeze:** rows sha256 `2f9d89ae…ae15`, notes sha256 `4ee89713…400d`, in `/tmp/opus-r5-scratch/`. `build.py` asserts that hash and every total.
- **After the freeze I read:**
  - my own `opus-round4.md/json`
  - stage-04 originals, as a numeric pixel diff plus one nav crop
  - the `web/src` diff e959112..fb2da30
  - the parent's `native-round5.json`
  - my native tab
- **Exposure:**
  - The brief gave the prior objections and the round-5 responses before I saw pixels, and `web/AGENTS.md` describes the fixes. That is priming, not a clean blind.
  - Directory listings showed the names of other reports, including `sol-round5.*`, which appeared during my review. I opened none of them.
- **Post-freeze numeric changes:** none.

## Per-file scores (H,T,D,C,A,Co → total)

All 32 rows are confidence high, with no capture artifacts. Each row's caveat: initial viewport only, live timestamps, synthetic fixture, and the stated empty, unpaired or unconfigured state. Light and dark score the same except on logs mobile.

| Surface | Desktop (light = dark) | Mobile light | Mobile dark |
|---|---|---|---|
| status | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | **8.0** |
| alerts | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | **8.0** |
| app-detail | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | **8.0** |
| activity | 8,8,8,8,8,8 → **8.0** | 8,8,**7**,8,8,8 → **7.8** | **7.8** |
| logs | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,**7**,8 → **7.9** | 8,8,8,8,8,8 → **8.0** |
| settings | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | **8.0** |
| backup | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | **8.0** |
| appearance | 8,8,**9**,8,8,8 → **8.2** | 8,8,8,8,8,8 → **8.0** | **8.0** |

Ky fit, reported only: 8.0 on every file, with each of the five dimensions at 8.

- **Palette:** reads as ky-ui 0.4.3.
- **Type geometry:** the intro size varies by page.
- **Shell:** 240px sidebar with the B stamp, a rail plus tint, and the account region pinned to the bottom.
- **Accent discipline:** orange appears only on the primary, the selection and focus.
- **Suite cohesion:** matches KyNotes.

These are all one step below my round-4 marks of 8.2–8.8. That is coarser judgment, not a product regression.

There is one left navigation region: natively, a single `nav` "Primary". The phone uses horizontal primary navigation with no second panel.

## Remaining items: evidence-backed vs taste

**F** marks an evidence-backed item, visible in the original. **J** marks reviewer judgment. **T** marks taste, which I do not demand.

### admin-console

**A1. F (capture state), Logs phone light: the current tab is clipped.**
- In the scored original, the selected Logs tab is cut at the nav's right edge: "Log" plus part of the "s", with the tint and orange bar cut too. The stage-04 light capture and the stage-05 dark capture both show it fully in view.
- The `AppHeader` `scrollIntoView` code is unchanged since round 4. Natively it held the row in view 7/7 times: 30ms and ~900ms after route changes, and 167ms after a fresh `#/logs` reload.
- Direction, the parent's choice: treat it as capture nondeterminism and recapture under identical conditions, or check whether the scroll runs before the phone nav reaches its final width on a deep-link load. No design change is implied.
- Effect: +0.005 if it reads correctly.

**A2. J (retained layout), Activity phone: the result sits low.**
- Five filter fields and two meta paragraphs put the empty-result line at y≈773 of 844, in 13px meta text.
- The layout is unchanged since round 4, when I scored it density 8, so this is reviewer variance, not a regression.
- It is not demanded. One bounded option, if wanted: put From and To on one phone row, or give the result line body size. No copy, route or handler change is needed.
- Effect: +0.02 at my round-4 judgment.

Both together give exactly 8.0 (see Sensitivity).

### settings

No blocking item.

### Taste, not scored down

- The persistent incident strip, including the redundant Alerts link on the Alerts page.
- The phone chronology and the mono GET line wrap.
- Alerts desktop Change column slack (~280px).
- Unequal Backup setup-card heights.
- The "Back up now" heading repeats its button label.
- Dark Appearance unselected radios are grey discs (shared ky-ui).
- Phone Silence button labels are left-aligned.

Nothing here requires a route, role, CSP, CSRF, API or handler change.

## Sensitivity (not projections or acceptance)

| Change | admin-console | settings |
|---|---:|---:|
| as frozen | 7.975 | 8.0333 |
| Activity phone D → 8 (my round-4 judgment) | 7.995 | — |
| Logs phone light A → 8 | 7.98 | — |
| both admin items | 8.0 | — |
| Appearance desktop D → 8 (my round-4 judgment) | — | 8.0 |

## Drift against my round 4 (read after the freeze)

Stage 04→05 pixel diffs, by channel delta > 8, run from 0% to 10% (Backup desktop light). The per-file values are in `drift.files`.

| File(s) | Round 4 → 5 | Dimension | Cause |
|---|---|---|---|
| alerts desktop ×2 | 7.8 → 8.0 | D 7→8 | Product: the Time column is now intrinsic, 166px for 145.5px of text natively (269.9 in round 4) |
| app-detail phone ×2 | 7.8 → 8.0 | D 7→8 | Product: aligned 2-column Silence grid, last button full width (175/175/358 natively) |
| activity desktop ×2 | 7.8 → 8.0 | H 7→8 | Product: the unfiltered empty copy no longer claims filters |
| activity phone ×2 | 7.8 → 7.8 | H 7→8, D 8→7 | H: the same product copy fix. D: my judgment on a retained layout (A2); none of anchor, cohort, correction or artifact applies |
| backup desktop ×2 | 7.8 → 8.0 | D 7→8 | Product: duplicate badges and the path/retention paragraph are removed |
| logs phone light | 8.0 → 7.9 | A 8→7 | Artifact (capture state): the stage-05 pixels changed only in the nav row (A1) |
| appearance desktop ×2 | 8.0 → 8.2 | D 8→9 | My judgment on unchanged pixels (the diff is banner timestamps only); none of anchor, cohort, correction or artifact applies |

- **Mean drift:**
  - admin-console +0.055 (7.92 → 7.975)
  - settings +0.0667 (7.9667 → 8.0333)
- **Unchanged-pixel judgment drift:**
  - Appearance desktop +0.2 ×2, in settings
  - Activity phone density −0.2 ×2, in admin-console, offset by the copy fix
  - These run in opposite directions, so Ky did not drift one way.
- **Settings without that drift:** if I hold Appearance at its round-4 judgment, settings is still 8.0. Its verdict does not rest on the Appearance up-drift.
- **Ky fit:** fell from 8.2–8.8 to 8.0. This is judgment, never gated.
- There is no competitor drift: there are no competitor files.

## Native supplemental (own incognito tab)

- **Browser:** T3 incognito tab opened with `reuseExistingTab:false`. Chrome/154 UA, DPR 2, 1280×800, then freeform 390×844. This differs from the scored Chromium 153 at scale 1.
- **Sign-in:** `preview_status` first. I took the login refs from the snapshot, waited for "Security check verified", signed in, then waited for `navigation[name='Primary']`.
- **Measured:**
  - 1 nav, labelled Primary. No horizontal overflow at 1280 or 390.
  - Backup: badges `None, Not paired, 0 of 7, Daily`, the path appears once, setup cards 335/367.
  - Alerts: Time column 166px vs 145.5px of text.
  - Activity copy:
    - initial and draft: "No audit events found in the retained 7-day window."
    - after Apply: "No audit events match these filters…"
    - after Clear: the unfiltered text again
  - Phone nav: the current row stayed in view 7/7.
- **No writes:** no pair, pin, silence, backup, edit, delete or theme change. `ky_theme` stayed null and localStorage was empty. I closed only my tab.
- **Locator errors:** none this round.
- These are supplemental reads. They replace no original and pass no gate.

## Checks and limitations

- **Run by me:** read-only provenance, pin, vendor, pixel-diff and arithmetic scripts (scratch `/tmp/opus-r5-scratch/`, outside the repo), plus the native reads above.
- **Not run by me:** frontend tests, build, `make ci`, browser, race, FIPS, Postgres, Docker, repeatable-build and security checks. The brief reports the fresh frontend/build/`make ci` passes and says the remote full-head checks are pending. I make no claim about either.
- **Gates:** keyboard, focus, contrast, states and responsive are untested.
- **Coverage:** synthetic admin only, initial viewport, empty and unpaired Logs/Activity, unconfigured Settings/Backup.
- **Not covered (unverified, not zero):** the viewer role, populated timelines, lower page content, named-theme contrast and complete behaviour acceptance.
- **Review limit:** this is one reviewer's blind judgment on 32 captures. Admin-console sits within reviewer variance of the gate in both directions.
