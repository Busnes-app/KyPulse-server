# KyPulse visual8: Opus 5.5, round 9 (stage 08-after), calibrated reassessment

> **EXPLICITLY UNBLINDED.** Round 9 reassesses the same 40 round-8 originals, which are unchanged byte for byte. I read every prior before I wrote my scores: my own round-8 scores (222/28 and 95/12, unmet), Sol's (240/28 and 102/12, satisfied), both calibration advisories and the parent audit.
>
> - This round is not a product iteration, not a blind review, not a threshold repair and not a requested score increase.
> - Every round-8 to round-9 change is a review calibration. **None is a product improvement.**
> - Round 8 stays frozen.

- **Reviewer:** Claude Opus 5.5 (`claude-opus-5-5`), primary. No subagents, no Gemini.
- **Machine record:** `opus-round9.json`. Schema `ky-ui-council-scores/2`, round 9, `unblinded: true`, `calibration_of: 8`. It holds all 40 files, the same membership as `inventory-v8.json`.
- **Rubric:** `ky-suite-v2`, sha256 `ad351662…2cf41`.
  - Weights: hierarchy .20, typography .15, density_layout .20, color .15, visible_affordances .15, coherence .15.
  - Each file total is a Decimal sum, rounded half-up to one decimal.
  - Cohort means are exact means of those totals. They are never rounded into 8.
- **Design contract:** the original `392afb88…a444d3`, recovered with `git show 639e5882:skills/ky-suite-ui/references/design-contract.md`. I did not use the dirty ky-ui working root.
- **Scope:** visual8 progression only. Every behaviour gate is **untested**. This is not full-product 8 or 9.

## Verdict: not satisfied

| Cohort | n | R8 sum | R9 sum | Exact mean | Fraction | ≥ 8 |
|---|---:|---:|---:|---:|---|---|
| admin-console | 28 | 222.0 | 224.4 | **8.0142857…** | 561/70 | yes |
| settings | 12 | 95.0 | 95.6 | **7.9666…** | 239/30 | **no**, 0.4 total points short |

**Settings is the remaining cohort below 8, by a small margin.**

- I found **no defensible required edit**. What is left is one judgement I keep: the Backup density deduction, on 4 files. It is my most stable Opus deduction (Backup desktop D7 in rounds 3, 4, 7 and 8, and Backup phone D7 since the flat layout in round 7).
- An optional spacing change would address it (P1 and P2 below). It is polish, not a defect that blocks a task.
- I am not changing the numbers to clear 8. The parent should report the settings gate as unresolved rather than reroll.

Other limits:

- There is no pooled, cross-cohort or cross-reviewer mean, and no threshold rounding or waiver.
- There is no competitor in the source set, so the Ky gap is N/A. The 4 KyNotes references are for brand only.
- Ky fit is unchanged from round 8 and is reported only.
- Gemini stays deferred, because neither primary has satisfied both cohorts.

## Method and order

1. I read the AGENTS chain, the original contract, the rubric, the inventory, the manifest, the duplicate audit, `COUNCIL.md` and `build-report.py`. None of these has per-file scores.
2. I viewed **all 40 originals one at a time at original resolution** (1280×900 and 390×844, light and dark, both Logs/Activity disclosure states). I took my own notes before opening any per-file prior. I used no contact sheet.
3. I then read `opus-round8.md/json`, `sol-round8.md/json`, both calibration advisories, the parent calibration audit, and my own round 3–8 history for the disputed files.
4. I made two 4× crops:
   - **The app-detail "Down" line.** My first pass had misread "Down" as monospace. The crop shows bold proportional Space Grotesk; only the GET diagnostic is monospace, which is fixed-width data the contract allows. I carried no typography deduction.
   - **The dark Appearance radio.** The disc is `#3b3b3b` and the ring is visible.
5. I chose new integers, then computed totals and means with a script (`Decimal`/`Fraction`). A read-only copy of the `build-report.py` assertions passes.
6. Directory listings showed `sol-round9.*` appear during this round. I did not open them.

## Changes from round 8, by dimension (all calibration, none from code)

Two causes are used:

- **Correction:** I applied a documented rubric or contract reading to the image.
- **Residual variation:** I am reverting my own round-8 shift on pixels that did not change.

**Appearance dark, desktop and phone (2 files): visible_affordances 7→8. Correction.**

- What the image shows: unselected native radios are grey discs with a visible ring. My own pixel sample gives 3.52:1 for the ring on the `#1f2b2e` card; the light ring is 3.84:1.
- Match system is unambiguous: it has an orange card border and a filled orange radio, and every option is a whole labelled card.
- The interior is dimmer than the hollow light radio. That is a real but minor slip, and it fits within an 8. The rubric says to judge each theme against its own background, with no automatic dark penalty.
- I keep the parent's point: subjective darkness could justify a different score. I do not think this one changes how the state reads at a glance.
- I do **not** rely on the contract's "native indicator" clause, which the Opus calibration cited. That clause is about focus.
- Totals: desktop 8.1→8.2, phone 7.9→8.0.

**Logs expanded desktop (2 files): hierarchy 7→8. Correction.**

- Apply is the single filled action. Log sources sits behind a divider and its own heading.
- Add source and Refresh sources share the same outline. That is a small emphasis slip in a secondary region.
- I asked for Add source to be demoted in round 2, to remove a second filled primary. Penalising that outcome would ask for two primaries again.
- Total 7.8→8.0.

**Logs and Activity expanded phone (4 files): density_layout 7→8. Correction.**

- Rows are an even 8px apart. The 2+1+1+1 grid follows the content: paired dates clipped at 390px and were rejected, and a single column pushes the result past 844px. The result line stays inside the fold.
- Round 8 already called this "borderline, close to taste".
- Total 7.8→8.0.

**App detail, desktop and phone (4 files): coherence 7→8 is a correction; density_layout 7→8 is residual variation.**

- **Coherence:** the danger-tinted banner, the flat fact rows and the bordered events table serve three semantic roles. They share one vocabulary: the table is the same events component as Alerts, the outlines, radius and type match, and nothing is nested. The rubric criterion is stray styles or nested chrome, and none is visible.
- **Density:** desktop section gaps measure about 32–35px throughout. The only irregularity is the inline empty Checks row, which was the round-3 fix for an empty half-column. I scored D8 on these pixels in rounds 3–6.
- Total 7.7→8.0.

**Backup desktop (2 files): hierarchy 7→8. Correction.**

- Pair is the only filled action. Refresh is a quiet outlined header action, two operations are visibly disabled, and Sign out belongs to the shell.
- Counting buttons does not measure competing emphasis. The repeated "Back up now" heading is copy polish and needs owner consent.
- Total 7.6→7.8.

**No file moves by 0.5 or more.** The largest change is +0.3, on app detail. In sums, admin-console moves 222.0→224.4 and settings 95.0→95.6. No product changed after `1fac233`: the diff from `1fac233` to `a1cc471` under `web/src` and `web/dist` is empty.

## Deductions I keep (still justified on the images)

**Logs collapsed desktop (2 files): H7.**

- No filled action is required in an empty view. My deduction is different: in view, the only task group is Add source next to Refresh sources, and the create action gets no more weight than a refresh.
- The contract provides a tertiary (text-only) kind that would fix this without adding a second filled action.
- On the phone, collapsed, that pair is below the fold, so the phone keeps H8.

**Backup desktop (2 files): D7.**

- The setup columns end about 31px apart (y456 vs y487) above the facts divider.
- The Back up now row puts its explanation paragraph **below** its three buttons. That inverts the heading → description → action order used by the setup columns and by Settings.

**Backup phone (2 files): D7.**

- The page-level notice names both paths, but it sits about 16px above the KyRecovery heading. The "Recovery key by hand" section starts about 48px below Pair.
- So the notice reads as belonging to KyRecovery only.

## Remaining issues

**Required:** none. No evidence-backed defect remains.

Optional polish. None of these is required:

| ID | Surface | Change | Completion criterion |
|---|---|---|---|
| P1 | Backup phone | Give the page-level notice the same gap to the first heading as the gap between sections. | At 390×844, light and dark, the two gaps are equal. |
| P2 | Backup desktop | Put the Back up now explanation above its action row. A heading rename needs owner copy approval. | At 1280, the paragraph comes before the buttons. |
| P3 | Logs collapsed | Make Refresh sources tertiary, per the contract. | Add source outranks Refresh at 1280 and 390, in both Busnes themes. Apply stays the sole filled action when expanded. |
| P4 | App detail | Stack the empty Checks hint under its heading. | The hint sits below the heading, with the same gap as KyYard and no empty half-column. |

P1 and P2 are the only items that touch the settings deduction I keep. Whether to make them, or to accept the unresolved settings gate, is the parent's and owner's call. I do not predict a score.

**Taste, not scored down:**

- the persistent incident strip;
- the scrolled phone navigation clipping a fragment at the left edge, which is the horizontal-scroll cue;
- Settings keeps section cards while Backup is flat;
- danger and accent are close in hue (an upstream token matter, Ky fit only);
- the "Settings" page title vs the "Settings & DB" navigation label.

## Per-file scores

H/T/D/C/A/Co = hierarchy / typography / density_layout / color / visible_affordances / coherence. Every row is high confidence, with no overlays or artifacts. The caveat for every row: initial viewport only; interaction, focus, measured contrast and lower content are not visible. The JSON has one concrete observation per dimension for every file.

| Original | Cohort | H/T/D/C/A/Co | R8 | R9 | Δ | Ky fit |
|---|---|---|---:|---:|---:|---:|
| [activity-desktop-dark.png](screenshots/kypulse-08-after-activity-desktop-dark.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [activity-desktop-light.png](screenshots/kypulse-08-after-activity-desktop-light.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [activity-filters-desktop-dark.png](screenshots/kypulse-08-after-activity-filters-desktop-dark.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [activity-filters-desktop-light.png](screenshots/kypulse-08-after-activity-filters-desktop-light.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [activity-filters-mobile-dark.png](screenshots/kypulse-08-after-activity-filters-mobile-dark.png) | admin-console | 8/8/8/8/8/8 | 7.8 | 8.0 | +0.2 | 8.2 |
| [activity-filters-mobile-light.png](screenshots/kypulse-08-after-activity-filters-mobile-light.png) | admin-console | 8/8/8/8/8/8 | 7.8 | 8.0 | +0.2 | 8.2 |
| [activity-mobile-dark.png](screenshots/kypulse-08-after-activity-mobile-dark.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |
| [activity-mobile-light.png](screenshots/kypulse-08-after-activity-mobile-light.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |
| [alerts-desktop-dark.png](screenshots/kypulse-08-after-alerts-desktop-dark.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [alerts-desktop-light.png](screenshots/kypulse-08-after-alerts-desktop-light.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [alerts-mobile-dark.png](screenshots/kypulse-08-after-alerts-mobile-dark.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |
| [alerts-mobile-light.png](screenshots/kypulse-08-after-alerts-mobile-light.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |
| [app-detail-desktop-dark.png](screenshots/kypulse-08-after-app-detail-desktop-dark.png) | admin-console | 8/8/8/8/8/8 | 7.7 | 8.0 | +0.3 | 8.4 |
| [app-detail-desktop-light.png](screenshots/kypulse-08-after-app-detail-desktop-light.png) | admin-console | 8/8/8/8/8/8 | 7.7 | 8.0 | +0.3 | 8.4 |
| [app-detail-mobile-dark.png](screenshots/kypulse-08-after-app-detail-mobile-dark.png) | admin-console | 8/8/8/8/8/8 | 7.7 | 8.0 | +0.3 | 8.2 |
| [app-detail-mobile-light.png](screenshots/kypulse-08-after-app-detail-mobile-light.png) | admin-console | 8/8/8/8/8/8 | 7.7 | 8.0 | +0.3 | 8.2 |
| [logs-desktop-dark.png](screenshots/kypulse-08-after-logs-desktop-dark.png) | admin-console | 7/8/8/8/8/8 | 7.8 | 7.8 | +0.0 | 8.4 |
| [logs-desktop-light.png](screenshots/kypulse-08-after-logs-desktop-light.png) | admin-console | 7/8/8/8/8/8 | 7.8 | 7.8 | +0.0 | 8.4 |
| [logs-filters-desktop-dark.png](screenshots/kypulse-08-after-logs-filters-desktop-dark.png) | admin-console | 8/8/8/8/8/8 | 7.8 | 8.0 | +0.2 | 8.4 |
| [logs-filters-desktop-light.png](screenshots/kypulse-08-after-logs-filters-desktop-light.png) | admin-console | 8/8/8/8/8/8 | 7.8 | 8.0 | +0.2 | 8.4 |
| [logs-filters-mobile-dark.png](screenshots/kypulse-08-after-logs-filters-mobile-dark.png) | admin-console | 8/8/8/8/8/8 | 7.8 | 8.0 | +0.2 | 8.2 |
| [logs-filters-mobile-light.png](screenshots/kypulse-08-after-logs-filters-mobile-light.png) | admin-console | 8/8/8/8/8/8 | 7.8 | 8.0 | +0.2 | 8.2 |
| [logs-mobile-dark.png](screenshots/kypulse-08-after-logs-mobile-dark.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |
| [logs-mobile-light.png](screenshots/kypulse-08-after-logs-mobile-light.png) | admin-console | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |
| [status-desktop-dark.png](screenshots/kypulse-08-after-status-desktop-dark.png) | admin-console | 9/8/8/8/8/8 | 8.2 | 8.2 | +0.0 | 8.4 |
| [status-desktop-light.png](screenshots/kypulse-08-after-status-desktop-light.png) | admin-console | 9/8/8/8/8/8 | 8.2 | 8.2 | +0.0 | 8.4 |
| [status-mobile-dark.png](screenshots/kypulse-08-after-status-mobile-dark.png) | admin-console | 9/8/8/8/8/8 | 8.2 | 8.2 | +0.0 | 8.2 |
| [status-mobile-light.png](screenshots/kypulse-08-after-status-mobile-light.png) | admin-console | 9/8/8/8/8/8 | 8.2 | 8.2 | +0.0 | 8.2 |
| [appearance-desktop-dark.png](screenshots/kypulse-08-after-appearance-desktop-dark.png) | settings | 8/8/9/8/8/8 | 8.1 | 8.2 | +0.1 | 8.6 |
| [appearance-desktop-light.png](screenshots/kypulse-08-after-appearance-desktop-light.png) | settings | 8/8/9/8/8/8 | 8.2 | 8.2 | +0.0 | 8.6 |
| [appearance-mobile-dark.png](screenshots/kypulse-08-after-appearance-mobile-dark.png) | settings | 8/8/8/8/8/8 | 7.9 | 8.0 | +0.1 | 8.4 |
| [appearance-mobile-light.png](screenshots/kypulse-08-after-appearance-mobile-light.png) | settings | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [backup-desktop-dark.png](screenshots/kypulse-08-after-backup-desktop-dark.png) | settings | 8/8/7/8/8/8 | 7.6 | 7.8 | +0.2 | 8.4 |
| [backup-desktop-light.png](screenshots/kypulse-08-after-backup-desktop-light.png) | settings | 8/8/7/8/8/8 | 7.6 | 7.8 | +0.2 | 8.4 |
| [backup-mobile-dark.png](screenshots/kypulse-08-after-backup-mobile-dark.png) | settings | 8/8/7/8/8/8 | 7.8 | 7.8 | +0.0 | 8.2 |
| [backup-mobile-light.png](screenshots/kypulse-08-after-backup-mobile-light.png) | settings | 8/8/7/8/8/8 | 7.8 | 7.8 | +0.0 | 8.2 |
| [settings-desktop-dark.png](screenshots/kypulse-08-after-settings-desktop-dark.png) | settings | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [settings-desktop-light.png](screenshots/kypulse-08-after-settings-desktop-light.png) | settings | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.4 |
| [settings-mobile-dark.png](screenshots/kypulse-08-after-settings-mobile-dark.png) | settings | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |
| [settings-mobile-light.png](screenshots/kypulse-08-after-settings-mobile-light.png) | settings | 8/8/8/8/8/8 | 8.0 | 8.0 | +0.0 | 8.2 |

## Provenance (scripted, read-only)

| Item | Value |
|---|---|
| Worktree head and calibration commit | `a1cc471d47a50b4a7250013e19a2c984b91aaa7d` |
| UI source | `1fac23363f782c09e5b281ccb05b46d19c08455f`. All 21 manifest source pins equal the git objects at `1fac233` and at `a1cc471`, with 0 mismatches. |
| Evidence | `4128723ff9742a6a6f5b1b22b0b990f9bea54ec0`. All 40 PNGs equal their committed blobs, and their sha256, bytes and dimensions match the inventory and manifest. 28 admin-console, 12 settings, 0 duplicates (duplicate-audit-v8 is empty). |
| Manifest | `councilRound` 8. Captured `2026-10-27T07:05:57.923Z`. `source_commit` `1fac233`. `uncommitted_source_sha256` null. Served index `1170657c…f37272`. Overflow 0 on all 40. |
| Capture | Linux 7.2.10-1-cachyos, Playwright 1.63.0 with Chromium 153.0.8010.12 at device scale 1, normal service workers, self-hosted Space Grotesk loaded. Viewports 1280×900 and 390×844. OS light/dark with no saved theme. Synthetic admin with 3 apps (ok, degraded, down). Logs/Activity are empty and unpaired, captured both collapsed and expanded. Settings/Backup are unconfigured. Timestamps and ages are live. |
| Vendor | ky-ui 0.4.3 from producer `58626b7`: `tokens.css` `636fcf04…f235f4`, `appearance.css` `f689ae75…ccba` |
| Brand references | 4 KyNotes references at `1ab04e6` (`brand-references.json` `94764539…12de`). Used for brand only. |

**Input sha256:**

| File | sha256 |
|---|---|
| inventory-v8.json | `5ea7b62a05b049f1b7abe8118e3472b7c05ebe0c16872378b4b6a11cc7cec29e` |
| manifest-08-after.json | `16ac716abc3ee73609284ec71767d1613e51bf0b139fbdc533efd70b9af86756` |
| duplicate-audit-v8.json | `39e8f0a1dca489dcce4cb1eccd37da0801536e3228c140804aced9195b7993be` |
| opus-round8.md | `d27f1266d43680023b4364551c2734eca948368eabe3911b3870b216d230c70d` |
| opus-round8.json | `355f83c25094017980748451571cc40a6f723b4383a16228b6ef154785f78305` |
| sol-round8.md | `8a6bd22dc5a199c6fe66baad694c1e7ee81e2e9ae1b334c9a73b1f248c3f74da` |
| sol-round8.json | `9aee27cec19370dda4266816302c7ce970e198f5117d42beb71fe272021c6320` |
| opus-round8-calibration.md | `7535accc6947ebc8469e9131485ed9e593c34945d48a55f947425ae4ff9900f8` |
| opus-round8-calibration.json | `988816881cc206340cb2f05ca859bb014cd345f84af5e4cc8ea924b126fd07d0` |
| sol-round8-calibration.md | `83ae379d3147a88dac9c6d070062d2691c20393ac3cb38cba3a031e073312ea5` |
| sol-round8-calibration.json | `1a6de5dde1ef0388a42393dad6b6eeacb7dd78f22b3979c0259515dbbca167d2` |
| round8-calibration-audit.md | `12ccdb66b548213052142cc0c032eda7bf338e7bab6a327784515db051c797c3` |
| round8-calibration-audit.json | `dbb87b345aa907ac3eed6ea8fe7c1c9b8c06d22190be2d8170fadb5376069ae7` |
| round8-audit.md | `fe6795d9e2d9b337d2956851072050cc068868a6ece05030cacfdcc6debca3ae` |
| round8-audit.json | `cb642f948856eb708268ec6550af24867a2e19e84c528e9e8930c2f62f605d22` |
| native-round8.json | `8d52d868411ea26cc52228986da314349ee3aeaa5cb13f8c025f9083177c00c9` |

- The four raw advisory hashes equal the pins in `round8-calibration-audit.json`.
- The round-8 reports, the freeze and every shared export are unmodified.

**Contract versions:**

- The Opus calibration used contract `c191803e` from producer `58626b7`.
- The parent verified that it differs from `392afb88` only by adding `themes-browser.js` to the list of generated palettes.
- The disputed rules and anchors are identical in both.

## Gates, checks and limits

- **Behaviour gates** (keyboard, focus, contrast, states, responsive) are all untested.
  - One dark/light radio-ring sample is not a contrast-gate measurement.
  - Other named themes, other roles, populated Logs/Activity, loading and error states, and lower page content stay unverified.
- **Native supplement: not performed.**
  - It was optional, and the originals are the scoring evidence.
  - No tab was opened and the parent tab was not touched. No theme, content or configuration was written.
- **Checks:** I ran no build, test or CI. The local round-8 checks (frontend 96 tests, build, typecheck, `make ci`; browser suite 22 passed with 6 layout skips, exit 0) are as reported by the parent, on the unchanged runtime.
- **Remote CI and security:**
  - Remote CI at `50576c8` was pending at assignment, and security needs a current marker authored for the exact head.
  - A later docs-only push would make that run historical.
  - **I make no CI or security clearance claim.**
- **DOX pass:** I reread the chain. I changed no AGENTS file: these two reviewer-owned files fit the documented workflow.
- **Files written:** only `opus-round9.md` and `opus-round9.json`. No export, audit, commit, push, merge or deploy.
