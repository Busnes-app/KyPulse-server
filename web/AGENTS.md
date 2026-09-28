# Web

## Purpose
React 19 + TypeScript + Vite PWA frontend embedding KySecurity color tokens (Busnes light/dark defaults plus `Patina Ky`, `Cyber`, `Nord`, `Paper`, `OLED`) and client-side WebCrypto PoW CAPTCHA.

## Ownership
Owns user interface components, service worker caching, PWA installation manifests, and frontend theme switching.

## Local Contracts
- Web themes default to the Busnes.app cream/light and charcoal/dark palettes with orange accents, following the OS until a browser-local choice is saved. Preserve existing named themes and saved choices.
- A signed-in user with `must_change_password` sees only password replacement and sign-out. Replacement uses `secureFetch`, returns to login after session revocation, and never exposes the normal navigation before completion.
- Strict TypeScript type safety without unused imports.
- The authenticated shell uses a persistent sidebar of `<a>` links; the selected page is marked by a quiet surface, slim accent rail and `aria-current="page"`, with a horizontal overflow navigation on small screens.
- Navigation is hash-routed (`#/status`, `#/alerts`, `#/apps/<id>`, `#/backup`, `#/settings`, `#/logs`, `#/activity`) by `src/router.ts`; there is no other navigation state. `App` polls `/api/status` every 15 s for the `AlertBar` shown above every page; pages fetch their own data through `src/monitor.ts` or `src/logs.ts` and call `onChanged` after a write. Admin-only controls render only for `role === 'admin'`; the server enforces it.
- Dynamic theme selection applies `data-theme` attribute to the root HTML document and persists to `localStorage`.
- Authenticated state-changing requests use `secureFetch` so the `ky_csrf` cookie is mirrored into `X-CSRF-Token`.
- Register the service worker from the production JS bundle; keep `script-src 'self'` intact.
- Worker caching is limited to the same-origin public shell, manifest and assets. HTML is network-first with offline fallback so deployments refresh; dynamic/auth routes stay uncached.
- `Backup.tsx` warns for as long as `database_driver` from `/api/backup/status` is not `sqlite`: only the SQLite path can snapshot a database into a capsule, so a Postgres deployment makes no capsules at all.
- `KyYardCard` (Settings, admin only) pairs/unpairs KyYard and shows organization, URL, last pull and state; unpairing here forgets the URL and token locally and does not revoke the token in KyYard. `monitor.kyYardPending` (`paired && !fetched_at && !error`) is the one place that defines "first pull pending"; while paired, `KyYardCard` re-reads `getKyYard()` every 5s (cleared on unmount/unpair). The `AlertBar` shows a neutral "first pull pending" line instead of the stale line while pending. `KyYardFacts` (app detail) shows the linked container's facts, or a hint when unlinked, unpaired, not yet seen in KyYard's inventory, or still pending; memory and restarts only when `has_sample` (with the sample age), otherwise "no sample yet"; "endpoint offline" beside the state. `TargetForm`'s container field offers `kyYardContainers()` suggestions as a datalist (fetched only for admins; a 403/error yields an empty list); picking an exact link prefills an empty name.


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
- Browser setup: build the frontend, run `go build -o .browser/server ./cmd/server` at the repo root, then `cd web && npx playwright install chromium && npm run test:browser`. CI also installs browser OS dependencies.
- `browser/monitor.spec.mjs` covers Status, Alerts, app detail and the webhook form against a fake app the spec runs itself; it needs a private IPv4 interface on the runner (the egress guard refuses loopback) and skips without one.
- `make test-web` or `cd web && npm ci && npm test`, then `npm run build` (vitest with jsdom; `src/pages/Backup.test.tsx` renders the recovery screen against a stubbed status route). Commit `web/dist` after a build; CI diffs it.

## Shared browser UI

- `src/ky-ui/` is generated from Busnes-app/ky-ui, pinned by `VERSION` file hashes. Change shared colors, navigation states and storage helpers upstream, then run its consumer sync with an explicit worktree map; do not hand-edit vendored files.
- Products own layout, routes, saved choice keys and named palettes. Busnes aliases consume shared tokens; mark primary navigation with `ky-nav-item` while preserving current-page semantics.
- Verify vendored files with `node src/ky-ui/check-vendor.mjs` from this document's directory. Builds/CI run that check. Rendered evidence and capture limitations are recorded in the repository-root `UI-VERIFICATION.md`.

## Child DOX Index
- [browser/AGENTS.md](./browser/AGENTS.md): Production-server browser regression harness and disposable test data.
