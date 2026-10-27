# Opus round11 review — KyPulse stage11-after

Claude Opus 5.5 (`claude-opus-5-5`), harness defaults, no subagents/Gemini. Rubric ky-suite-v2 `ad351662…2c41`, contract `392afb88…44d3`. Runtime `3b69600` (21 source pins verified against git blobs), evidence `104af24`; head at write `b8f8c05` is test/evidence-only. JSON: [opus-round11.json](opus-round11.json).

## Verdict

**Not satisfied.** Admin-console 222.0/28 = 111/14 = 7.9286 (<8). Settings 96.4/12 = 241/30 = 8.0333 (≥8). No pooling, no rounding into 8.

A1 and A2 are real visible gains, A3 fixed the twin-button reading and N1 behaves correctly natively. Admin stays below 8 because I still see the empty Logs/Activity next step sitting in muted meta text and the Log sources heading/paragraph rhythm off-grid. Part of the shortfall is my own variation on retained pixels (net -0.8 in the admin sum), which I disclose rather than average away. I do not request a score change.

## Inputs and blinding

- 40/40 originals: SHA256 = manifest = inventory = git blob at 104af24; bytes, IHDR dimensions, overflow 0, set membership verified. 28 admin-console, 12 settings; no near duplicates.
- Vendor 0.4.3: all 13 vendored files match pins. Four KyNotes brand refs (KyNotes-server 1ab04e64) hash-verified and viewed before brand scoring; brand only.
- Source diff 61e4a0c..3b69600 under web/src read before scoring; it matches N1/A1/A2/A3 as briefed.
- Priming: the brief gave patch intent, prior aggregates and taste items. Before freeze I opened no per-file prior score, prior report body, peer R11 or contact sheet; I read only the scoreless header of my R10 JSON for schema shape. `sol-round11` appeared during the task and was not opened.
- Freeze: 2026-10-27T08:41:47Z, notes sha256 `e39e2d9bce7f551adefc729804b897dceb7e64d704ce77a5b82e346e2d3c8f5e`. A first freeze at 2026-10-27T08:41:37Z (sha256 47473b9c4ee97f7941d50a054e4a31d883c24cbc60977d325ed486413f333e0a) was missing rows for logs-filters-mobile-dark and activity-filters-mobile-dark, which I had viewed but not written down. I added both 10 seconds later, before opening any history, and refroze.
- Pre-freeze self-correction: alerts desktop light/dark density_layout 7->8 before any history: the space below three rows is the sparse captured state and the rubric forbids rewarding density for more data.
- Frozen means: admin 221.6/28 = 277/35 = 7.9142857; settings 96.4/12 = 241/30 = 8.0333333.
- Post-freeze factual correction (4 app-detail files, typography 7→8): Factual misreading. The frozen note said the banner "Down" renders in a mono/display face. Native computed style is Space Grotesk 700 20px with the font loaded. Not a product change and not a judgement change. Admin sum 221.6→222.0; still below 8.

## Scores

| File | H | T | D | C | A | Co | Total | R10 | Δ | Ky fit |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|
| activity-desktop-dark (admin-console) | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.8 |
| activity-desktop-light (admin-console) | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.8 |
| activity-filters-desktop-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| activity-filters-desktop-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| activity-filters-mobile-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.6 |
| activity-filters-mobile-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.6 |
| activity-mobile-dark (admin-console) | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.6 |
| activity-mobile-light (admin-console) | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 8.0 | -0.2 | 8.6 |
| alerts-desktop-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| alerts-desktop-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| alerts-mobile-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| alerts-mobile-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| app-detail-desktop-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.8 |
| app-detail-desktop-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.8 |
| app-detail-mobile-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.7 | +0.3 | 8.6 |
| app-detail-mobile-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.7 | +0.3 | 8.6 |
| logs-desktop-dark (admin-console) | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | +0.0 | 8.8 |
| logs-desktop-light (admin-console) | 7 | 8 | 8 | 8 | 8 | 8 | 7.8 | 7.8 | +0.0 | 8.8 |
| logs-filters-desktop-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| logs-filters-desktop-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| logs-filters-mobile-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.6 |
| logs-filters-mobile-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 7.8 | +0.2 | 8.6 |
| logs-mobile-dark (admin-console) | 7 | 8 | 7 | 8 | 8 | 8 | 7.6 | 8.0 | -0.4 | 8.6 |
| logs-mobile-light (admin-console) | 7 | 8 | 7 | 8 | 8 | 8 | 7.6 | 8.0 | -0.4 | 8.6 |
| status-desktop-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| status-desktop-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| status-mobile-dark (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| status-mobile-light (admin-console) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| appearance-desktop-dark (settings) | 8 | 8 | 8 | 8 | 8 | 9 | 8.2 | 8.0 | +0.2 | 8.8 |
| appearance-desktop-light (settings) | 8 | 8 | 8 | 8 | 8 | 9 | 8.2 | 8.0 | +0.2 | 8.8 |
| appearance-mobile-dark (settings) | 8 | 8 | 8 | 8 | 8 | 9 | 8.2 | 8.0 | +0.2 | 8.6 |
| appearance-mobile-light (settings) | 8 | 8 | 8 | 8 | 8 | 9 | 8.2 | 8.0 | +0.2 | 8.6 |
| backup-desktop-dark (settings) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| backup-desktop-light (settings) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.8 |
| backup-mobile-dark (settings) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| backup-mobile-light (settings) | 8 | 8 | 8 | 8 | 8 | 8 | 8.0 | 8.0 | +0.0 | 8.6 |
| settings-desktop-dark (settings) | 8 | 8 | 8 | 8 | 8 | 7 | 7.9 | 8.0 | -0.1 | 8.6 |
| settings-desktop-light (settings) | 8 | 8 | 8 | 8 | 8 | 7 | 7.9 | 8.0 | -0.1 | 8.6 |
| settings-mobile-dark (settings) | 8 | 8 | 8 | 8 | 8 | 7 | 7.9 | 8.0 | -0.1 | 8.4 |
| settings-mobile-light (settings) | 8 | 8 | 8 | 8 | 8 | 7 | 7.9 | 8.0 | -0.1 | 8.4 |

Per-dimension observations, confidence and caveats for every file are in the JSON. Ky fit is separate and never added to the total.

## Drift against my R10

Max |Δ| 0.4; none reaches 0.5. Admin sum 221.8→222.0: product-attributable +1.0 (A2 density 7->8 on four app-detail files; A1 affordance 7->8 on two phone app-detail files); reviewer variation on retained regions -0.8 (Activity collapsed hierarchy 8->7 x4: -0.8; phone Logs collapsed hierarchy and density 8->7 x2: -0.8; phone expanded filters density 7->8 x4: +0.8). Logs desktop: A3 removed the R10 hierarchy reason (twin outlined buttons); I still hold H7 for a different, retained reason.
Settings sum 96.0→96.4: +0.4 (Appearance coherence 8->9 x4 +0.8; Settings coherence 8->7 x4 -0.4). That is reviewer variation, not a product change.
Every retained-region dimension change is labelled reviewer variation in the JSON. N1 is a behaviour fix and is not a screenshot gain.

## Native supplement (after freeze)

Own Incognito tab, Chrome/154.0.0.0, DPR 2 (scored originals are Chromium 153, DPR 1), OS light only, no writes, closed afterwards; parent tab untouched.
- N1: fresh reload of #/backup at 1280x900 (selected Backup inside sidebar 16..223, scrollLeft 0) -> resize to 390x844 → selected Backup x 275..374 inside nav 16..374, scrollLeft 120, pageY 0, overflow 0. Focus/scroll: pageY 600 retained, focus retained on Run restore drill, selected Backup still inside nav (275..374 of 16..398) (desktop max scroll on this page is 290, so pageY 600 was tested only across a phone-width resize).
- A1: Silence 1h 175x40 side gaps 54/54; 8h 175x40 53/53; until fixed 358x40 119/119.
- A2: every .app-detail-page .dr-section > h3 sits 16px above its next element at 390 and 1280 (Silence, Checks, Settings, KyYard, Recent logs, Alert history, Stop watching).
- A3: Refresh sources btn-tertiary 40px, transparent background and border, colour rgb(24,35,38) = --ink-strong #182326, radius 8px; Add source btn-secondary with visible field border; focus shows 2px solid rgb(191,63,24) outline, offset 2px; no filled accent button on collapsed Logs. hover rule present in served CSS (.btn-tertiary:hover:not(:disabled) background --panel-hover) but not exercised; disabled state not exercised.
- Logs phone rhythm: Log sources h2 bottom to first paragraph 0px; paragraph to paragraph 12px (390x844).
- Banner font: app-detail .banner h2 computed Space Grotesk 700 20px; document.fonts.check true.

## Remaining concrete defects

- **R1** (6 files where Log sources is visible: logs collapsed desktop/phone light/dark and logs expanded desktop light/dark (phone collapsed scored D7; desktop scored D8, an inconsistency I disclose rather than revise post-freeze); density_layout): Log sources h2 has 0px gap to its first paragraph while the two paragraphs are 12px apart (measured at 390); neither is on the 8px scale. Fix: Give the Log sources heading an 8px gap to its text and the paragraphs a 16px gap (or 8px) using --ky-space tokens; no copy change. Done when: At 390 and 1280 heading-to-text and paragraph gaps are 8 or 16px.
- **R2** (Logs and Activity collapsed, 8 files (hierarchy 7); hierarchy): Empty state: "No logs found." / "No audit events found…" is the strongest text, while the only next step (Pair it in Settings) sits in the muted 13px meta line above Filters. The contract asks empty states to offer the next action.. Fix: Place the existing KyYard-not-paired sentence and link directly under the empty message (layout only, existing copy), or ask the owner before any new copy. Done when: The next action sits next to the empty message in both views and themes. Copy changes need owner approval; this is a layout move of existing copy.

Optional or taste, not required:
- Settings: one bordered card per section (coherence 7) is a mild stray from the flat contract surface; not nested and consistent, so optional
- Status desktop intro wraps to a one-word last line; Settings intro wraps to "Appearance."; taste
- Appearance desktop last row of three; inherent to 23 themes
- Phone filter 8px rhythm: I now score 8 (R10 7); no change wanted
- Natural Backup heights, Status hierarchy 8 vs 9: unchanged taste

## Gates and verification

All behaviour gates are **untested**; the native measurements above are scoped evidence, not a pass. I did not rerun product checks.
- Full browser at 3b69600: /tmp/kypulse-r11-browser.log: 22 passed, 2 failed, 8 skipped; checks/r11-browser-failed.exit.txt = 1. Both failures are danger-button.spec page.screenshot "Clipped area is either empty or outside the resulting image" (light-1280, dark-1280).
- b8f8c05 changes that spec to scroll targets into view; checks/r11-danger-after.txt shows 2 passed, 2 skipped for that spec only. I saw no full-suite rerun at b8f8c05.
- Remote: a8b6e51 results are historical; current-head CI/security pending per brief; not checked by me.
- No full-product 8/9 acceptance, CI, security or consensus claim. MySlop: no board handoff by me.

## Limits

Initial viewport only; synthetic admin; empty/unpaired Logs/Activity; unconfigured Backup; viewer role, populated, loading and error states not covered; native check in Chrome 154 DPR 2 OS light only.
