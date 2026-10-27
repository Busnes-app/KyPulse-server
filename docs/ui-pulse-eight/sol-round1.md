# KyPulse baseline — GPT Sol round1

Schema: ky-ui-council-scores/2 · Model: gpt-6.1-sol · Rubric: ky-suite-v2 · Round:1

**Not satisfied. Admin-console 7.39/10 (n=20); settings 8.00/10 (n=12).** Both cohorts must reach8 independently. Settings meets the covered visual gate; admin-console does not. These are actual baseline scores, not projected targets or full-product acceptance.

**Procedural caveat:** all32 images were viewed individually at original resolution before historical numbers.28 dimension rows were explicitly frozen first. Status overall8.2 desktop/7.5 phone was noted before history, but its four dimension rows were transcribed afterward. Those totals were retained; the parent should flag this limitation or independently blind-rescore Status. No Opus report was read.

## Bounded critique

No P0/P1 task-blocking defect is established by this capture set. The following P2 findings describe scanning cost and inconsistent emphasis.

### P2 F1 — Compact the global AlertBar chronology on phones while keeping every problem represented

**Visible evidence:** The 125px phone header is followed by a roughly134px global band for only two problems: long underlined absolute-date links and a separate Alerts line. This repeats app-specific Down content on detail and takes priority above every configuration view.

**Smallest fix:** Give each global problem a short app/state/age lead, with the full since/cause information in subordinate wrapping metadata; keep problem links and the Alerts destination. Arrange Alerts alongside the summary rather than as a separate full-width line where space permits. Preserve problem meaning, delivery/stale notices, roles and the rule that silencing outbound messages never hides the problem. Keep the app-detail health panel diagnostic and prominent; it is a different component.

**Evidence:** [kypulse-00-before-status-mobile-light.png](screenshots/kypulse-00-before-status-mobile-light.png); [kypulse-00-before-app-detail-mobile-dark.png](screenshots/kypulse-00-before-app-detail-mobile-dark.png); [kypulse-00-before-appearance-mobile-light.png](screenshots/kypulse-00-before-appearance-mobile-light.png).

**Locations:** web/src/components/AlertBar.tsx:45, web/src/styles/theme.css:458, web/src/styles/theme.css:511.

### P2 F2 — Make Logs and Activity filters a deliberate responsive grid

**Visible evidence:** Three short inputs each occupy a separate phone row at about196px width inside a358px content column; both date bounds follow. Desktop wraps only To and actions while the source-origin field truncates its placeholder. These are inefficient captured layouts, not proof of unreachable results.

**Smallest fix:** Use an explicit filter grid: compact compatible fields together at390px, full-width text/date fields when needed, and one action row; align date bounds together on desktop. Give the source HTTPS origin flexible width. Preserve explicit Apply/Clear semantics and all UTC, retention and transport help.

**Evidence:** [kypulse-00-before-activity-mobile-light.png](screenshots/kypulse-00-before-activity-mobile-light.png); [kypulse-00-before-logs-mobile-dark.png](screenshots/kypulse-00-before-logs-mobile-dark.png); [kypulse-00-before-logs-desktop-light.png](screenshots/kypulse-00-before-logs-desktop-light.png).

**Locations:** web/src/styles/theme.css:514, web/src/pages/Logs.tsx:23, web/src/pages/Activity.tsx:23, web/src/components/LogSources.tsx:63.

### P2 F3 — Make reset and refresh secondary actions

**Visible evidence:** Apply and Clear have identical orange fills; Logs adds equally filled Add source and Refresh sources. Separate tasks lack a clear leading action.

**Smallest fix:** Keep Apply and Add source primary within their respective groups; apply existing btn-secondary to Clear and Refresh sources. Keep labels, event handlers and permissions.

**Evidence:** [kypulse-00-before-logs-desktop-light.png](screenshots/kypulse-00-before-logs-desktop-light.png); [kypulse-00-before-activity-desktop-dark.png](screenshots/kypulse-00-before-activity-desktop-dark.png).

**Locations:** web/src/pages/Logs.tsx:29, web/src/pages/Activity.tsx:29, web/src/components/LogSources.tsx:74, web/src/styles/theme.css:533.

### P2 F4 — Present complete event summaries without split transition labels on phones

**Visible evidence:** pending → degraded breaks across three lines and Delivery is beyond the initial horizontal table view. Native inspection confirms the table scrolls internally; this is not an unreachable column or document overflow claim.

**Smallest fix:** At the existing phone breakpoint give each event an app/change lead and labeled time/reason/delivery below, retaining the same event data and links; desktop keeps its aligned table. Preserve screen-reader table or equivalent list semantics rather than visually hiding columns.

**Evidence:** [kypulse-00-before-alerts-mobile-light.png](screenshots/kypulse-00-before-alerts-mobile-light.png); [kypulse-00-before-alerts-mobile-dark.png](screenshots/kypulse-00-before-alerts-mobile-dark.png).

**Locations:** web/src/pages/Alerts.tsx:54, web/src/styles/theme.css:504.

### P2 F5 — Use the shared warning and link vocabulary on operator data

**Visible evidence:** Degraded text is subdued brown beside a bright yellow dot in dark Status/Alerts; app links use browser blue/violet while the detail Status link uses ink. This reduces emphasis consistency; no numeric contrast failure is asserted.

**Smallest fix:** Use existing warning tokens for degraded text/dots and an underlined ink link treatment for ordinary in-content app links. Check both Busnes and all named palettes before accepting contrast; do not hand-edit vendored ky-ui.

**Evidence:** [kypulse-00-before-status-desktop-dark.png](screenshots/kypulse-00-before-status-desktop-dark.png); [kypulse-00-before-alerts-desktop-dark.png](screenshots/kypulse-00-before-alerts-desktop-dark.png); [kypulse-00-before-alerts-desktop-light.png](screenshots/kypulse-00-before-alerts-desktop-light.png).

**Locations:** web/src/styles/theme.css:488, web/src/styles/theme.css:492, web/src/pages/Alerts.tsx:65.

Implement F1–F5 as one composition/style batch, then review matched after captures. Keep the single left region, existing Appearance picker, app-detail diagnostic Down block, flat facts, roles and security contracts. No extra features or wholesale redesign are needed. A successful after score must be measured; no score is promised.

Uppercase table headings and slightly different title sizes are polish preferences, not standalone progression blockers. No penalty was assigned for synthetic data, unpaired integrations or off-screen lower content.

## Provenance and coverage

- Review HEAD: d3e823db6e6ace4f081f9eb3a089c27d8d6ad24d. Product source: fef7b670a36d48792137f56d839e49a86d44eadc. Committed and working web/src + web/dist differences are empty.
- Served index and committed index agree: d45283dff4c3b67ea620708230919e7cfe20edfacc09b4d1a18517b8fb859a37. All9 manifest source pins and all12 vendor pins match; actual palette authority is vendored ky-ui0.4.1.
- All32 hashes, byte sizes and PNG dimensions match both inventory and manifest;32 distinct hashes.240 same-dimension pairs checked: no max-channel-delta≤8 candidates; minimum differing-pixel fraction4.726649%.32 manifest document overflows are0.
- Scored capture: Chromium153.0.8010.12, scale1,1280×900/390×844, OS light/dark, no saved theme, normal service workers; bundled Space Grotesk loaded.
- Current evidence:20 admin-console images (Status, Alerts, app detail, Logs, Activity) and12 settings images (Settings, Appearance, Backup). Logs/Activity empty/unpaired; settings synthetic/unconfigured; three synthetic watched apps.
- Four pinned KyNotes images were inspected and hash/dimension checked for brand fit only. Shared palette, typography, control geometry, single navigation region and Appearance previews support a two-product cohesion comparison. Notes is not rerated or pooled.
- T3 preview: own incognito tab opened after preview_status, explicit tabId throughout, tab closed. Chromium154/scale2 measurements are supplemental. Native Appearance has23 radios, system checked and no ky_theme;390×844 header125px, nav358px client/754px scroll; document overflow0. Alerts table421.34px within356px internally scrolling container; global AlertBar134px.
- No writes to source/tests/common artifacts, GitHub, git state or shared board; no fixture restart or broad suites. Login and navigation only; no theme save, pairing, backup, silence or application mutation.

## Arithmetic and per-file scores

Weights: hierarchy.20, typography.15, density_layout.20, color.15, visible_affordances.15, coherence.15. Each file total uses exact Decimal weighted sum and ROUND_HALF_UP to1 decimal. Cohort means use only those rounded unique totals. Ky fit is a separate equally weighted five-dimension score and never enters quality totals.

Admin-console: sum 147.8 ÷20 = 7.39. Settings: sum 96.0 ÷12 = 8.00. No pooled app score.

The JSON contains all32 complete evidence rows, six concrete dimension observations per row, inherited hash/cohort/state/theme/confidence/caveat, separate Ky-fit scores/observations and source paths. H/T/D/C/A/U below mean hierarchy/typography/density/color/affordances/coherence.

| File | Cohort | H/T/D/C/A/U | Quality | Ky fit | SHA-256 |
| --- | --- | --- | ---: | ---: | --- |
| [kypulse-00-before-activity-desktop-dark.png](screenshots/kypulse-00-before-activity-desktop-dark.png) | admin-console | 7/8/7/7/7/8 | 7.3 | 8.0 | 4727ad4f0b1f05bd4f732bb74e6d9af7558091b91029150f4028be9988fc2b49 |
| [kypulse-00-before-activity-desktop-light.png](screenshots/kypulse-00-before-activity-desktop-light.png) | admin-console | 7/8/7/7/7/8 | 7.3 | 8.0 | f0025cb6e9851ea712764f11d07d016c7970da23a41493fc963ce9edd4cabf80 |
| [kypulse-00-before-activity-mobile-dark.png](screenshots/kypulse-00-before-activity-mobile-dark.png) | admin-console | 6/8/6/7/7/8 | 6.9 | 7.6 | 058a58c5403a46d3cb53f9905220c290f4970381fc5e4ca7028bd797301f59bb |
| [kypulse-00-before-activity-mobile-light.png](screenshots/kypulse-00-before-activity-mobile-light.png) | admin-console | 6/8/6/7/7/8 | 6.9 | 7.6 | 3c130990c644ce98498fb04107faaf5c2150488f3b053d5906524c7b967feeaa |
| [kypulse-00-before-alerts-desktop-dark.png](screenshots/kypulse-00-before-alerts-desktop-dark.png) | admin-console | 8/8/8/7/7/7 | 7.6 | 8.2 | 40c38837e646a567996a5bdc71bbb0474b2aba88ded179cd35ef2678f31cf95b |
| [kypulse-00-before-alerts-desktop-light.png](screenshots/kypulse-00-before-alerts-desktop-light.png) | admin-console | 8/8/8/7/7/7 | 7.6 | 8.2 | dc5539ac285b56cf653be9df6ce0aa7c1009acc93d5ed7672aa0f1bb87fa52b5 |
| [kypulse-00-before-alerts-mobile-dark.png](screenshots/kypulse-00-before-alerts-mobile-dark.png) | admin-console | 7/7/6/7/6/7 | 6.7 | 8.0 | af56b9d4f5cbc5942f9415374f1586e413034f59334a5feff2d72e3cd389496b |
| [kypulse-00-before-alerts-mobile-light.png](screenshots/kypulse-00-before-alerts-mobile-light.png) | admin-console | 7/7/6/7/6/7 | 6.7 | 8.0 | 5e4d3ac2859aad370fb6db2c93531e6f322e932751f66dd3b36412885fc63110 |
| [kypulse-00-before-app-detail-desktop-dark.png](screenshots/kypulse-00-before-app-detail-desktop-dark.png) | admin-console | 8/8/8/8/9/8 | 8.2 | 8.2 | f4426e10a61574e550da69a1151238bbee25a24ed021e068e6c975e4f25d11aa |
| [kypulse-00-before-app-detail-desktop-light.png](screenshots/kypulse-00-before-app-detail-desktop-light.png) | admin-console | 8/8/8/8/9/8 | 8.2 | 8.2 | 26ba9f4fdcc2fff7a0a841910cb04f70699dd84d9f6977af5c6781850582e5c2 |
| [kypulse-00-before-app-detail-mobile-dark.png](screenshots/kypulse-00-before-app-detail-mobile-dark.png) | admin-console | 7/8/7/8/8/8 | 7.6 | 8.0 | 6e22f244f59eee25680d1604b9c338aeaf153bdd99f25530ef627616056de831 |
| [kypulse-00-before-app-detail-mobile-light.png](screenshots/kypulse-00-before-app-detail-mobile-light.png) | admin-console | 7/8/7/8/8/8 | 7.6 | 8.0 | ee69b58d34415a9426defbb7f9654987da1f8d4c70bb1fff08c415e57a3f8d77 |
| [kypulse-00-before-appearance-desktop-dark.png](screenshots/kypulse-00-before-appearance-desktop-dark.png) | settings | 9/8/9/8/9/9 | 8.7 | 8.8 | 7d788212492a76f7cded8cd2be90967a3d6f50c56a09acecf5f72fb3e69f52cb |
| [kypulse-00-before-appearance-desktop-light.png](screenshots/kypulse-00-before-appearance-desktop-light.png) | settings | 9/8/9/8/9/9 | 8.7 | 8.8 | 741b6690c9be27e5e7864f12c6c6ae2cbb4289b7f44a7eab368a61206ffdfaf7 |
| [kypulse-00-before-appearance-mobile-dark.png](screenshots/kypulse-00-before-appearance-mobile-dark.png) | settings | 8/8/8/8/9/9 | 8.3 | 8.6 | 08c84a2cec52d41e3d30eacd3eacff18015ceb0b1896610da2c53390b22c249f |
| [kypulse-00-before-appearance-mobile-light.png](screenshots/kypulse-00-before-appearance-mobile-light.png) | settings | 8/8/8/8/9/9 | 8.3 | 8.6 | 10d37a5d6f998f0b07cece94b4f08c377502285bcbd4de58c70a006c4ea9ed20 |
| [kypulse-00-before-backup-desktop-dark.png](screenshots/kypulse-00-before-backup-desktop-dark.png) | settings | 8/8/8/7/8/8 | 7.9 | 8.2 | 3744324bdc567c3d6df80c360fe1fef0b73dedd985c87e9360bf82392b1d255f |
| [kypulse-00-before-backup-desktop-light.png](screenshots/kypulse-00-before-backup-desktop-light.png) | settings | 8/8/8/7/8/8 | 7.9 | 8.2 | 7d471478b8317d5c87c4a7f8ff638df2dae09c51949ad36f35fe9fb573179e7e |
| [kypulse-00-before-backup-mobile-dark.png](screenshots/kypulse-00-before-backup-mobile-dark.png) | settings | 7/8/7/7/8/8 | 7.5 | 8.0 | 4c846b6f40d390df10b1fa03a384f78f6c40a0a194bd564564ceaa47031f1b6b |
| [kypulse-00-before-backup-mobile-light.png](screenshots/kypulse-00-before-backup-mobile-light.png) | settings | 7/8/7/7/8/8 | 7.5 | 8.0 | 1d778c3ce18cc6a7d686cdae0dd48e1e9bf66b75ab83642b2718c2107d0a077d |
| [kypulse-00-before-logs-desktop-dark.png](screenshots/kypulse-00-before-logs-desktop-dark.png) | admin-console | 6/8/7/7/7/7 | 7.0 | 7.8 | 7dd45a837ca76bb698cdd65db67c6a8850a4482dd6b3d3c26fcac289e4c9db52 |
| [kypulse-00-before-logs-desktop-light.png](screenshots/kypulse-00-before-logs-desktop-light.png) | admin-console | 6/8/7/7/7/7 | 7.0 | 7.8 | 8f189045cd6875746f8e983d289be6a6c5040d5e58d33381dba10699739575f2 |
| [kypulse-00-before-logs-mobile-dark.png](screenshots/kypulse-00-before-logs-mobile-dark.png) | admin-console | 6/8/6/7/7/8 | 6.9 | 7.6 | 3f9bfc8b9f88981eb9dac7c02af30c750a579fe4f3b3833dd51815f1793d6e33 |
| [kypulse-00-before-logs-mobile-light.png](screenshots/kypulse-00-before-logs-mobile-light.png) | admin-console | 6/8/6/7/7/8 | 6.9 | 7.6 | 087bd03c3f92338e410f7304817bded25b2fbd0f2309016a9623c5978d8d97ed |
| [kypulse-00-before-settings-desktop-dark.png](screenshots/kypulse-00-before-settings-desktop-dark.png) | settings | 8/8/8/8/8/8 | 8.0 | 8.2 | c0ce040038e405440561b3ba9139e38de818e25fbe69b1064ad525872a57a596 |
| [kypulse-00-before-settings-desktop-light.png](screenshots/kypulse-00-before-settings-desktop-light.png) | settings | 8/8/8/8/8/8 | 8.0 | 8.2 | 8bcc76304322c737cc2ad8e89e68c1c14b84ac80d719706eea6168bcd050d6da |
| [kypulse-00-before-settings-mobile-dark.png](screenshots/kypulse-00-before-settings-mobile-dark.png) | settings | 7/8/7/8/8/8 | 7.6 | 8.0 | 171c084ceed68f4e18352405423801cfc5cafdd398290ef2ff3210ec6e9fef2d |
| [kypulse-00-before-settings-mobile-light.png](screenshots/kypulse-00-before-settings-mobile-light.png) | settings | 7/8/7/8/8/8 | 7.6 | 8.0 | 68db7ecb827d3b9e973456c61e75a54f339f3bf34b47b330aab521caedceb972 |
| [kypulse-00-before-status-desktop-dark.png](screenshots/kypulse-00-before-status-desktop-dark.png) | admin-console | 8/8/8/8/9/8 | 8.2 | 8.2 | cf070b3fc986bf24cbae7a2fc3f0650090c2762da9d99202c6887be49d0cbc1a |
| [kypulse-00-before-status-desktop-light.png](screenshots/kypulse-00-before-status-desktop-light.png) | admin-console | 8/8/8/8/9/8 | 8.2 | 8.2 | 6630c65ea972d0d9dfdebb3c02d9f3ca73b69ed92235422a460f945287ef2c95 |
| [kypulse-00-before-status-mobile-dark.png](screenshots/kypulse-00-before-status-mobile-dark.png) | admin-console | 7/8/7/8/7/8 | 7.5 | 8.0 | 716996b2d84ab484dc8378f0a11d5b45a5c0311dd59a18a552bf686bf2c7694b |
| [kypulse-00-before-status-mobile-light.png](screenshots/kypulse-00-before-status-mobile-light.png) | admin-console | 7/8/7/8/7/8 | 7.5 | 8.0 | 48c1d9d296134a967b06926ffe0f85b0118ff56e0570bf4c08331525fc00bb05 |

## Gates, limits and drift

Keyboard, focus, measured all-theme contrast, complete states and full responsive behavior: **untested**. Zero document overflow is recorded evidence, not a complete responsive gate pass. No screenshot can establish keyboard/contrast/performance acceptance.

- Only initial1280x900 and390x844 viewports, Chromium153.0.8010.12 scale1, OS-following light/dark, no saved theme choice, normal service workers.
- Only synthetic administrator coverage. Existing viewer/role tests were not rerun by parent or reviewer; no authorization acceptance.
- Logs/Activity are empty/unpaired, not populated-state evidence. Settings/Backup are unconfigured defaults. Status has three synthetic watched apps; no multi-team/dense-history coverage.
- Keyboard, focus, all-theme contrast, screen readers, WebKit, soft keyboards, performance, loading/error task states and full behavior acceptance remain untested.
- No penalty for lack of real data or for lower-page content outside the captures. No off-screen feature absence inferred.
- Live Computer Use is supplemental Chromium154/scale2; timestamps/latencies change. No saved theme, pairing, backup, silence or app data was modified.
- Historical coverage and capture rules differ, including full-page images and blocked workers in follow-ups. Current-versus-historical scores are not a controlled improvement.
- No competitor set; no cross-cohort, whole-app, provider or whole-fleet quality ranking. Brand references are excluded from all quality counts/means.
- No broad build/test/capture suite or fixture restart was run; review integrity checks are not product acceptance.
- Impeccable context found no PRODUCT.md/DESIGN.md; the explicit Operate brief and incumbent captured UI supplied review context. No context files were created.
- Other current review was not read. Opus score/satisfaction is pending to this reviewer; Gemini deferred and not invoked. No subdelegation, GitHub, git mutation or shared-board action.

Historical context was read after the blind pass: the suite handoff records Pulse5.6 before refinement and6.9 latest scored views. Those differ in image coverage, state, source and capture extent; the follow-up set is expressly unscored. No same-hash historical current input was identified, so all32 prior_total/delta fields and mean drift are N/A. No controlled improvement, competitor win or cross-cohort rank is claimed. Existing ink return links, flat fact rows and inline Backup badges are current baseline strengths, not unfinished historical fixes.

Council status: Sol round1 admin-console7.39/settings8.00, unsatisfied; Opus5.5 pending/unread; Gemini4Argon deferred and unused. Parent owns synthesis, spread reconciliation, spreadsheet and progression.

## Review artifact checks

All32 evidence rows, score bounds, six observations, unique membership, Decimal totals and cohort sums were recomputed before writing. The JSON is the canonical machine-readable record. No product acceptance suite was run under this read-only assignment. DOX pass complete: existing AGENTS ownership and workflow still apply, so no instruction documents were changed.

## Input hashes

- docs/ui-pulse-eight/inventory-v1.json: 4808fbe8194c89560623f636a6b607fecb4d8de5a9d4a667893e97eabc553136
- docs/ui-pulse-eight/manifest-00-before.json: 1d5fe6fc2b668bae968857b771d64809bdb8768e8e29a69a168d29c8b5acbfd8
- docs/ui-pulse-eight/brand-references.json: 947645397875278d9f168d2e4bab46d4d40788f3074f30efc8695f69bb8912de
- docs/ui-pulse-eight/native-baseline.json: a9b99b5f64b3136975612836b9608cbc242a713073a735d4647cc959dfdb5f37
- /home/yoshi/.codex-cpa/skills/ky-suite-ui/references/rubric.md: ad3516620420fbdc30f5273f82b7c72c76e1365d1b6296fcaa56be3cbebf2c41

Review these two reports, especially the separate cohort threshold and Status blindness caveat. The reviewer task is complete; no implementation is included.
