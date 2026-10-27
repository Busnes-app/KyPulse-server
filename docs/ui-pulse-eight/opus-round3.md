# KyPulse visual8: Opus 5.5, round 3 (stage 03-after)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. I did not open any Sol report (`sol-round3.*` appeared during this review), COUNCIL.md, the workbook, the score/round/spread CSVs, the audits, README or my round 1.
- Rubric: `ky-suite-v2`, sha256 `ad351662…2cf41`. Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15. Each file total uses Decimal half-up rounding to 1 decimal. Each cohort mean is the exact mean of the rounded file totals and is never rounded into 8.
- Machine record: `opus-round3.json` (schema `ky-ui-council-scores/2`, round 3). It has all 32 rows, drift, blinding, sensitivity and the verdict. I recomputed every total and both means with a separate script.
- Scope: this is the visual8 progression gate, not 9/10 Accept. Behaviour gates are untested.

## Verdict: not satisfied

| Cohort | n | Exact mean | Blind frozen | Round 2 | Min / max | ≥ 8.0 |
|---|---:|---:|---:|---:|---|---|
| admin-console (Status, Alerts, app detail, Logs, Activity) | 20 | **8.00** | 7.98 | 7.61 | 8.0 / 8.0 | yes, at the floor |
| settings (Settings, Appearance, Backup) | 12 | **7.9667** | 7.9667 | 7.8667 | 7.8 / 8.0 | **no** |

I computed no pooled, cross-cohort or cross-reviewer mean. There is no competitor in the set, so the Ky gap is N/A.

**admin-console reaches exactly 8.00, and that result is fragile.** Two things hold it there:

- **A post-freeze correction moved it from 7.98 to 8.00.** My blind app-detail typography 7 cited only one fact: that the "Down" state word was monospace. That was a misreading. The zoomed original shows proportional glyphs (`w` wider than `o`, unlike the mono GET line under it), and my tab's computed style reads `h2.state-down` = Space Grotesk 20px/700. I made the same misreading in round 2. I changed the score to 8 because every empty note is now 13px `dr-hint`, which removes the other typography evidence I had then. The frozen values stay in each row's `post_freeze_correction`.
- **Alerts desktop moved +0.3 without a product change.** The pixels barely changed: 1.2% of pixels differ, all timestamps and underlines. In round 2 I said Reason/Delivery "crowd right", but they keep at least 100px of slack, so that part was a misread. I still cite the over-wide Time column, which I scored 7 then and 8 now. That is reviewer drift. At my round 2 judgment the cohort would be 7.97.

**settings is below 8** because of one surface, Backup desktop at 7.8 (blocker S1).

## Provenance (scripted, read-only)

| Check | Result |
|---|---|
| 32 `kypulse-03-after-*` PNGs on disk vs inventory-v3 vs manifest-03-after | Exact set equality |
| sha256, bytes and width×height vs inventory and manifest, plus the committed blob at HEAD | 0 mismatches |
| Duplicates | 0 exact, 0 near. Closest pair is activity vs logs desktop dark: 3.893% of pixels differ, max channel delta 218. |
| Manifest `overflow` | 0 for all 32 |
| Manifest provenance | `source_commit` 3984044, `uncommitted_source_sha256` null |
| 21 `source_files` pins | All match the worktree, `3984044` and HEAD `d3c069f` |
| `git diff 3984044 HEAD` | Nothing under `web/src` or `web/dist`, and nothing outside `docs/`. `web/` is clean in the worktree. |
| `web/dist/index.html` sha256 | Equals `served_index_sha256` (`0168da5f…`) |
| KyNotes brand references (`kynotes-server@1ab04e6`) | All 4 hashes match. I viewed 2 (editor desktop light, appearance mobile dark) for `ky_fit.suite_cohesion` only. They are excluded from every quality mean. |

Input hashes:

- inventory-v3: `dc445a67…d1f`
- manifest-03-after: `7d18c3c3…7f9`
- brand-references: `94764539…12de`
- native-round3, read after the freeze: `705256d8…11b3`

The parent's uncommitted `verification.json` and `checks/r3-*` files were present in the tree. I did not read them.

## Blinding

- **Read before the freeze:**
  - the AGENTS chain (workspace, product, web, web/browser, evidence)
  - the ky-suite-ui skill, rubric and design contract, and the council skill
  - the non-score fields of the inventory, manifest and brand references
  - all 32 originals, one at a time at full resolution
  - 2 KyNotes brand references
- **Freeze:** 2026-10-27T05:21:19Z, rows sha256 `d86a230e…119e`. I wrote the frozen rows to `opus-round3.json` before reading any prior report.
- **Read only after the freeze:**
  - my round 2 `.md`/`.json`
  - my stage-02 Alerts desktop light original, as a drift control
  - `native-round3.json`
  - my own native tab
- **Exposure, disclosed:**
  - The assignment brief listed the prior qualitative objections and the claimed fixes before I viewed any pixels, though no numbers.
  - `web/AGENTS.md` also describes those fixes.
  - I scored pixels and gave no credit for a reported fix, but this is priming, not a clean blind.
- **Post-freeze numeric change:** exactly one, the app-detail typography correction above (4 rows, plus `ky_fit.type_geometry`). It crosses the gate, and I have flagged that.

## Per-file scores (H,T,D,C,A,Co → total)

Light and dark score identically on every surface.

| Surface | Desktop | Mobile | Ky fit (d / m) |
|---|---|---|---|
| status | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.4 / 8.2 |
| alerts | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.4 / 8.2 |
| app-detail | 8,8,8,8,8,8 → **8.0** (frozen 8,7,8,8,8,8 → 7.9) | 8,8,8,8,8,8 → **8.0** (frozen 7.9) | 8.4 / 8.2 |
| logs | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.4 / 8.2 |
| activity | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.4 / 8.2 |
| settings | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.4 / 8.2 |
| appearance | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.4 / 8.2 |
| backup | 8,8,**7**,8,8,8 → **7.8** | 8,8,8,8,8,8 → **8.0** | 8.4 / 8.2 |

Ky fit is reported separately and never added to the total. It is the mean of palette_tokens 9, type_geometry 8, shell 9 on desktop / 8 on mobile, accent_discipline 8 and suite_cohesion 8. Suite cohesion is judged only against the KyNotes references.

The navigation is one left region. Natively there is a single `nav` "Primary" at x=16, w=207. Phone uses a horizontal primary navigation with no adjacent panel.

No dimension earns a 9. Each screen still has at least one visible slip, listed below.

## Remaining blockers (bounded, evidence-backed)

**F** = factual defect, visible and measured. **T** = taste or contract interpretation, which I do not demand.

### settings: below 8

**S1. F, Backup desktop: the hard-drive icon sits alone on its own line.**
- The icon sits above "Local copies land in …", detached from its sentence.
- Native: `svg.lucide-hard-drive` inside `p.dr-hint` computes `display:block`, at y=533.5 above the text.
- A bounded styling fix: render that icon inline with the text, or drop it. This is CSS only; it needs no copy, route or semantics change.

**S2. T/F, Backup desktop: stretched empty space in the KyRecovery card.**
- The two setup cards are side by side and stretched to equal height. The KyRecovery form ends at y=489, but the card ends at 623.5, leaving about 134px empty beside the taller key-by-hand card.
- Equal-height card pairs are a common pattern, so the void is partly taste. Together with S1, it is why density is 7.

**S3. T, Backup.**
- "None" appears twice, in red, in the card header and again in the facts. The pink strip, the warning banner and the red badge stack three alert tones on phone.
- The round 2 objections are resolved in these pixels:
  - Pair is now the enabled filled primary, before the facts.
  - The configuration badges are neutral (native: `dr-badge-muted`).

Settings and Appearance pixels are unchanged from round 2, apart from timestamps.

### admin-console: at 8.00, no visual8 blocker

**A1. T, incident strip on desktop.** It has two long underlined status-coloured links and a trailing "Alerts" link, which is redundant on the Alerts page. On app detail it stacks a second pink zone above the danger banner.

**A2. T, Activity empty message.** It still says "match these filters" when no filter is set. You kept this wording deliberately, and the next action ("Pair it in Settings", now a link) is present.

**A3. T, minor.**
- The Alerts desktop Time column is over-wide.
- Alerts has no intro line, while Status has one.
- The "Down" state is 20px/700, a deliberate step outside the 24/17/15/13 contract scale.
- The mono GET line wraps across three lines on phone.

Nothing here requires route, role, CSP, CSRF, API or handler changes.

## Round 2 objections: status in these pixels

| Round 2 objection | Round 3 evidence | Status |
|---|---|---|
| Log sources h2 at 22.5px/700 | 17px/600 (native-round3; heading sits clearly below the 24px title in pixels) | fixed |
| Apply + Add source both filled | Add source outlined (native background transparent) | fixed |
| Phone "Reason:" label in mono | Label Space Grotesk, value Plex Mono | fixed |
| Empty Checks half-column void; mixed 15/13px empty notes | Checks note inline; every hint 13px `dr-hint` (native) | fixed |
| Unpaired guidance not linked | "Pair it in Settings" → `#/settings` | fixed |
| Status rows lacked an at-rest link cue | App names visibly underlined | fixed |
| Backup next step buried; green config badges | Pair form leads with enabled filled Pair; badges `dr-badge-muted` | fixed, but the new layout introduced S1 and S2 |

## Drift against my round 2 (read after the freeze)

Files are matched one-to-one by surface, viewport and theme.

**admin-console: +0.39 mean, mostly product change.**
- Logs +1.0 / +0.5: heading, second filled action, link.
- App detail +0.7 / +0.5: inline Checks and unified notes, plus the disclosed typography correction.
- Alerts phone +0.3: label face.
- Activity +0.2: the next-action link.
- Status +0.1: underlines.
- **Alerts desktop +0.3 is reviewer drift, not product.** It is explained in the verdict section.

**settings: +0.10 mean.**
- Backup +0.3 on desktop and on phone: a product change. The new layout also cost one density point on desktop.
- **Control:** Settings and Appearance are unchanged (0.31–0.44% pixel diff, timestamps only) and moved 0.0.

Per-file causes and dimension changes are in `drift.files` in the JSON.

## Native supplemental (own incognito tab)

- **Browser:** T3 incognito, Chrome/154 UA, DPR 2. This differs from the scored Chromium 153 at scale 1.
- **Actions:** signed in as the synthetic admin, then changed the hash route only after primary navigation was present. I read computed styles and geometry on app detail and Backup.
- **No writes:** I made no pair, pin, silence, edit or theme choice. At the end, `ky_theme` was null and localStorage was empty. I closed the tab. The appearance setting was never changed, so there was nothing to restore.
- These measurements are not contrast or behaviour gate passes.

## Checks and limitations

- **Run by me:** read-only provenance scripts (hashes, pins, membership, near-duplicates and arithmetic), plus the native style reads above. The scripts live in scratch outside the repo. I opened no Sol report.
- **Not run by me:**
  - frontend, make ci, upstream unit/browser, Docker or FIPS checks
  - the SQLite/Postgres FIPS final exit and the full browser matrix, which were reported as still in flight

  I make no pass claim for any of them.
- **Gates:** keyboard, focus, contrast, states and responsive are untested.
- **Coverage:**
  - synthetic admin only, initial viewport
  - empty and unpaired Logs/Activity; unconfigured Settings/Backup
  - live ages and timestamps differ between captures

  Not covered: viewer role, populated timelines, lower page, named themes and other browsers.
