# KyPulse visual8: Opus 5.5 round 8 calibration advisory (unblinded)

- Reviewer: Claude Opus 5.5 (`claude-opus-5-5`). No subagents, no Gemini.
- This is advice. It contains **no new numerical score** and repairs no frozen number. The scores in `opus-round8.json` and `sol-round8.json` stay authoritative.
- Rubric `ky-suite-v2` (producer `58626b7`, sha256 `ad351662…2cf41`). Design contract sha256 `c191803e…7358c`. Vendored ky-ui 0.4.3: `tokens.css` `636fcf04…35f4` and `appearance.css` `f689ae75…ccba`, both equal to the producer `58626b7` blobs. I did not read or edit the dirty root ky-ui working files.
- Runtime UI `1fac233`; evidence `4128723`.
- Machine record: `opus-round8-calibration.json`. `build-report.py` skips this filename, because its regex is `(opus|sol|gemini)-round(\d+)\.json`.

## Verdict

**No defensible required change. The raw gate stays unmet.**

- Opus 222/28 (admin-console) and 95/12 (settings) remain below 8.0. Visual8 is therefore not satisfied for Opus.
- Gemini stays deferred. Nothing here waives, rounds or averages.

Every disputed deduction falls into one of three groups:

- **Taste**, constrained by layout choices that were already rejected.
- **Reviewer variation on unchanged pixels**, contradicting six or seven earlier Opus rounds on the same pixels.
- **A reversal of a fix Opus itself requested earlier.**

None of them is a user-visible required defect at full resolution. A further code iteration aimed at these points would be score-chasing, and two of the candidates would reopen earlier fixes.

## Inputs (sha256)

| File | sha256 |
|---|---|
| opus-round8.md | d27f1266d43680023b4364551c2734eca948368eabe3911b3870b216d230c70d |
| opus-round8.json | 355f83c25094017980748451571cc40a6f723b4383a16228b6ef154785f78305 |
| sol-round8.md | 8a6bd22dc5a199c6fe66baad694c1e7ee81e2e9ae1b334c9a73b1f248c3f74da |
| sol-round8.json | 9aee27cec19370dda4266816302c7ce970e198f5117d42beb71fe272021c6320 |
| round8-audit.md | fe6795d9e2d9b337d2956851072050cc068868a6ece05030cacfdcc6debca3ae |
| round8-audit.json | cb642f948856eb708268ec6550af24867a2e19e84c528e9e8930c2f62f605d22 |
| native-round8.json | 8d52d868411ea26cc52228986da314349ee3aeaa5cb13f8c025f9083177c00c9 |
| inventory-v8.json | 5ea7b62a05b049f1b7abe8118e3472b7c05ebe0c16872378b4b6a11cc7cec29e |
| manifest-08-after.json | 16ac716abc3ee73609284ec71767d1613e51bf0b139fbdc533efd70b9af86756 |

- The four raw-report hashes equal the pins in `round8-audit.json`.
- All 40 originals match `inventory-v8.json`, with 0 mismatches.
- I viewed 12 originals one at a time at original resolution (1280×900 or 390×844). Their hashes are in the JSON.
- I did not use a contact sheet.
- I opened prior-round reports only to read the score history on the disputed dimensions.

## Disputed claims

Classes used below:

- **required**: a user-visible defect that needs a fix.
- **polish**: optional improvement.
- **taste**: a preference with no defect behind it.
- **misapplied**: the rubric was applied incorrectly, or the score varies on unchanged pixels.
- **unknown**: the evidence cannot settle it.

### 1. Appearance dark: visible_affordances, Opus 7 vs Sol 9 (J5)

**Class: misapplied.** The native radio does not call for a producer ky-ui change.

The radio itself:

- The radio is the browser's native control (`appearance: auto`, 16px), with `accent-color` set to the accent token.
- In dark mode, Chromium paints an unchecked radio as a `#3b3b3b` disc with a 1px ring.

Contrast, measured on the original pixels:

| Theme | Ring | Card | Ring contrast |
|---|---|---|---:|
| Dark | `#7c7d7d` | `#1f2b2e` | 3.52:1 |
| Light | `#828282` | `#ffffff` | 3.84:1 |

- The two themes differ by 0.3:1, and both rings clear 3:1.
- Opus scored light 8 and dark 7. That asymmetry has no measured basis. The rubric says "a dark theme gets no automatic bonus or penalty".

The card carries the selection:

- Each option is a whole `<label>` card, 186×120 natively, with a pointer cursor.
- Selection is shown by an accent border, an accent outline and a filled accent radio.
- At full resolution, the selected card is unambiguous in both dark originals.
- The design contract explicitly accepts "the browser's visible native indicator inside native controls".

History:

- Opus scored this unchanged surface A8 in rounds 3–7.
- Opus's own round-8 drift section attributes the change to variation on unchanged pixels.

What remains unknown:

- Rendered contrast in the 21 other named themes.
- Hover and focus in dark.
- The parent's single-ring measurement is not a contrast-gate pass, and neither is mine.

**Recommendation:**

- No ky-ui change.
- A custom-drawn radio (`appearance: none`) would trade a native control for new suite-wide vocabulary in order to fix a dim interior that the card already makes redundant.

### 2. Logs: Add source vs Refresh sources vs Apply (J1)

**Class: misapplied. The current split is coherent.**

What the screenshots show:

- In the collapsed state there is no filled button.
- Expanded, Apply is the single filled button.
- Add source and Refresh sources are identical ink outlines; I confirmed both are `btn-secondary` natively.

History of this criterion:

| Round | Reviewer | What was asked or scored |
|---|---|---|
| 1 | Sol | Asked for Apply and Add source each to lead its own group. |
| 2 | Opus | Asked for Add source to be demoted, because it competed with Apply as a second filled primary. |
| 3 | Product | Shipped exactly that demotion. |
| 3–6 | Opus | Logs desktop H8 on these buttons, which have been unchanged since round 3. |
| 7 | Opus | Collapsed H7, expanded H8. |
| 8 | Opus | H7 on both states, for the outcome Opus requested. |

This is a reversed criterion, not a new observation.

**Do read-only or empty views need a filled primary? No.**

- The rubric asks for "one obvious primary action". The contract says orange marks "the single primary action of a view", which does not mean every view must have one.
- Status, Alerts and app detail carry no filled action and hold H8–9 from both reviewers.
- The empty Logs view already offers its next action: the "Pair it in Settings" link and the labelled Log sources form.
- `round7-audit.md` reached the same conclusion.

**If the owner still wants a change, the one coherent one-primary plan is:**

- Make Add source `<button type="submit">` (filled).
- Make Logs Apply `btn-secondary`.
- Leave Clear and Refresh sources secondary.

Completion criterion: exactly one computed accent fill in `main` in both collapsed and expanded Logs, at both widths.

Costs, which are why I do not recommend it:

- Logs Apply would then differ from Activity Apply, which stays filled as that page's only form. That creates a new sibling inconsistency for the next round to find.
- Making a filter submit secondary is also debatable.

I advise against both plan variants:

- **Two filled buttons in the expanded state:** this breaks the contract.
- **A text-only tertiary Refresh:** KyPulse has no tertiary class, so it would add new vocabulary (`btn-secondary` and `btn-danger` are the only variants in the codebase).

### 3. Expanded phone Logs/Activity: 2+1+1+1 grid (J2)

**Class: taste.** Opus's own report already calls this "borderline and close to taste".

- At 390px, the five fields are two short fields paired, then the text field, then two native date fields.
- The dates cannot pair; that prototype was rejected because the native control clipped at 390px.
- A single column would add one row (about 72px). The result line sits at y≈760–825 (Logs about 771, Activity about 790 in the originals), so it would be pushed past 844px. That would reopen the round-7 fold complaint.
- Giving free text the full width is a reasonable choice, not a defect.
- Spacing is an even 8px.

**Recommendation:** no change.

### 4. App detail: filled status, flat facts, bordered history, inline Checks (J3)

**Class: misapplied for coherence, polish at most for the inline heading.**

- **The three treatments are three semantic roles,** not three vocabularies:
  - the tinted danger banner is status, paired with text;
  - the flat key/value facts are configuration;
  - the bordered `events` table is history.
- The history table is the same `events` component the Alerts page uses (`AppDetail.tsx:196`, `Alerts.tsx:56`), so it is product-level vocabulary reuse.
- **The inline "Checks — No checks reported yet."** was the round-3 fix for Opus's own round-2 complaint about an "empty Checks half-column void" (`theme.css:590-592`, commit `3984044`).
- Opus scored app-detail coherence 8 in rounds 3–7. The round-8 drop to Co7 happened on pixels where only the chronology changed.

**Recommendation:** no change.

- The only defensible polish is to drop `theme.css:591-592` so that Checks stacks its hint like KyYard, Recent logs and Alert history do. Line 590 alone already prevents the half-column void.
- Completion criterion for that polish: in the empty-checks state at both widths, the "No checks reported yet." hint starts below the Checks heading, with the same heading-to-hint gap as KyYard.
- This is optional. I do not expect it to be the reason any gate passes or fails.

### 5. Backup: repeated heading/action, seven buttons, vertical gaps, unequal heights (J4)

**Class: polish or taste.** This is the most stable remaining Opus deduction: Backup desktop D7 appears in four of six Opus rounds (3, 4, 7 and 8).

**Seven buttons:**

- Refresh, Pair (filled primary), Pin key, Back up now (disabled), Download capsule (disabled), Run restore drill, and the sidebar Sign out.
- The two disabled buttons are visibly distinct from enabled secondaries, as the contract requires. They show what becomes available once a key is pinned.
- When a key is pinned, Pair turns secondary and Back up now becomes the filled primary (`Backup.tsx:365`, `:494`), so one primary holds across states.
- This is not a defect.

**The "Back up now" heading repeats the button label beside it.** This is real but minor redundancy. Renaming the heading is a copy change, and the contract says to ask before changing copy. Polish, owner's call.

**Unequal heights:** the columns end about 31px apart (Pair row at y456, Pin key row at y487), and a shared divider sits 32px below the taller one. Columns of different content naturally end at different heights. Taste.

**Phone gaps:** measured ink-to-ink on the original:

- notice → KyRecovery heading: 17px;
- Pair button → "Recovery key by hand": 49px.

Tight under a page-level notice and loose between peer sections is hierarchical grouping, not an uneven rhythm. Taste. The off-8px source values (`dr-section` 20px, `dr-two` gap 20px) belong under Ky-fit type_geometry, not quality density.

**Recommendation:** no required change. The heading rename is the only candidate, and it needs owner consent for the copy.

## Smallest justified implementation list

**Empty.** There is no required item.

Optional items, for owner choice only. Neither is a gate lever:

| Item | Completion criterion |
|---|---|
| Drop `theme.css:591-592` (stacked Checks hint) | Hint below the Checks heading in empty-checks app detail, at 1280 and 390, in both themes. |
| Rename the "Back up now" section heading | Needs owner copy approval. Heading text differs from every button label in that section. |

## Gate statement (unchanged)

| Reviewer | admin-console | settings | Visual8 |
|---|---|---|---|
| Opus round 8 | 222.0/28 = 7.928571… | 95.0/12 = 7.916666… | not satisfied |
| Sol round 8 | 240.0/28 = 8.571428… | 102.0/12 = 8.5 | satisfied |

- No pooling, no rounding and no waiver.
- This advisory finds that the Opus shortfall comes mainly from round-8 judgment shifts on retained pixels, not from product defects.
- It does not convert that finding into a pass.
- Only a future, explicitly commissioned Opus scoring pass can change the Opus record.
  - That pass would be primed by this document and must say so.
  - Whether to commission it, or to accept the unmet gate, is the parent's and owner's decision.

## Native supplement (Chrome 154, DPR 2, not the scored Chromium 153 at DPR 1)

**Procedure:**

- Ran `preview_status`, then opened my own incognito tab with `reuseExistingTab: false` on fixture 18431.
- Entered credentials through the snapshot refs, waited for "Security check verified.", submitted, and waited for `navigation[name='Primary']` before any route.
- Did not touch the parent tab.

**Appearance:**

- 1 Primary nav and 0 horizontal overflow.
- 23 radios, with `system` checked.
- Radio `appearance: auto`, 16×16, `accent-color` rgb(191,63,24).
- Each option is a `LABEL` card, 186×120, `cursor: pointer`, with border `rgba(24,35,38,.38)`.
- The selected card's border and 1px outline are rgb(191,63,24).

**Logs, collapsed:**

- Filters summary 40px, outlined.
- Add source and Refresh sources are both `btn-secondary` with a transparent fill.

**State:**

- `ky_theme` was null throughout.
- No theme, configuration or content writes.
- I closed only my own tab.

## Limits

- Visual calibration only.
- **Not verified:** keyboard, focus, contrast, states or responsive gates; other roles; populated Logs/Activity; named-theme rendered contrast; lower page content.
- I ran no build, test or CI. Current-head CI and the exact-head security review are parent-owned and still required.
- I viewed 12 of the 40 originals at full resolution, chosen as the disputed surfaces. The rest were not re-inspected.
- As the same model as the Opus primary, I share its priors. Read this as calibration, not as independent confirmation.
