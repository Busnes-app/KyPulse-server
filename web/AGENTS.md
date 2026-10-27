# Web

## Purpose
React 19 + TypeScript + Vite PWA frontend using the shared ky-ui theme catalog (Busnes light/dark defaults, the suite palettes and the base-family `patina`, `cyber`, `nord`, `paper`, `oled`) and client-side WebCrypto PoW CAPTCHA.

## Ownership
Owns user interface components, service worker caching, PWA installation manifests, and frontend theme switching.

## Local Contracts
- Logs/Activity use the shared page-title geometry, responsive full-width filters and secondary Clear/Refresh actions. Empty-result messages use shared body size and strong ink. Phone alert tables retain explicit table/row/cell roles with visible field labels; all delivery/reason/time data remains available.
- The phone AlertBar shows relative age and cause compactly; each link retains the complete chronology in its accessible name and on desktop. Degraded states consume the shared warning alias, including named palettes.
- Disabled buttons use neutral surfaces and ink; panel headers wrap before squeezing headings.
- Backup keeps its existing pairing/key forms before the operational summary when a key or destination is missing, and below it when ready. Configuration-only local retention/schedule badges are neutral; all values and recovery actions retain their original meaning. Pair leads setup; pin-by-hand and schedule saves remain secondary. Setup cards keep their own content height rather than stretching to match. The fact summary owns key/pairing badges and the local directory/retention values; setup forms do not repeat them.
- Section h2/h3 use the shared section scale. Phone alert labels use the body face even when their values are monospace. Status app names are visibly underlined links. Unpaired collector guidance links to the existing Settings route. Empty app checks do not reserve half of the diagnostic area. Phone silence actions use aligned columns; desktop alert Time columns fit their unbroken timestamp. Activity distinguishes an unfiltered empty result from a filtered empty result using the submitted filter snapshot.
- Newly added suite palettes tint error/success text toward their ink for readable status text on light and dark surfaces; the five shipped base palettes retain their original status values. Browser regressions measure all six new light palettes against both page and panel surfaces. The filled `.btn-danger` takes the page colour (`--bg`) as its label under the same 15 suite palettes, because the tint makes the fill too dark for a dark `--button-text`. Busnes and the five base palettes keep `--button-text`. `browser/danger-button.spec.mjs` measures the rendered label on the real Delete button at rest, hover and focus.
- Web themes default to the Busnes.app cream/light and charcoal/dark palettes with orange accents, following the OS until a browser-local choice is saved. Preserve existing named themes and saved choices.
- A signed-in user with `must_change_password` sees only password replacement and sign-out. Replacement uses `secureFetch`, returns to login after session revocation, and never exposes the normal navigation before completion.
- Strict TypeScript type safety without unused imports.
- The authenticated shell uses a persistent sidebar of `<a>` links; the selected page is marked by a quiet surface, slim accent rail and `aria-current="page"`, with a horizontal overflow navigation on small screens that keeps the current row in view on every route change, deep links included. Repeat the nearest scroll after fonts settle, since font loading can widen a row; cancel stale route callbacks. The header is sticky, so the page does not move; focus stays put. `browser/nav.spec.mjs` covers deep links and delayed fonts.
- App detail (`src/pages/AppDetail.tsx`): Edit is the only page-header action; Delete is the filled `.btn-danger` in a closing Stop watching section with the same confirmation (the F7 spec measures it there). `.dr-facts` are flat label/value rows on the page surface, labels in sentence case; the rows share one subgrid label column (at least 8rem, wider when a label carries a status badge, as on Backup) and `.dr-fact-label` is a wrapping flex row. Consumers: app detail, `KyYardFacts`, `KyYardCard`, Backup. Evidence: `docs/ui-review-followups-2026-10-26/`. Evidence: `docs/ui-visual-refinement-2026-10-26/`.
- Navigation is hash-routed (`#/status`, `#/alerts`, `#/apps/<id>`, `#/backup`, `#/settings`, `#/appearance`, `#/logs`, `#/activity`) by `src/router.ts`; there is no other navigation state. `App` polls `/api/status` every 15 s for the `AlertBar` shown above every page; pages fetch their own data through `src/monitor.ts` or `src/logs.ts` and call `onChanged` after a write. Admin-only controls render only for `role === 'admin'`; the server enforces it.
- `src/theme.ts` adapts the shared catalog (`ky-ui/themes.js`): `Match system` plus every catalog theme, saved by catalog ID under `ky_theme`; older saved IDs (including base `patina`, distinct from suite `patina-ky`) resolve unchanged. It sets `data-theme` and, for named palettes, the colours inline through `applyThemeVariables`; Busnes clears them so the token aliases apply. Every theme control reads the one `useTheme()` store.
- The Appearance page (`src/pages/Appearance.tsx`) is the theme picker for every signed-in role: shared native radio cards (`ky-ui/appearance.css`, previews from `appearance-themes.css`, imported after product CSS). The signed-in shell has no other theme selector; only the sign-in page keeps `ThemeSwitcher`, over the same store. The page uses the shared `ky-appearance-description` copy.
- Authenticated state-changing requests use `secureFetch` so the `ky_csrf` cookie is mirrored into `X-CSRF-Token`.
- Register the service worker from the production JS bundle; keep `script-src 'self'` intact.
- Worker caching is limited to the same-origin public shell, manifest and assets. HTML is network-first with offline fallback so deployments refresh; dynamic/auth routes stay uncached.
- `Backup.tsx` warns for as long as `database_driver` from `/api/backup/status` is not `sqlite`: only the SQLite path can snapshot a database into a capsule, so a Postgres deployment makes no capsules at all.
- `KyYardCard` (Settings, admin only) pairs/unpairs KyYard and shows organization, URL, last pull and state; unpairing here forgets the URL and token locally and does not revoke the token in KyYard. `monitor.kyYardPending` (`paired && !fetched_at && !error`) is the one place that defines "first pull pending"; while paired, `KyYardCard` re-reads `getKyYard()` every 5s (cleared on unmount/unpair). The `AlertBar` shows a neutral "first pull pending" line instead of the stale line while pending. `KyYardFacts` (app detail) shows the linked container's facts, or a hint when unlinked, unpaired, not yet seen in KyYard's inventory, or still pending; memory and restarts only when `has_sample` (with the sample age), otherwise "no sample yet"; "endpoint offline" beside the state. Restart history under one minute says "since monitoring began (under 1 min)". `TargetForm`'s container field offers `kyYardContainers()` suggestions as a datalist (fetched only for admins; a 403/error yields an empty list); an exact link prefills an empty name on blur, after typing has finished, and never overwrites a supplied name.
- `TwoFactorCard` (Settings, every role, only `sso_provider === 'local'`) drives `/api/auth/mfa/enroll`, `/enroll/confirm` and `/disable` through `secureFetch`. No QR library is bundled: the `otpauth://` URI and key are shown as read-only text. Recovery codes live in component state only, are shown once after confirm and dropped on dismiss or unmount. Disable asks for the password and a code or recovery code.
- `WebhookForm` never shows the saved URL (it can be the credential): the server reports only `url_origin`, the URL field starts empty with that origin as placeholder, and every save re-enters it. It explains that Gotify requires an app token and offers saved-token removal only for other presets. Blank-token reuse requires the same preset, URL scheme and host; the server validates edits before saving. Changing presets resets the removal checkbox.


### Admin log screens

- `KyYardCollectors` displays separate inventory/container-log/audit-feed last-success ages,
  stale flags and fixed errors in Settings and admin Logs/Activity. The latter pages own a
  cancellable 15 s status subscription. Unsupported audit cursor APIs name the required
  upstream update; the UI explains inclusive replay and possible gaps at the 1,000-line cap.

- `src/logs.ts` checks read/write JSON at the API boundary. Source writes use `secureFetch`;
  a 401/403 clears the authenticated shell. Logs/Activity remount for a new user identity,
  reject viewers before fetching, and abort/ignore superseded or unmounted requests.
- `#/logs` and `#/activity` use explicit Apply/Clear filters and descending-ID Load more,
  capped at 1,000 rendered rows. Filter changes apply only on submission. UTC bounds go to
  the server as RFC3339. Failed requests have retry states distinct from empty results.
- `LogTimeline` owns plain-text message/raw rendering, also used by the admin-only
  app-detail `RecentLogs` request (`target_id`, limit 20). Viewer detail makes no log request.
- `LogSources` binds the watched app before issuing a code. The sender supplies its name;
  the UI name field builds the displayed command only. Codes stay in component state and
  disappear at expiry/unmount. Source tokens are never fetched or rendered; revocation
  requires explicit confirmation. The sender HTTPS origin defaults to the current origin
  only on HTTPS pages; HTTP pages require the operator to supply it. Validate an origin
  without credentials/path/query/fragment before creating a code; use its canonical URL
  in the command. Explain reachable trusted TLS setup; never infer TLS from an HTTP URL.
- Activity displays server burst summaries as screen-only triage hints. Its selected-app
  empty state uses `has_app_events`, so an actor/outcome/time filter cannot falsely label
  an app as lacking retained audit events. All coverage statements name 7-day retention.

## Verification
- Status uses a responsive failure-first list; retain the status, cause, state age, last check, latency, silence label and detail link for each target.
- Browser setup: build the frontend, run `go build -o .browser/server ./cmd/server` at the repo root, then `cd web && npx playwright install chromium && npm run test:browser`. CI also installs browser OS dependencies.
- `browser/monitor.spec.mjs` covers Status, Alerts, app detail and the webhook form against a fake app the spec runs itself; it needs a private IPv4 interface on the runner (the egress guard refuses loopback) and skips without one.
- `make test-web` or `cd web && npm ci && npm test`, then `npm run build` (vitest with jsdom; `src/pages/Backup.test.tsx` renders the recovery screen against a stubbed status route). Commit `web/dist` after a build; CI diffs it.

## Shared browser UI

- `src/ky-ui/` is generated from Busnes-app/ky-ui, pinned by `VERSION` file hashes. Change shared colors, navigation states and storage helpers upstream, then run its consumer sync with an explicit worktree map; do not hand-edit vendored files.
- Shell geometry (sidebar width, 40px navigation and control rows, 8px radius, title/body type) reads ky-ui tokens in the suite-shell block at the end of `src/styles/theme.css`.
- Shared files include `appearance.css`, `appearance-themes.css` and `themes.js`; the catalog and palette values come only from there. Products own layout, routes and saved choice keys. Busnes aliases consume shared tokens; mark primary navigation with `ky-nav-item` while preserving current-page semantics.
- Verify vendored files with `node src/ky-ui/check-vendor.mjs` from this document's directory. Builds/CI run that check. Rendered evidence and capture limitations are recorded in the repository-root `UI-VERIFICATION.md`; the fleet Appearance stages (screenshots, manifests with hashes and behaviour checks, results CSV, reproducible `capture.mjs`) live in `docs/ui-fleet-2026-10-26/`.

## Child DOX Index
- [browser/AGENTS.md](./browser/AGENTS.md): Production-server browser regression harness and disposable test data.
