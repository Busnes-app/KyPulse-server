# KyPulse visual8: Opus 5.5 round 14 (stage14-after), focused fix confirmation

Claude Opus5.5 (`claude-opus-5-5`), primary Council reviewer. Rubric `ky-suite-v2` (sha256 `ad351662…`), design contract `392afb88…`. Runtime `f1dcb5553e2693ee997488e2a3bd4434b2bb4f1e`, evidence `7569de314217dd4b161b968dd44a1a8230df1828`. Machine record: `opus-round14.json`.

**Mode:** an explicitly unblinded focused fix confirmation of my own round 13 report. It is not a fresh independent rating or a competitive rerank. My unaffected R13 dimension scores are carried forward. I independently reassessed only the dimensions the badge change can affect, on the files where a badge is visible.

Baseline, pinned before I viewed any stage14 capture (both files unchanged):

- `opus-round13.md` sha256 `3c594b6c99c799e770ef68eafeb146f3ac73d0c4d97d1b93653900ac6e633fd9`
- `opus-round13.json` sha256 `b15bba4a1b45e27a74caa38912782474d9adf0b7e4ee51da41da5c07ceb65b63`

## Verdict

| Cohort | n | Sum | Fraction | Exact mean | At least 8 | R13 sum |
|---|---:|---:|---|---|---|---:|
| admin-console | 28 | 224.0 | 224.0/28 = 8 | 8.0 | True | 224.0 |
| settings | 12 | 96.0 | 96.0/12 = 8 | 8.0 | True | 95.8 |

**Satisfied, for the captured visual scope only.** Both exact means are exactly 8. I did not pool cohorts or round any value up into passage. The margin is zero: any single −1 on a 0.15-weight dimension would fail either cohort.

The only score movement is on Backup desktop light and dark. Typography went from 7 to 8 because the R13-1 finding is resolved. Each file moves from 7.9 to 8.0, which adds 0.2 to the settings sum. Admin-console is fully carried.

Brand fit is kept separate: 320.8/40, a mean of 8.02. By cohort: admin 224.0/28 = 8.0, settings 96.8/12 = 8.067.

## What the patch did

`f1dcb55` changes only `.badge { font-size: 12px }` to `var(--ky-type-meta)` (0.8125rem = 13px), plus `web/AGENTS.md` and the rebuilt bundle. The 72ch `.dr-hint` cap is unchanged.

- **Backup desktop:** the four fact badges (None, Not paired, 0 of 7, Daily) now render at 13px/600, the same size as the 13px labels beside them. Natively each badge is 25.5px tall, and none is clipped. The label column (`minmax(8rem, max-content)`) grew to 166.5px, so the values start about 4px further right (x=454.5). Each fact row is about 1px taller. The Back up now action row sits 3–4px lower and still meets the 900px fold, as it did in R13. Nothing wraps, overlaps or overflows.
- **Settings desktop and phone:** the Off badge is now 13px (37×25.5) and stays on the heading line, ending at x=319 of 390 on the phone. The Two-factor card grows about 1px, and later content shifts by the same amount.
- **Backup phone:** the badges sit at y≥1024.5, below the 844 fold. The captures show no badge, and the pixels match stage13 apart from the live alert-strip ages.
- **All other surfaces:** no badges. A pixel diff against stage13 shows only live chronology (alert-bar ages and times, durations, timestamps), and 11 files are pixel-identical.

## Findings

- **R13-1 resolved.** This was the only required finding from R13. The badges now use the shared 13px label size.
- **No regressions.**
- O13-1 and O13-2 are optional and unchanged: short last lines, and collapsed Logs having no filled action.
- I did not reopen the unselected taste items: Settings first-level cards, the theme grid's last row, quiet native radios, natural Backup heights and phone filter gaps.

### Newly noticed pre-existing concerns (not scored)

- **P14-1:** The Settings `db_driver` badge ("sqlite", `Settings.tsx:38`) uses `badge-accent`. That gives a non-action value orange text, an orange border and the accent-soft fill, while `web/AGENTS.md` reserves accent for actions, selection and focus. It sits below the fold in every capture: y=1008 on desktop and y=1472 on the phone. It falls outside the captured scope, so the parent decides.
- **P14-2:** The two-factor "On" badge also uses `badge-accent` (`TwoFactorCard.tsx:83`). That state is not captured, so this is visually unverified.
- **P14-3:** The Backup desktop action row meets the fold. This is informational only: it is not clipping, and no score cites it.

## Per-file scores

Columns: hierarchy, typography, density/layout, color, affordances, coherence; total; R13 total; Δ; ky_fit; basis. Confidence is high for all 40 files.

| File | Cohort | H | T | D | C | A | Co | Total | R13 | Δ | ky_fit | Basis |
|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| activity-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| activity-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| activity-filters-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| activity-filters-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| activity-filters-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| activity-filters-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| activity-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| activity-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| alerts-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| alerts-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| alerts-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| alerts-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| app-detail-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| app-detail-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| app-detail-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| app-detail-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| logs-desktop-dark | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | 0.0 | 8.0 | carried |
| logs-desktop-light | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | 0.0 | 8.0 | carried |
| logs-filters-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| logs-filters-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| logs-filters-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| logs-filters-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| logs-mobile-dark | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | 0.0 | 8.0 | carried |
| logs-mobile-light | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | 0.0 | 8.0 | carried |
| status-desktop-dark | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.2 | 0.0 | 8.0 | carried |
| status-desktop-light | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.2 | 0.0 | 8.0 | carried |
| status-mobile-dark | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.2 | 0.0 | 8.0 | carried |
| status-mobile-light | admin-console | 9 | 8 | 8 | 8 | 8 | 8 | 8.2 | 8.2 | 0.0 | 8.0 | carried |
| appearance-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 | carried |
| appearance-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 | carried |
| appearance-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 | carried |
| appearance-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 | carried |
| backup-desktop-dark | settings | 8 | **8** | 8 | 8 | 8 | 8 | 8.0 | 7.9 | +0.1 | 8.0 | reassessed |
| backup-desktop-light | settings | 8 | **8** | 8 | 8 | 8 | 8 | 8.0 | 7.9 | +0.1 | 8.0 | reassessed |
| backup-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| backup-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | carried |
| settings-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | reassessed |
| settings-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | reassessed |
| settings-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | reassessed |
| settings-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.0 | reassessed |

**Carried and reassessed dimensions**

- **Reassessed (6 files):** backup-desktop ×2 and settings-desktop/mobile ×4. On these I re-judged hierarchy, typography, density_layout and coherence on the stage14 pixels, plus brand type_geometry. Color and visible_affordances, and the other four brand dimensions, are carried from R13 because the badge colours, borders and semantics are unchanged.
- **Carried (34 files):** every dimension is carried. These files have no badge in the initial viewport, and their pixel changes are live chronology only.

The JSON records the per-file lists under `dimension_basis` and `confirmation_method`.

**Changed dimensions and why**

- Backup desktop typography, light and dark, 7→8: R13 deducted for 12px/600 badges under the 13px label size. They now render 13px in the captures and natively, and no new typography problem appears. It is not 9 because this is still the densest type mix in the set: 24/17/15/13, monospace, and a three-line hint.
- Backup desktop brand type_geometry, light and dark, 7→8: same cause.
- Reassessed but unchanged at 8: Backup hierarchy, density and coherence, because the shifts are a few pixels with no wrap, clip or overlap. Settings hierarchy, typography, density and coherence, because the Off badge stays subordinate to its 17px heading and the card grows by about 1px.

The JSON keeps six quality observations and five brand observations for every file. Two carried observations were reworded to stage14 live values (the Alerts time and the Activity phone age). One carried R13 wording error is corrected: on the Settings phone capture, Save webhook is below the fold. That wording has no score impact.

## Computer Use supplement

- **Tab:** my own Incognito tab, `tab_2q_2e419d46-7b07-427f-bf30-1831131b4cba`, opened with `reuseExistingTab:false` and closed afterwards. I never targeted the parent tab `tab_2f_…`.
- **Browser:** Chrome/154, DPR 2 (the scored originals are Chromium 153, DPR 1). OS light, `ky_theme` null, Space Grotesk loaded, service-worker controller present, f1dcb55 bundle (`index-BAnOs74O.css`).
- **Sign-in:** synthetic admin via snapshot refs. I waited for "Security check verified" before Sign In, then for `role=navigation[name='Primary']`. I made no writes.
- **Backup at 1280×900:**
  - None 50×25.5, Not paired 84.5×25.5, 0 of 7 54×25.5, Daily 49×25.5, all 13px/600 and unclipped.
  - Label column 166.5px.
  - Hints 13px with a 612px max-width.
  - One nav, overflow 0.
- **Settings at 1280×900:** Off 13px, 37×25.5; sqlite 13px Plex Mono at y=1008, below the fold. One nav, overflow 0.
- **Settings at 390×844:** Off at x 282–319, y=343. One nav, overflow 0.
- **Backup at 390×844:** all four badges 13px and unclipped at y 1024.5–1270.5, below the fold. One nav, overflow 0.
- **Agreement:** these match `native-round14.json`.
- **Limits:** OS light only. This is not keyboard, focus, contrast or state acceptance.

## Provenance and limits

- **Captures:** 40 manifest shots = 40 inventory-v14 rows = 40 stage14 PNGs on disk = 40 blobs in 7569de3. sha256, bytes and dimensions match. Overflow is 0 for all, the 40 hashes are distinct, and there are no near duplicates. Cohorts are 28 admin-console and 12 settings.
- **Source:** all 21 source pins match the worktree and the blobs at f1dcb55. The served index sha256 matches, and the worktree is clean.
- **Vendor, rubric and contract:** `check-vendor.mjs` reports "ky-ui 0.4.3: vendor files verified", and tokens.css equals producer 58626b7. The rubric sha256 matches at 639e588 and 58626b7; the contract sha256 matches at 639e588.
- **Brand references:** all 4 KyNotes references were re-extracted from 1ab04e6, their hashes match, and I viewed each at original resolution. I did not rerate Notes.
- **Root ky-ui:** the checkout already had a modified `AGENTS.md` and untracked files. I did not create or touch them.
- **Unblinded:** before viewing captures I had read the parent brief, the AGENTS file, `round13-audit.md`, my own R13 report and `native-round14.json`. I did not open any peer report. Stage13 images were used only as diffs and crops to locate the change, not as current captures.
- **Validation:** I validated only this file's arithmetic, against inventory-v14, with no glob over peer reports. I did not run `build-report.py`.
- **Untested gates:** keyboard, focus, contrast, states and responsive. These scores make no claim about full-product quality, peer consensus, CI, security or deployment.
- **Checks the parent supplied (I did not run them):** f1 vendor/build/TypeScript/make ci pass. The full browser run's exit is pending, recorded by the parent. Remote head 7569de3 requires CI and security, and no gate is claimed. MySlop returns 401, and I made no post.
