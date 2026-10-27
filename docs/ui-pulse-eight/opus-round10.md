# KyPulse Council round 10 — Claude Opus 5.5

Stage10 (`manifest-10-after.json`, `inventory-v10.json`), runtime `61e4a0c2fdcff64910c1a9d6a8f33e81cb1df910`, evidence `a8b6e511217493ef7f9622f7719c83d8075b508c`. Rubric `ky-suite-v2` (sha256 `ad351662…bf2c41`), contract `392afb88…a444d3` (ky-ui 639e5882). Machine-readable report: `opus-round10.json` (sha256 `00a5b03d116383a73abd7ad64f088a643cf2a4b3122f72476d977716b88b566e`).

## Verdict

| Cohort | n | Sum | Exact mean | Fraction | ≥ 8 |
| --- | ---: | ---: | ---: | ---: | --- |
| admin-console | 28 | 221.8 | 7.921428 | 1109/140 | False |
| settings | 12 | 96.0 | 8.0 | 8/1 | True |

**Satisfied: False.** Settings clears 8 exactly (96.0/12); the Backup patch did what it claimed. Admin-console is below 8 (221.8/28). That miss comes entirely from my blind re-weighting of retained screens, not from any product change. Not satisfied, and I do not request a score increase.

Scope: this is the visual 8 progression gate only. It is not a full-product 8 or a 9 Accept. Every behaviour gate (keyboard, focus, contrast, states, responsive) is untested, and I make no CI or security claim.

## Provenance checked

- The worktree was clean at a8b6e51, and `git diff 61e4a0c..a8b6e51 -- web` is empty. All 21 manifest source pins equal git objects at 61e4a0c. `uncommitted_source_sha256` is null. Overflow is 0 on all 40 views.
- All 40 inventory rows match on disk, by sha256, bytes and IHDR dimensions, and match the git blobs at a8b6e51. Membership equals the manifest shots: 28 admin-console and 12 settings, with 0 duplicates (`duplicate-audit-v10` is empty).
- `git diff 1fac233..61e4a0c -- web/src` touches only `Backup.tsx` and the backup-scoped rules in `theme.css`.
- A pixel diff of stage08 against stage10 shows that every retained view differs only in live timestamp and duration text. Seven retained views are pixel-identical.

## Order, freeze and disclosures

1. I read the AGENTS files, rubric, contract, validator, README and COUNCIL aggregates, then verified the manifest and inventory.
2. I read all 40 originals one at a time at original resolution and took pixel-run measurements.
3. I froze all 240 dimension choices at **2026-10-27T08:15:22.390525+00:00**. The blob sha256 is `1fc3a1091f58f623f364669d7adc9f25cc70fcbed73bde42e8401b724cd2320f`. The frozen sums were admin 221.4 and settings 96.0.
4. Only after the freeze did I read my R9 report and my R3–R8 history, the R9 and calibration audits, the Opus calibration verdicts and `native-round10.json`. I then ran the Computer Use supplement and checked the KyNotes brand references.

- **One post-freeze correction:** Logs phone collapsed (light and dark), hierarchy 7→8. My frozen note for this file read "scores similar" and carried the desktop deduction (Add source and Refresh sources are identical outlines); neither button is inside the 390x844 capture, which shows title, meta, Filters, the empty result, a divider and Log sources copy only. Without that fact the capture matches Activity mobile, which I froze at H8.
- **Priming:** The brief and README/COUNCIL aggregates (read before images) disclosed R9 cohort sums, the Backup R9 deductions, the patch contents and the Logs/App-detail optional items. My Backup rating is therefore primed by knowing what was fixed; I disclose it rather than claim blindness.
- **Peer exposure:** Accidental and post-scoring. After opus-round10.json was generated with final scores, I ran a read-only copy of build-report.py validation for my own file. Its glob also loaded sol-round10.json, which a peer had written meanwhile, and printed Sol cohort means. I never opened the Sol file and changed no score after the exposure; the frozen hash and the one disclosed correction predate it.

## Per-file scores (H Ty D C A Co → total; Ky fit separate)

| File | Cohort | H | Ty | D | C | A | Co | Total | R9 | Δ | Ky fit |
| --- | --- | -: | -: | -: | -: | -: | -: | -: | -: | -: | -: |
| activity-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| activity-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| activity-filters-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| activity-filters-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| activity-filters-mobile-dark | admin-console | 8 | 8 | 7 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.2 |
| activity-filters-mobile-light | admin-console | 8 | 8 | 7 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.2 |
| activity-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| activity-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| alerts-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| alerts-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| alerts-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| alerts-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| app-detail-desktop-dark | admin-console | 8 | 8 | 7 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.4 |
| app-detail-desktop-light | admin-console | 8 | 8 | 7 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.4 |
| app-detail-mobile-dark | admin-console | 8 | 8 | 7 | 8 | 7 | 8 | 7.7 | 8.0 | -0.3 | 8.2 |
| app-detail-mobile-light | admin-console | 8 | 8 | 7 | 8 | 7 | 8 | 7.7 | 8.0 | -0.3 | 8.2 |
| appearance-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.4 |
| appearance-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.4 |
| appearance-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| appearance-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| backup-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | 0.2 | 8.4 |
| backup-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | 0.2 | 8.4 |
| backup-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | 0.2 | 8.2 |
| backup-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | 0.2 | 8.2 |
| logs-desktop-dark | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | 0.0 | 8.4 |
| logs-desktop-light | admin-console | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | 0.0 | 8.4 |
| logs-filters-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| logs-filters-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| logs-filters-mobile-dark | admin-console | 8 | 8 | 7 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.2 |
| logs-filters-mobile-light | admin-console | 8 | 8 | 7 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.2 |
| logs-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| logs-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| settings-desktop-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| settings-desktop-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.4 |
| settings-mobile-dark | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| settings-mobile-light | settings | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | 0.0 | 8.2 |
| status-desktop-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.4 |
| status-desktop-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.4 |
| status-mobile-dark | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.2 |
| status-mobile-light | admin-console | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.2 | -0.2 | 8.2 |

I wrote concrete observations for each dimension of each file, with theme-specific colour notes, in `opus-round10.json`. The key ones:

- **activity-filters-mobile density_layout 7:** Same 8px inter-group stacking as Logs filters mobile (Activity app/Actor pair, Outcome, two dates); result ends ~822px inside 844px but the block reads cramped against the page rhythm (7).
- **app-detail-desktop density_layout 7:** Section gaps are ~32px, but Checks keeps its empty hint inline on the heading row while KyYard and Recent logs stack the hint below, and Alert history sits ~8px above its table versus ~16px heading-to-content elsewhere; a scanning user notices the rhythm break (7).
- **app-detail-mobile density_layout 7:** Same inline Checks irregularity as desktop at 390px ("Checks  No checks reported yet." on one row while Settings stacks); otherwise 16-32px steps (7).
- **app-detail-mobile visible_affordances 7:** The silence grid stretches buttons to 175px/358px but labels stay left-aligned (live: 17px left vs ~90px/220px right padding), so they read like list rows or fields rather than buttons (7).
- **backup-desktop density_layout 8:** Setup columns end at natural heights (Pair row 456, Pin key 487) with the facts rule 32px below the taller one; facts rows ~42-62px even; operations now read heading 16px -> paragraph 12px -> buttons (live). Explanation precedes actions like the setup columns above.
- **backup-mobile density_layout 8:** Notice-to-KyRecovery and Pair-to-second-setup gaps are both 32px (live 335->367, 655->687); heading-to-hint ~24px, label-to-field even; the notice now reads as page-level, above both paths.
- **logs-desktop hierarchy 7:** Title, two meta lines, outlined Filters, "No logs found.", divider, then Log sources with Add source and Refresh sources as identical outlined buttons; the section's create action is not distinguished from its refresh, and nothing on the collapsed page is primary (7).
- **logs-filters-mobile density_layout 7:** Five field rows stacked with 8px between groups and ~4-6px label-to-field (live: field bottom 481.5 to next label 489.5); Filters to first label 8px, To field to Apply 8px. Grouping still reads, but the block is tighter than every other gap on the page (16-24px) and feels cramped (7).

## Drift against my R9 (stage08 images matched by surface, theme, viewport and state)

Admin moved from 224.4 to 221.8, and settings from 95.6 to 96.0. No file moved by |Δ| ≥ 0.5; the largest change is −0.3, on App detail phone.

- **Actual Backup patch effect, 4 files, +0.2 each:** Backup 4 files: density 7 -> 8 each (+0.2 total each, +0.8 settings sum). Both R9 Backup deductions are gone in the pixels: desktop explanation now precedes its buttons; phone notice/setup and setup/setup gaps are both 32px.
- **Reviewer variation, 16 retained files:** 14 retained admin files and 2 Appearance desktop files moved down by one dimension on pixels that differ from stage08 only in live timestamp text (7 views are byte-for-byte pixel identical). These are my R10 blind judgement differing from my R9 calibrated judgement; they are not product regressions. My blind pass largely returned to my R8 positions (Status H8 as in R3-R7; app-detail and phone-filter density 7 as in R7/R8; Appearance desktop density 8 as in R3/R4/R6/R7).
  - Status, 4 files, H 9→8. The incident strip carries the first red emphasis above the page. I name no defect.
  - App detail, 4 files, D 8→7. The inline Checks hint and the tight Alert history heading gap are the same irregularity R9 recorded but scored 8.
  - App detail phone, 2 files, A 8→7. Stretched silence buttons keep left-aligned labels. R9 noted the same thing and scored it 8.
  - Logs and Activity filter phones, 4 files, D 8→7. The 8px inter-group stacking reads cramped. R9 scored it 8, citing the rejected alternatives; the rubric judges captured pixels.
  - Appearance desktop, 2 files, D 9→8. The grid is unchanged, and I name no defect.
- **No gain** is attributed to live chronology, new coverage or rubric variation. Every retained-screen change is a judgement difference on identical layout. None of it is a product change.

**Reading the result:** On admin-console, my R9 sum was 224.4 and reviewer variation on retained pixels removes 2.6. That gives 221.8. A3 is unchanged from R9 (Logs desktop H7). A1, A2 and T1 are observations R9 also recorded but scored 8. The admin miss is a weighting disagreement with my own previous round on pixels that did not change. The parent should treat it as reviewer variation and decide whether A1-A3 merit implementation. I predict no score after any fix.

## Remaining actionable visual defects (separate from taste)

- **A1 App detail phone** (2 files, visible_affordances 7): Silence buttons stretch to grid width with left-aligned labels, so they read like rows or fields. Fix: Centre the content of stretched silence buttons (justify-content:center), or keep them content-width as on desktop. Done when: At 390x844 label left and right padding are equal for all three silence buttons.
- **A2 App detail desktop and phone** (4 files, density_layout 7): The empty Checks hint is inline on the heading row while KyYard/Recent logs stack their hints. The Alert history heading sits ~8px above its table versus ~16px heading-to-content elsewhere. Fix: Stack the Checks hint under its heading with the KyYard spacing, without restoring the old empty half-column; give Alert history the same heading gap. Done when: Heading-to-hint/content gaps are equal across Checks, KyYard, Recent logs and Alert history at 1280 and 390.
- **A3 Logs desktop collapsed** (2 files, hierarchy 7): Add source and Refresh sources are identical outlines. Fix: Render Refresh sources as contract tertiary (text-only), keeping Apply the only filled control when expanded. Done when: Add source visibly outranks Refresh at 1280 in both Busnes themes.

Taste or constrained, not defects:

- **T1 Logs/Activity filters phone:** 8px inter-group stacking reads cramped. Widening it to 16px pushes the Activity result (~822px) past the 844px fold, and the paired-date alternative was already rejected for truncation. This is my judgement, not a defect with a clean fix; R9 scored it 8.
- **T2 Status:** The incident strip carries the first red emphasis above the page; scored H8 not H9. This is not a defect.
- **T3 Appearance desktop:** Density 8 vs R9 9 on an unchanged grid; no defect named.

## Computer Use supplement (after the freeze)

- I used own Incognito tab tab_2i_2e419d46-7b07-427f-bf30-1831131b4cba, opened with reuseExistingTab:false and closed; parent tab tab_2f untouched. The browser was Chrome/154, DPR2 (differs from scored Chromium153 DPR1); OS light only; no saved theme (localStorage keys: none).
- Sign-in: admin / disposable fixture password; waited for "Security check verified." before Sign In, then for the Primary navigation landmark (accessible name is "Primary"; the first wait on "Primary navigation" timed out, snapshot then confirmed the signed-in shell).
- Writes: none: no pairing, key pin, configuration, content or theme change; Logs Filters details opened then closed.
- Backup desktop: the heading ends at 791.5, the description runs 807.5–846.5 and the actions 858.5–898.5. The action row has an 8px bottom margin. There is 1 nav and 0 overflow.
- Backup phone: the notice ends at 335 and the first setup starts at 367 (32px). The first setup ends at 655 and the second starts at 687 (32px). The description comes before the actions. There is 1 nav and 0 overflow.
- App detail phone: the silence buttons have 17px of padding on the left and about 90px or 220px on the right. Logs phone filters have 8px gaps between groups.
- Matches native-round10.json desktop and phone figures exactly. This supplements the originals; it is not acceptance.

## Ky brand fit (separate, never added)

For desktop files I scored palette 9, type geometry 8, shell 9, accent 8, cohesion 8, which averages 8.4. Phone files score shell 8 and average 8.2. Pixels sampled on Status match the ky-ui 0.4.3 tokens exactly. The four pinned KyNotes references were verified against git and share the tokens, the nav treatment and the Appearance picker. Brand fit is separate from quality.

## Confidence and caveats

Confidence is high for every file: they are clear full-resolution PNGs without overlays. The captures show only the initial viewport. Other roles, populated Logs and Activity, configured Backup, loading and error states, focus, keyboard behaviour and computed contrast are all unverified. The full production browser suite was still pending when I wrote this, and I did not rerun any product checks.
