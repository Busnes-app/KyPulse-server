# KyPulse visual8: Opus 5.5, round 4 (stage 04-after)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. I did not open any Sol report, COUNCIL.md, README, the workbook, the CSVs, the audits, verification/native JSON or `checks/`.
- Rubric: `ky-suite-v2`. Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15. Each file total uses Decimal half-up rounding to 1 decimal. Each cohort mean is the exact mean of the rounded file totals and is never rounded into 8. Ky fit is separate: five dimensions at .20 each.
- Machine record: `opus-round4.json` (schema `ky-ui-council-scores/2`, round 4). It holds all 32 rows, drift, sensitivity, blinding and the verdict. A second script recomputed every total, Ky fit and mean, and checked the rows against the frozen file and the inventory. No mismatches.
- Scope: visual8 progression gate only. This is not 9/10 Accept, behaviour gates are untested, and I make no Gemini request.

## Verdict: not satisfied

| Cohort | n | Exact mean | My round 3 | Min / max | ≥ 8.0 |
|---|---:|---:|---:|---|---|
| admin-console (Status, Alerts, app detail, Logs, Activity) | 20 | **7.92** | 8.00 | 7.8 / 8.0 | **no** |
| settings (Settings, Appearance, Backup) | 12 | **7.9667** | 7.9667 | 7.8 / 8.0 | **no** |

There is no pooled, cross-cohort, provider or cross-reviewer mean. There is no competitor in the set, so the Ky gap is N/A.

- **settings is still below 8.** The round-4 change did what it claims; I confirmed it natively: no icon, `align-items:start`, cards 334.5px and 418px. My blind read still gives Backup desktop density 7, for two reasons. First, there is an ~82px ragged gap under the shorter KyRecovery card. Second, the summary rows repeat setup state the cards show just above them: None ×2, Not paired ×2, and the local path ×2.
- **admin-console fell from 8.00 to 7.92 with no product change.** Stage 03→04 pixels on those screens differ by ≤1.3%, all of it timestamps and ages. Blind, I scored three slips I already knew about one point lower than I did in round 3. This is reviewer variance, not a regression. Neither round misread the image. I did not move any frozen number toward my prior or toward the gate.

## Provenance (scripted, read-only)

| Check | Result |
|---|---|
| 32 `kypulse-04-after-*` on disk vs inventory-v4 vs manifest-04-after | Exact set equality. No duplicate names. |
| sha256, bytes and width×height vs inventory, manifest and the committed blob at HEAD | 0 mismatches |
| Duplicates | 0 exact, 0 near. Closest pair is activity vs logs desktop light: 4.21% of pixels differ, max channel delta 224. |
| Manifest | `overflow` 0 ×32. `source_commit` 1bf757d. `uncommitted_source_sha256` null. |
| 21 `source_files` pins | All match the worktree and `1bf757d` |
| HEAD `e959112` vs `1bf757d` | `web/src` and `web/dist` identical. The only commit after the source commit touches `docs/` only. The worktree is clean. |
| `web/dist/index.html` | Equals `served_index_sha256` |
| KyNotes brand refs (`1ab04e6`) | All 4 hashes match. I viewed 2 for `suite_cohesion` only; they are excluded from every mean. |

Input sha256:

- inventory-v4: `a0f20da9…950f`
- manifest-04-after: `49fec56d…7dc`

The full hashes of the rubric, brand pin, my round-3 pair and geometry-round4 are in the JSON `inputs`.

## Blinding

- **Before the freeze I read:**
  - the AGENTS chain, the ky-suite-ui skill, the rubric, the design contract and the council skill
  - the inventory, manifest and brand pin, none of which has score fields
  - all 32 originals, one at a time at full resolution
  - two 3× crops of originals, to check letterforms
  - 2 KyNotes references
- **Freeze:** 2026-10-27T05:43:09Z. Rows sha256 `31eab0a7…3a0a`, notes sha256 `719711d9…a81`. Both files are in scratch and read-only.
- **After the freeze I read:**
  - my own `opus-round3.md/json`
  - stage-03 originals, as a numeric pixel diff only
  - `geometry-round4.json`
  - my native tab
- **Exposure:**
  - The brief gave the prior objections, the round-4 change and the parent's 334.5/416px measurement before I saw any pixels. `web/AGENTS.md` describes the fixes too. That is priming, not a clean blind.
  - My provenance script printed the *names* of the files changed after 1bf757d, including `sol-round3.*`. I did not open them.
- **Pre-freeze self-correction:**
  - My first note read the app-detail "Down" as Plex Mono. That is the same misreading I disclosed in rounds 2 and 3.
  - The 3× crop shows proportional glyph widths, unlike the mono GET line under it, so I never scored a penalty for it.
- **Post-freeze numeric changes:** none.

## Per-file scores (H,T,D,C,A,Co → total)

Light and dark score identically on every surface. All 32 rows are confidence high, with no capture artifacts. Each row's caveat: initial viewport only, live timestamps, synthetic fixture, and the empty, unpaired or unconfigured state where it applies.

| Surface | Desktop | Mobile | Ky fit, light / dark (d · m) |
|---|---|---|---|
| status | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.8/8.6 · 8.6/8.4 |
| alerts | 8,8,**7**,8,8,8 → **7.8** | 8,8,8,8,8,8 → **8.0** | 8.8/8.6 · 8.6/8.4 |
| app-detail | 8,8,8,8,8,8 → **8.0** | 8,8,**7**,8,8,8 → **7.8** | 8.8/8.6 · 8.6/8.4 |
| logs | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.8/8.6 · 8.6/8.4 |
| activity | **7**,8,8,8,8,8 → **7.8** | **7**,8,8,8,8,8 → **7.8** | 8.8/8.6 · 8.6/8.4 |
| settings | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.8/8.6 · 8.6/8.4 |
| appearance | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.8/8.6 · 8.6/8.4 |
| backup | 8,8,**7**,8,8,8 → **7.8** | 8,8,8,8,8,8 → **8.0** | 8.6/8.4 · 8.4/8.2 |

Ky fit, reported only:

- palette_tokens 9, or 8 in dark, where the danger coral sits close to the salmon accent
- type_geometry 8, because meta drops to ~12px and intros are 13px on some pages and 15px on others
- shell 9 on desktop, 8 on phone (the phone shows no name or role)
- accent_discipline 9, or 8 on Backup, where the problem banner border uses the accent hue
- suite_cohesion 9 against KyNotes

There is one left navigation region. Natively there is a single `nav` "Primary" at x=16, w=207. The phone uses horizontal primary navigation with no second panel.

No dimension earns a 9. Every screen keeps at least one visible slip, or a repeated element such as the persistent incident strip that a refined screen would drop.

## Remaining items: evidence-backed vs taste

**F** marks an evidence-backed item, visible and measured. **T** marks taste, which I do not demand.

### settings

**S1. F, Backup desktop: setup state is shown twice.**
- The summary rows repeat None ×2 and Not paired ×2 (native badge list), plus the backups path (card sentence and the Local copies row), directly below the setup cards.
- Bounded direction: while setup is incomplete, show each state once. No copy, value or route change is needed.

**S2. T, Backup desktop: unequal card heights.**
- The content-height cards (334.5 and 416–418px) leave ~82px empty under KyRecovery.
- This is legitimate, and better than round 3's stretched in-card void. On its own I would not demand a change.

### admin-console (all unchanged since round 3)

**A1. F, Alerts desktop: Time column.** Natively the Time column is 270px wide for 140.5px of text, which pushes the other columns right.

**A2. F (copy), Activity empty message.** It says "No audit events match these filters" when every filter is empty, then adds a fragment. You kept this wording on purpose. In my blind read it costs one hierarchy point.

**A3. T, app detail phone.** The Silence buttons wrap 2+1, and the mono GET line breaks mid-timestamp. This is a natural responsive wrap.

**Also T, not scored down:**
- The persistent incident strip, with a redundant "Alerts" link on the Alerts page.
- Alerts has no intro line.
- Settings' intro leaves "Appearance." alone on its second line.
- Dark Appearance uses filled grey unselected radios. This is shared ky-ui and identical in KyNotes.

Nothing here requires route, role, CSP, CSRF, API or handler changes.

## Sensitivity (not projections or acceptance)

| Change | admin-console | settings |
|---|---:|---:|
| as frozen | 7.92 | 7.9667 |
| Alerts desktop D → 8 | 7.94 | — |
| Activity H → 8 (4 files) | 7.96 | — |
| App detail phone D → 8 | 7.94 | — |
| All three admin items at my round-3 judgment | 8.00 | — |
| Backup desktop D → 8 | — | 8.00 |

Admin-console reaches 8 only if all three of my blind admin slips are judged as I judged them in round 3 on the same pixels. Settings depends on Backup desktop density alone.

## Drift against my round 3 (read after the freeze)

- **admin-console −0.08 mean.** Eight files moved −0.2 each:
  - alerts desktop ×2: D 8→7
  - app-detail phone ×2: D 8→7
  - activity ×4: H 8→7

  Both rounds cite the same visible facts, so none of the rubric causes (anchor, cohort, correction, artifact) applies. This is reviewer judgment noise on unchanged pixels, not product change.
- **settings 0.00 mean.** The Backup desktop pixels changed by 10.8%. Round 3's S1 (detached icon) and its stretched in-card void are gone, but my density score stayed at 7 for the reasons in S1 and S2 above. The other 10 settings files are unchanged in pixels and in score.
- **Ky fit:**
  - accent_discipline and suite_cohesion moved 8→9 off Backup.
  - Dark palette moved 9→8.

  This is judgment drift. Ky fit is reported only and never gated.

Per-file prior total, total, delta, dimension changes and cause are in `drift.files`.

## Native supplemental (own incognito tab)

- **Browser:** T3 incognito, Chrome/154 UA, DPR 2, 1280×800. This differs from the scored Chromium 153 at scale 1 and 1280×900.
- **Actions:** `preview_status` first, then I opened my own tab. I signed in as the synthetic admin and waited for `navigation[name='Primary']`. I then changed the hash route only, to `#/backup` and `#/alerts`, and read geometry.
- **Measured:**
  - 1 nav, `align-items:start`, cards 334.5/418, 0 icons in the note, overflow 0
  - badges `Not paired, None, None, Not paired, 0 of 7, Daily`
  - Alerts Time column 269.9px vs 140.5px of text
- **No writes:** no pair, pin, silence, backup, edit, delete or theme change. `ky_theme` stayed null and localStorage was empty. I closed the tab. The theme was never changed, so there was nothing to restore.
- These measurements are not gate passes.

## Checks and limitations

- **Run by me:** read-only provenance, duplicate, pin and arithmetic scripts (in scratch at `/tmp/opus-r4/`, outside the repo), plus the native reads above.
- **Not run by me:** frontend, `make ci`, browser, race, FIPS, Postgres, Docker and security checks. I make no claim about them, and none about the round-4 remote CI that is still pending.
- **Gates:** keyboard, focus, contrast, states and responsive are untested.
- **Coverage:**
  - synthetic admin only, initial viewport
  - empty and unpaired Logs/Activity; unconfigured Settings/Backup

  Not covered: viewer role, populated timelines, the lower page, named themes and other browsers.
