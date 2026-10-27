# KyPulse visual8: Opus 5.5, round 2 (stage 02-after)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`), independent primary reviewer. I did not read `sol-round1.*`, `COUNCIL.md`, the workbook or any score CSV.
- Rubric: `ky-suite-v2`, sha256 `ad351662…2cf41`. Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15. Each file total uses Decimal half-up rounding to 1 decimal. Each cohort mean is taken over the rounded file totals; the exact value is kept and never rounded up into 8.
- Machine record: `opus-round2.json` (schema `ky-ui-council-scores/2`, round 2). It covers all 32 files. Each row has sha256, size, dimensions, six integer scores, observations, total, `duplicate_of`, `ky_fit`, confidence and caveat.
- Scope: this is the visual8 progression gate, not 9/10 Accept. Behaviour gates are untested.

## Verdict: not satisfied

| Cohort | n | Exact mean | Min | Max | ≥ 8.0 |
|---|---:|---:|---:|---:|---|
| admin-console (Status, Alerts, app detail, Logs, Activity) | 20 | **7.61** | 7.0 | 7.9 | no |
| settings (Settings, Appearance, Backup) | 12 | **7.8667** | 7.5 | 8.0 | no |

I computed no pooled or cross-reviewer mean. There is no competitor in this set, so the Ky gap is N/A.

Surface diagnostics within each cohort (not rankings):

- admin-console: Logs 7.25, App detail 7.40, Alerts 7.70, Activity 7.80, Status 7.90
- settings: Backup 7.60, Appearance 8.00, Settings 8.00

## Provenance (scripted, read-only)

| Check | Result |
|---|---|
| 32 `kypulse-02-after-*` PNGs on disk = inventory-v2 = manifest-02-after | exact set equality. No extras or omissions. |
| sha256, bytes, width and height vs inventory and manifest | 0 mismatches |
| Duplicates | 0 exact, 0 near. Closest pair: activity vs logs desktop light, 4.56% of pixels differ. |
| Manifest `overflow` | 0 for all 32 |
| 19 `source_files` pins | all match `HEAD` = `f4fa422` and the worktree; product tree clean |
| Manifest `source_commit` d3e823d + `uncommitted_source_sha256` 178f377b… | **Recorded mismatch.** It predates f4fa422: 13 of 19 pins differ from d3e823d. The pinned files are the authority, and they match f4fa422. I could not recompute the uncommitted hash because its method is not recorded, so it stays unverified. |
| Files changed in f4fa422 but not pinned | ky-ui `tokens.css`/`themes.js`/`themes-browser.js` are bound by the pinned `VERSION` hashes (`node src/ky-ui/check-vendor.mjs` → "ky-ui 0.4.3: vendor files verified"). `dist/assets/*` are named by content hash from the pinned `index.html`. `capture.mjs` and `web/AGENTS.md` are not rendered. |
| Live `GET /` at :18422 | sha256 `e651cf9b…` = `served_index_sha256` |
| Brand refs | all 4 KyNotes hashes match at `1ab04e6`. I viewed 2 of them (editor desktop light, appearance mobile dark). They are excluded from all means. |

Input hashes: inventory-v2 `7b3e34bb…e690`; manifest-02-after `9ce59277…35b9`; brand-references `94764539…12de`; native-round2 `20f45af4…9020`; rubric `ad351662…2cf41`.

HEAD moved during this review. Validation ran at `f4fa422`, when the evidence was untracked. The parent then committed `7c1961e..135ee69`. I re-checked at `135ee69`:

- `web/src` and `web/dist` are byte-identical to `f4fa422`.
- All 19 pins still match.
- The committed blobs of the 32 PNGs and the four evidence JSONs equal the hashes above.
- `sol-round2.*` appeared while I was writing. I did not open it.

## Blinding

- **Opened before freeze:** the AGENTS chain, the skills, the rubric, the non-score fields of the inventory/manifest/brand refs, all 32 originals at full resolution, and zoom crops of 4 originals.
- **Freeze:** rows frozen 2026-10-27T04:57:20Z, rows sha256 `5e4dde03…cf70`.
- **Opened only after the freeze:** my round1 report, `native-round2.json`, the ky-ui diff, the KyNotes images and my native tab.
- **Never opened:** stage01 images, Sol reports, COUNCIL/workbook, scores/rounds/spreads CSVs, README.
- **Disclosed correction:** my frozen app-detail rows said the "Down" state word was monospace. Zoomed pixels and the computed style (`.banner h2.state-down`, Space Grotesk 20px/700) show it is not. I corrected the observation text on those 4 rows after the freeze and kept the numbers. The other typography/coherence evidence still holds: the empty-note sizes mix 15px and 13px. The frozen text is preserved in `post_freeze_correction`.

## Per-file scores (H,T,D,C,A,Co → total)

Light and dark score identically everywhere. The only difference between them is the native dark radio, which I judged legible.

| Surface | desktop (light = dark) | mobile (light = dark) | Ky fit |
|---|---|---|---:|
| status | 8,8,8,8,7,8 → **7.9** | 8,8,8,8,7,8 → **7.9** | 8.0 |
| alerts | 8,8,7,8,8,7 → **7.7** (7.65) | 8,7,8,8,8,7 → **7.7** | 8.0 / 7.8 |
| app-detail | 7,7,7,8,8,7 → **7.3** | 8,7,7,8,8,7 → **7.5** | 7.8 |
| logs | 6,7,7,7,8,7 → **7.0** (6.95) | 7,7,7,8,8,8 → **7.5** (7.45) | 7.6 |
| activity | 8,8,7,8,8,8 → **7.8** | 8,8,7,8,8,8 → **7.8** | 8.0 |
| settings | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.0 |
| appearance | 8,8,8,8,8,8 → **8.0** | 8,8,8,8,8,8 → **8.0** | 8.2 |
| backup | 7,8,8,7,8,7 → **7.5** | 7,8,8,7,8,8 → **7.7** (7.65) | 7.8 |

Ky fit is scored on palette, type/geometry, shell, accent and cohesion. It is reported separately and never added to the total. Suite cohesion is judged only against the KyNotes references. The shell is uniform: one 240px left navigation region on desktop (native: a single `nav` at x=16, w=207) and horizontal primary navigation on phone, with no adjacent panel.

## Remaining blockers (bounded, evidence-backed)

**F** = factual defect visible or measured. **T** = taste or contract-interpretation distinction.

### admin-console

1. **F, Logs: "Log sources" heading renders at the browser default.**
   - Native measurement: `h2` is 22.5px/700 against the 24px/600 h1. Pixels match: cap heights 16 vs 17px.
   - Cause: `LogSources.tsx:61` puts an `<h2>` inside `.dr-section`, but only `.dr-section h3` is styled (`theme.css:312`).
   - The Activity "Failed sign-in bursts" `h2` (`Activity.tsx:33`) has the same gap. It is unscored here because that state was not captured.
2. **T (contract), Logs: Apply and Add source are both filled accent** in one view (native: both `rgb(191,63,24)`). The contract reserves one filled primary per view. My round1 fix list only demoted Clear and Refresh sources, so this residual was my own omission there, not a regression.
3. **F, Alerts phone: the "Reason:" label renders in IBM Plex Mono.**
   - The other row labels are Space Grotesk.
   - Cause: `.events td::before` (`theme.css:550`) inherits the font of the `.dr-mono` cell. Setting the label pseudo-element's font family to the body face would fix it.
4. **T, App detail desktop: weak section hierarchy.**
   - Six equal-weight sections stack with tight gaps.
   - The empty "Checks" half leaves a void beside the Settings facts.
   - "No retained logs for this app." is 15px plain text, while the sibling hint is 13px `dr-hint` (natively measured).
5. **T, Logs/Activity empty states.**
   - The guidance "Pair it in Settings" is plain text. Native check: there is no `#/settings` link.
   - Activity reports "No audit events match these filters" when no filter is set.
   - These are visible empty-state quality slips. Wording changes need owner intent, so I do not demand them.
6. **T, minor.**
   - Desktop Alerts table: the Time column is much wider than its content.
   - Status rows show no at-rest link cue. Natively, the anchor reports underline, but it is not visible on the block children.

### settings

7. **T (owner-deferred), Backup badges.**
   - "0 of 7" and "Daily" use `badge-success` green (native `rgb(38,116,66)`) under "No backups are being made".
   - This is the main colour drag on the only settings surface below 8. You deferred it deliberately, so I record it and do not demand it.
8. **T, Backup hierarchy.**
   - In the unconfigured state every task action is disabled, and Refresh is the only enabled header action.
   - The actual next step, pinning the key, is below the initial viewport.
   - Flat fact rows followed by cards and pill chips put several vocabularies on one page.

Nothing here requires route, role, CSP or CSRF changes.

## Round1 findings: status in these pixels

| Round1 finding | Round2 evidence | Status |
|---|---|---|
| Phone AlertBar too tall (134px) | 65px natively, one line per problem with relative age and cause | fixed |
| Competing filled actions | Settings 3→1, Activity 2→1, Logs 4→2 | fixed, except the Logs residual (blocker 2) |
| Logs/Activity title scale and filter geometry | h1 24px/600; 3-column grid, no orphan field; full-width on phone | fixed. The new defect is the h2 (blocker 1). |
| UA-blue links | ink, underlined (Alerts, Settings intro) | fixed |
| Phone alert table overflow | labelled rows, document overflow 0 | fixed, except the mono label (blocker 3) |
| Disabled faded primaries | neutral `rgb(242,239,231)` fill, muted ink, opacity 1 | fixed |
| Squeezed Backup heading | card heading full width, actions stacked | fixed |
| Decorative Settings orange icons | neutral grey icons; muted "Off" chip | fixed |
| Duplicate nav icons | Status, Activity, Logs and Backup now distinct | fixed |
| Degraded colour hard-coded / danger-red in AlertBar | `--warning` consumed: light `#875b12`; dark `rgb(224,183,117)` on panel 7.76:1 (native, Busnes Dark) | fixed |

## Drift against my round1 (read after freeze)

The files match one-to-one by route, viewport and theme, but the images differ, so most of each delta is product change, not rubric drift. The rubric is unchanged. Mean per-file delta is +1.2.

- **Admin-console** moved from 6.3 to 7.61.
  - Alerts phone +2.5, Logs phone +2.2, Activity +1.9/+2.1, Logs desktop +1.7, Alerts desktop +1.6: the button vocabulary, title, link, filter and phone-row fixes above.
  - Status +0.3 to +0.7: warning tone, distinct icons, compact phone bar.
  - App detail phone +0.5: the compact bar returns the first viewport to the page.
  - App detail desktop −0.2 (correction): I now weigh the six equal sections and the empty Checks column more heavily in hierarchy (8→7).
- **Settings** moved from 7.0 to 7.8667.
  - Settings +1.6/+1.8: one primary, neutral icons, ink link.
  - Backup phone +1.4: unsqueezed card and compact bar. Backup desktop +0.4: neutral disabled buttons.
  - Appearance +0.0 to +0.3.
- **Control:** Appearance's source is unchanged, and it drifted only +0.0 to +0.3. That comes from the compact phone bar (density 7→8) and a correction: I now read the dark unselected radios as legible (7→8). This small drift on an unchanged surface supports reading the large moves elsewhere as product change, not reviewer inflation.

## ky-ui `a7e93b7` (0.4.3) warning diff

The change sets `--warning` to `var(--ky-warning-<scheme>)` for named palettes and clears it with the other `THEME_VARIABLES`. Busnes and System keep the tokens.css alias. The dark warning changes from `#d8a657` to `#e0b775`. The Pulse vendored `tokens.css`/`themes.js` hashes equal the upstream blobs at `a7e93b7`.

I found no runtime defect. Two low, non-blocking items:

- `themes.d.ts:39` still types `THEME_VARIABLES` as `Record<keyof ThemePalette, string>`, but the runtime object now also has `warning`, which is not a `ThemePalette` key. No Pulse code iterates it.
- The `THEME_VARIABLES` comment ("properties each palette key sets") no longer holds for `warning`, which is set separately. A palette that someday defined `warning` would be silently overridden, unlike `danger`/`success`, which use `??`.

I did not rerun upstream unit14/browser36.

## Native supplemental (own incognito tab)

- **Browser:** T3 native Chrome 154 UA, DPR 2. This differs from the scored Chromium 153 at scale 1.
- **Actions:** sign-in as the synthetic admin, hash navigation, emulated dark, 390×844 resize and computed-style reads only. I made no writes and chose no theme (`ky_theme` stayed null; localStorage empty).
- **Cleanup:** I restored appearance to system and closed the tab.
- None of these measurements is a contrast or behaviour gate pass.

## Checks and limitations

- **Run by me:** `node src/ky-ui/check-vendor.mjs` passed. The provenance script lives outside the repo in scratch.
- **Not run by me:** I did not run or observe frontend, make ci, FIPS, Docker, browser, Postgres or repeatable-build checks. Of these, the final browser, Postgres and repeatable-build runs were reported to me as still in flight, and I make no pass claim for them.
- **Gates:** keyboard, focus, contrast, states and responsive are all untested.
- **Coverage:** synthetic admin only, initial viewport, empty/unpaired Logs/Activity, unconfigured Settings/Backup. Live ages and timestamps differ between captures.
- **Not covered:** viewer role, dense or populated data, lower page, named themes, WebKit.
