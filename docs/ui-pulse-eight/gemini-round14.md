# KyPulse Visual 8 — Gemini 4 Argon Advisory Review (Round 14 / `stage14-after`)

- **Reviewer:** Gemini 4 Argon (`antigravity/gemini-4-argon-high`, `ThinkingLevel=high`)
- **Harness / Provider:** Google DeepMind Antigravity ACP (`antigravity-acp`) in T3 Code
- **Role:** Advisory UI Council reviewer (first KyPulse review; no nested subagents, delegates, or repeated Gemini calls invoked; Gemini approval is not a progression gate)
- **Schema & Rubric:** `ky-ui-council-scores/2`, rubric `ky-suite-v2` (SHA-256 `ad3516620420fbdc30f5273f82b7c72c76e1365d1b6296fcaa56be3cbebf2c41`; weights `hierarchy: 0.20`, `typography: 0.15`, `density_layout: 0.20`, `color: 0.15`, `visible_affordances: 0.15`, `coherence: 0.15`)
- **Runtime Commit:** `f1dcb5553e2693ee997488e2a3bd4434b2bb4f1e` (`align badge labels with the shared type scale`)
- **Evidence Commit:** `7569de314217dd4b161b968dd44a1a8230df1828`
- **Machine Record:** `docs/ui-pulse-eight/gemini-round14.json` (SHA-256 `5304b70f63b7deb5436d33fe95f6d9f24a7ec5e7fe0b7eb4deee155e50b899e6`)

---

## 1. Executive Summary & Cohort Means (No Visual Score — Tool Limitation)

| Cohort | Total Files | Unique Files (`duplicate_of: null`) | Gemini Visual Sum | Gemini Exact Mean | Gemini `ky_fit` Mean | Verdict (`satisfied`) | Opus 5.5 R14 Mean | GPT Sol R14 Mean |
| :--- | ---: | ---: | :---: | :---: | :---: | :---: | :---: | :---: |
| `admin-console` | 28 | 28 | `null` | `null` (no visual score) | `null` | `null` | `224.0 / 28 = 8.0` | `236.8 / 28 = 296/35 ≈ 8.4571` |
| `settings` | 12 | 12 | `null` | `null` (no visual score) | `null` | `null` | `96.0 / 12 = 8.0` | `101.2 / 12 = 253/30 ≈ 8.4333` |

### Why No Numeric Visual Score Is Reported (`G14-TOOL-1`)
Per `ky-ui@58626b77f26ee3f294c9a6a7a9f24f030c25ada5:reviews/AGENTS.md` (*"If a reviewer cannot inspect the original images directly, it must report no visual score rather than infer from source, metadata or other reviewers"*) and the review instructions (*"If your tools cannot display originals, explicitly report no visual score and the real tool limitation; do not infer ratings from source, metadata or primary scores"*):

1. **`view_file` and `client_view_file` on PNG paths** in `antigravity-acp` invoke UTF-8 text-file readers (`fs/read_text_file`) and return lossy replacement text (`{"result": "\ufffdPNG\r\n\u001a..."}`) rather than injecting an `image/png` multimodal content block into the model context.
2. **MCP image-returning tools (`t3-code` `html_preview` and `preview_snapshot` with `includeImage: true`)** render in headless Chromium, but the `antigravity-acp` host strips the returned `image/png` attachment and substitutes a text pointer (`[Resource offloaded to file:///.../.system_generated/steps/51/media_0.png]`).
3. Consequently, **none of the 40 Stage 14 PNGs or 4 KyNotes brand reference PNGs could be visually displayed at 1:1 resolution in model context.** All 40 per-file `scores`, `total`, and `ky_fit.scores` in `gemini-round14.json` are preserved as `null` (`confidence: "none"`).

---

## 2. Blind Freeze & Provenance Verification

### Blind Freeze Before Supplement and Primary Reports
Before opening `opus-round14.*`, `sol-round14.*`, `native-round14.json`, or historical Council reports, and before launching the live T3 Computer Use supplement, the complete blind `no_visual_score` payload was frozen:
- **Frozen At:** `2026-10-27T10:14:16Z`
- **Freeze File:** `/tmp/gemini-round14-blind-freeze.json`
- **Freeze SHA-256:** `c01fd456222e40de05a1196729da353482a1eebad09d15821bc311190dc327a8`

### Cryptographic Verification of Inputs
1. **40 Stage 14 Original PNGs (`docs/ui-pulse-eight/screenshots/kypulse-14-after-*.png`):**
   - Verified all 40 files on disk against `docs/ui-pulse-eight/manifest-14-after.json` (SHA-256 `dc76ac451d4f934ab6d279f56d507a09fb914f2f01d72891fd7f1efe78fdcff5`) and `docs/ui-pulse-eight/inventory-v14.json` (SHA-256 `f7b1af0aef140508df7a51284d437ad79c1690d7b6b2d2ace051a54494209871`).
   - Every file matches exact byte length, SHA-256 digest, and PNG `IHDR` dimensions (`20` desktop files at `1280×900`, `20` mobile files at `390×844`).
   - All 40 SHA-256 digests are unique (`duplicate-audit-v14.json` SHA-256 `39e8f0a1dca489dcce4cb1eccd37da0801536e3228c140804aced9195b7993be` has `near_duplicates: []`), and all 40 manifest `overflow` measurements are `0`.
   - **Cohort Membership Note (`G14-ADV-3`):** The prompt template mentions `"admin-console 24 unique (+4 exact duplicate system-dark files linked via duplicate_of)"`, which reflects an earlier pre-Round-7 / KyNotes cohort structure. In actual `inventory-v14.json`, there are **28 unique `admin-console` files** (including the 8 expanded `logs-filters` and `activity-filters` captures) and **12 unique `settings` files**, with `duplicate_of: null` on all 40 rows.
2. **21 Runtime Source Pins & Vendor `ky-ui` `0.4.3`:**
   - All 21 `source_files` entries in `manifest-14-after.json` match both current worktree bytes and git blobs at `f1dcb5553e2693ee997488e2a3bd4434b2bb4f1e` (`uncommitted_source_sha256: null`).
   - `web/src/ky-ui/VERSION` is `0.4.3`, and `web/src/ky-ui/tokens.css` (SHA-256 `636fcf04ce783edd7625b5265f63abb8b3abd9fae78ee653b1a2ea3142f235f4`) matches `ky-ui` producer commit `58626b77f26ee3f294c9a6a7a9f24f030c25ada5`. The dirty root `/home/yoshi/git/busnes.app/ky-ui` working tree was never used as authority; all `ky-ui` governance and contract files were read exclusively via `git show 58626b77f26ee3f294c9a6a7a9f24f030c25ada5:...`.
   - **Served Index Hash Note:** `manifest-14-after.json` records `served_index_sha256: "753be1fda88844c5eb59a6906a14edfe8c95da5e3c3c9493f2e3b4ca9f808230"`, whereas `web/dist/index.html` committed in `f1dcb55` has SHA-256 `86d6f2c990d13dac0a9dda8de1f9219a17bfc0b3de4f49e75c9f1478a4e95447` (referencing rebuilt `index-BAnOs74O.css`).
3. **4 KyNotes Brand Reference PNGs (`docs/ui-pulse-eight/brand-references.json`):**
   - Extracted all 4 pinned brand reference PNGs from `/home/yoshi/git/busnes.app/kynotes-server` at commit `1ab04e64de747f409d88b98f02682e85862defe1` into `/tmp/kynotes-brand-ref/` and verified exact SHA-256 and `IHDR` dimensions (`ff94b5e5...` `1280×900`, `e1585cc6...` `1280×900`, `43accf28...` `390×844`, `f77379a3...` `390×844`).

---

## 3. Post-Freeze T3 Computer Use Supplement (`preview_*`)

After freezing `/tmp/gemini-round14-blind-freeze.json`, live browser verification was executed using native `t3-code` `preview_*` tools (`preview_status`, `preview_open`, `preview_navigate`, `preview_resize`, `preview_set_appearance`, `preview_snapshot`, `preview_click`, `preview_press`, `preview_evaluate`, and `t3_preview_close`):

- **Port Resolution:** Calling `preview_open` on `http://127.0.0.1:45249` returned `net::ERR_CONNECTION_REFUSED`. Inspection of `docs/ui-pulse-eight/fixture.json` and active sockets showed the live Round 14 `KEEP_FIXTURE=1` server (`PID 1676027`) listening on `http://127.0.0.1:18436`.
- **Tab Hygiene:** Opened an isolated preview tab (`tab_2s_2e419d46-7b07-427f-bf30-1831131b4cba` with `reuseExistingTab: false`), signed in as synthetic `admin`, inspected all 10 surface states at `1280×900` and `390×844` in both `light` and `dark` appearances without mutating server or theme state, and closed `tab_2s_...` via `t3_preview_close` (leaving the parent tab `tab_2f_...` untouched).

### Live Measurements Across All 10 Surface States (`1280×900` & `390×844`, `light` & `dark`)

| Surface / Route | `1280×900` `scrollHeight` | `390×844` `scrollHeight` | `overflowX` (All 4 Combos) | Mobile Nav `[aria-current="page"]` Box (`nav` `[16,68,358,40]`) | Key Live Verification |
| :--- | ---: | ---: | :---: | :---: | :--- |
| `#/status` | `900px` | `855px` | `0` | `[16, 68, 93, 40]` (visible) | Failure-first order (`Backup agent` down → `Mail relay` degraded → `Billing API` ok); `Add app` `111×40px` (`5.33:1` light / `6.47:1` dark). |
| `#/alerts` | `900px` | `844px` | `0` | `[113, 68, 88, 40]` (visible) | 5-column table on desktop; stacked card rows at `390px` with `data-label` prefixes. |
| `#/apps/tgt_d5cdbaee54e58040` | `1193px` | `1607px` | `0` | `[16, 68, 93, 40]` (visible) | `Edit` (`83×40px`), `Silence 1h/8h/until fixed` (`40px` height, `4.54:1` border), `Delete` (`89×40px`, `6.57:1` light / `6.62:1` dark). |
| `#/logs` (collapsed) | `900px` | `1065px` | `0` | `[294, 68, 81, 40]` (visible) | `.dr-hint` `13px` (`max-width: 72ch` = `612px` desktop / `358px` mobile); `Filters` summary `80×40px`; `Add source` uses `.btn-secondary`. |
| `#/logs` (expanded) | `900px` | `1399px` | `0` | `[294, 68, 81, 40]` (visible) | 5 filter inputs (`40px` height, `4.54:1` border) + `Apply` (`71×40px` filled accent) + `Clear` (`76×40px` secondary); no datetime clipping. |
| `#/activity` (collapsed) | `900px` | `844px` | `0` | `[188, 68, 102, 40]` (visible) | `.dr-hint` `13px` (`612px` max-width); `Filters` summary `80×40px`; `15px` empty state (`--ink-strong`). |
| `#/activity` (expanded) | `900px` | `864px` | `0` | `[188, 68, 102, 40]` (visible) | 3-col desktop / 2-col mobile filter grid; `Apply` (`71×40px`) + `Clear` (`76×40px`); zero horizontal overflow. |
| `#/backup` | `1230px` | `1992px` | `0` | `[275, 68, 99, 40]` (visible) | **Round 14 fix confirmed:** 4 `.dr-facts` badges (`None` `50×25.5`, `Not paired` `84.5×25.5`, `0 of 7` `54×25.5`, `Daily` `49×25.5`) render at `13px/600` (`--ky-type-meta`), unclipped. |
| `#/settings` | `1103px` | `1559px` | `0` | `[236, 68, 138, 40]` (visible) | **Round 14 fix confirmed:** `Off` badge (`37×25.5px`, `13px/600`) stays on heading row (`x=282..319` at `390px`); `sqlite` badge (`66×25.5px`, `13px`) at `y=1008` desktop / `y=1472` mobile. |
| `#/appearance` | `906px` | `1982px` | `0` | `[245, 68, 130, 40]` (visible) | 23 `.ky-appearance-option` radio cards (`186×120px` desktop 5-col, `173×120px` mobile 2-col); checked `Match system` card has `1px` border + `1px` outline in `--accent` (`4.93:1` light / `6.47:1` dark). |

### Contrast & Focus-Visible Verification
- **Keyboard `:focus-visible` (`preview_press("Tab")`):** Verified live on navigation links and page controls: renders `outline: 2px solid var(--accent); outline-offset: 2px` (`rgb(191, 63, 24)` in light, `rgb(245, 134, 95)` in dark).
- **Text Contrast:** Primary ink (`--ink-strong`) achieves `14.86:1` on `--bg` (`13.68:1` on `--panel`) in light and `13.99:1` on `--bg` (`12.67:1` on `--panel`) in dark. Secondary ink (`--ink`) achieves `5.73:1` in light and `8.26:1` in dark.
- **Control & Non-Text Contrast:** Primary filled buttons achieve `5.33:1` (light) and `6.47:1` (dark). Secondary buttons, inputs, selects, and filter summaries use `--ky-field-border` (`#727777` light / `#9b9d9a` dark), achieving `4.54:1` (light) and `4.55:1` (dark) against their parent surface (exceeding WCAG 1.4.11 `3:1`).

---

## 4. Advisory Findings & Comparison with Primary Reviewers (Opus 5.5 & GPT Sol)

1. **`G14-VERIFY-1` — Concurrence on Round 14 `.badge` Typography Fix:**
   - **Agreement with Opus 5.5, GPT Sol, and `native-round14.json`:** Commit `f1dcb5553e2693ee997488e2a3bd4434b2bb4f1e` changes `.badge { font-size: 12px }` in [theme.css](file:///home/yoshi/git/busnes.app/kyPulse-server/.claude/worktrees/ui-pulse-eight/web/src/styles/theme.css#L285-L294) to `font-size: var(--ky-type-meta)` (`0.8125rem` = `13px`). Live DOM measurements confirm all four Backup badges (`None`, `Not paired`, `0 of 7`, `Daily`) and the Settings Two-factor `Off` badge now render at `13px/600` (`25.5px` height) with zero text clipping, wrapping, or horizontal overflow at both `1280×900` and `390×844`.
2. **`G14-ADV-1` — Concurrence with Primary `P14-1` / `P1` on Settings `sqlite` Accent Badge (Optional / Below Fold):**
   - **Agreement with Opus (`P14-1` / `P14-2`) and Sol (`P1`):** In [Settings.tsx](file:///home/yoshi/git/busnes.app/kyPulse-server/.claude/worktrees/ui-pulse-eight/web/src/pages/Settings.tsx#L38), the passive database driver readout (`sqlite`) uses `className="badge badge-accent font-mono"`, applying terracotta accent color to a non-interactive readout (and [TwoFactorCard.tsx](file:///home/yoshi/git/busnes.app/kyPulse-server/.claude/worktrees/ui-pulse-eight/web/src/components/TwoFactorCard.tsx#L83) uses `badge-accent` for the unpictured `On` state). Because `sqlite` sits below the initial fold (`y=1008` on desktop, `y=1472` on mobile) and predates `f1dcb55`, it does not affect initial-viewport confirmation and can be switched to `dr-badge-muted` (or `badge-success` for `On`) in a future cleanup.
3. **`G14-ADV-2` — Whole-Control Assessment of Appearance Radio Tiles (`#/appearance`):**
   - Evaluated as a whole control per `ky-ui` `reviews/AGENTS.md`, each `<label class="ky-appearance-option">` card combines a `48px` swatch preview, `14px/500` title (`16.06:1` light / `13.99:1` dark), `12px` mode subtitle, and a `16×16px` native radio indicator whose border has `16.06:1` (light) / `12.67:1` (dark) contrast, while the checked card has a `4.93:1` (light) / `6.47:1` (dark) `--accent` border and outline. Unchecked outer card borders use decorative `--line-strong` (`2.28:1` light / `2.09:1` dark against `--panel`); if `3:1` outer-card boundary contrast is ever desired independently of the inner radio indicator, changing `.ky-appearance-option` border to `var(--ky-field-border)` would raise it to `~4.5:1`.
4. **Comparison on Collapsed `#/logs` Hierarchy (`Opus 7` vs `Sol 8`):**
   - While Gemini assigns no visual score due to `G14-TOOL-1`, source and live DOM inspection show that when `.log-filter-panel` is collapsed on `#/logs`, the page intentionally avoids a filled orange CTA because `Filters` is a disclosure toggle (`80×40px` with `--ky-field-border`) and `Add source` inside `.log-sources` is a secondary setup action (`.btn-secondary`). This structural choice preserves `--accent` discipline for primary actions (`Apply` when filters are expanded).

---

## 5. Limitations

- **No 1:1 PNG Visual Inspection:** `antigravity-acp` cannot display PNG files as visual images in model context (`view_file`/`client_view_file` decode binary bytes as replacement UTF-8 text; MCP image responses are offloaded to disk paths). No numeric visual scores are inferred.
- **Browser Engine & DPR Difference in Supplement:** The post-freeze T3 Computer Use supplement ran in `HeadlessChrome/154.0.8037.92` at `DPR 2` against `http://127.0.0.1:18436`, whereas the 40 Stage 14 PNGs were captured by Playwright `Chromium 153.0.8010.12` at `DPR 1`.
- **Fixture State Coverage:** Only the synthetic default admin fixture states were inspected (unconfigured Backup, unpaired KyYard, empty Logs/Activity timelines in collapsed and expanded filter states, TOTP disabled). Populated log/activity timelines, paired KyRecovery/KyYard states, TOTP setup/enabled flows, error/loading states, and assistive-technology screen-reader announcements remain outside this review's scope.
