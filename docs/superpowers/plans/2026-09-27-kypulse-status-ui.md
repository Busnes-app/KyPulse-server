# kyPulse Status and Alerts UI (step 2c) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the scaffold Overview with kyPulse's own screens: an alert bar on every page, a Status tab (grid of watched apps), an Alerts tab, an app detail page with silence controls, admin target add/edit, and the webhook form on Settings, all reachable by hash routes.

**Architecture:** The React shell gets a tiny hash router (`#/status`, `#/alerts`, `#/apps/<id>`, `#/backup`, `#/settings`) so alert messages can deep-link to an app. `App` polls `GET /api/status` every 15 s and feeds the `AlertBar`; each page fetches its own data through typed helpers in `web/src/monitor.ts` and asks `App` to re-poll after a write. Admin-only controls render only for `user.role === 'admin'`; the server already answers 403 to viewers, the UI just does not offer them.

**Tech Stack:** React 19, TypeScript strict, Vite 6, vitest + @testing-library/react (jsdom), Playwright 1.63 against the real Go server, lucide-react icons, existing `theme.css` tokens. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-kypulse-design.md` (Screens; §2 Watched apps, State machine, Webhook; Testing → Browser; Build order step 2 bullet 3). Backend contract: `internal/api/AGENTS.md` → Monitoring table.

## Global Constraints

- Strict TypeScript, `noUnusedLocals`/`noUnusedParameters`: every import is used. `npm run build` (tsc -b + vite) must pass after every task.
- Every state-changing request goes through `secureFetch` from `web/src/api.ts` (CSRF header). Reads use plain `fetch`.
- The webhook token is write-only: never rendered, never echoed back into an input value.
- Alert bar copy (verbatim): green `All {n} apps healthy`, red one line per problem `{name} {state} since {time}` , delivery `Alerts not being delivered`.
- States and colours: `ok` green, `basic` green with the word "basic", `degraded` amber, `down` red, `pending` grey, paused (enabled=false) grey with "paused".
- Silence options are exactly `1h`, `8h`, `until_fixed`, `off`; the API takes `{"for": ...}`.
- Interval: default 30 s, minimum 10 s, maximum 3600 s (server validates; the form uses `min=10 max=3600`).
- `web/src/ky-ui/` is vendored: never edit it. Primary navigation keeps `ky-nav-item` and `aria-current="page"`.
- Only `#/status` and `#/alerts` (plus `#/apps/<id>` and `#/settings`) are reachable by a viewer; `#/backup` is admin-only and redirects a viewer to `#/status`. No Logs or Activity tabs in this step.
- `web/dist` is rebuilt and committed in the last task; CI diffs it.
- No Docker, no new server routes. The backend is complete for this step.

## Review Focus

1. **Unknown app id in `#/apps/<id>`** (deleted target, mistyped link from an old alert): the page must show "App not found" with a link back to Status, not spin forever or throw. Pinned in Task 5.
2. **Status response with no targets** (fresh install): the alert bar must say there is nothing watched yet and, for an admin, point at the add form; never "All 0 apps healthy". Pinned in Task 2.
3. **Webhook edit that keeps the token**: saving with an empty token field and an unchanged preset/host must not send `clear_token`; the form must not show the token as ever having been readable. Pinned in Task 6.
4. **Viewer role**: no Add/Edit/Delete/Silence/Webhook controls render for a viewer, and `#/backup` typed by hand lands on Status. Pinned in Tasks 2, 3, 5, 6.
5. **Session expiry mid-page** (a 401 from any read): the page must show "Signed out" and offer the login screen, not a blank grid. Pinned in Task 1 (`readJSON` throws `ApiError` with status) and Task 2 (App reloads on 401).

---

### Task 1: Hash router and typed monitor client

**Files:**
- Create: `web/src/router.ts`
- Create: `web/src/monitor.ts`
- Test: `web/src/router.test.ts`
- Test: `web/src/monitor.test.ts`

**Interfaces:**
- Produces: `useHashRoute(): Route` where `Route = { path: string; parts: string[] }` (`path` always starts with `/`, default `/status`); `navigate(path: string): void`; `hrefFor(path: string): string` (returns `#` + path).
- Produces (monitor.ts): types `TargetState`, `Target`, `TargetEvent`, `StatusSummary`, `Problem`, `WebhookInfo`, `DeliveryStatus`, `HealthCheck`, `LastResult`, `TargetInput`; class `ApiError extends Error { status: number }`; functions `getStatus`, `listTargets`, `getTarget`, `listAlerts`, `createTarget`, `updateTarget`, `deleteTarget`, `silenceTarget`, `getWebhook`, `saveWebhook`, `deleteWebhook`, `testWebhook`; pure helpers `sortTargets`, `stateLabel`, `stateClass`, `sinceLabel`, `timeLabel`, `webhookFailing`.

- [ ] **Step 1: Write the failing router test**

```ts
// web/src/router.test.ts
import { afterEach, describe, expect, it } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { hrefFor, navigate, useHashRoute } from './router';

afterEach(() => { window.location.hash = ''; });

describe('useHashRoute', () => {
  it('defaults to /status when the hash is empty', () => {
    const { result } = renderHook(() => useHashRoute());
    expect(result.current).toEqual({ path: '/status', parts: ['status'] });
  });

  it('follows hashchange and splits the path', () => {
    const { result } = renderHook(() => useHashRoute());
    act(() => { navigate('/apps/tgt_1'); window.dispatchEvent(new HashChangeEvent('hashchange')); });
    expect(result.current).toEqual({ path: '/apps/tgt_1', parts: ['apps', 'tgt_1'] });
    expect(window.location.hash).toBe('#/apps/tgt_1');
  });

  it('normalises a hash without a leading slash', () => {
    window.location.hash = '#alerts';
    const { result } = renderHook(() => useHashRoute());
    expect(result.current.path).toBe('/alerts');
  });

  it('builds hrefs', () => {
    expect(hrefFor('/alerts')).toBe('#/alerts');
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/router.test.ts`
Expected: FAIL, cannot resolve `./router`.

- [ ] **Step 3: Write the router**

```ts
// web/src/router.ts
import { useEffect, useState } from 'react';

export interface Route {
  path: string;
  parts: string[];
}

function parse(hash: string): Route {
  let path = hash.replace(/^#/, '');
  if (!path.startsWith('/')) path = '/' + path;
  if (path === '/') path = '/status';
  return { path, parts: path.split('/').filter(Boolean) };
}

export function currentRoute(): Route {
  return parse(window.location.hash);
}

export function navigate(path: string): void {
  window.location.hash = path.startsWith('/') ? path : '/' + path;
}

export function hrefFor(path: string): string {
  return '#' + path;
}

// useHashRoute re-renders on every hashchange; the hash is the only navigation state.
export function useHashRoute(): Route {
  const [route, setRoute] = useState<Route>(currentRoute);
  useEffect(() => {
    const onChange = () => setRoute(currentRoute());
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
  }, []);
  return route;
}
```

- [ ] **Step 4: Run the router test**

Run: `cd web && npx vitest run src/router.test.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Write the failing monitor client test**

```ts
// web/src/monitor.test.ts
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  ApiError, getStatus, silenceTarget, sinceLabel, sortTargets, stateLabel, webhookFailing, type StatusSummary, type Target,
} from './monitor';

function target(over: Partial<Target>): Target {
  return {
    id: 'tgt_' + over.name, name: 'x', url: 'https://x/', interval_sec: 30, enabled: true, state: 'ok',
    state_since: '2026-09-27T10:00:00Z', until_fixed: false, last_result: '', last_latency_ms: 0,
    created_at: '2026-09-27T09:00:00Z', updated_at: '2026-09-27T09:00:00Z', basic: false, ...over,
  };
}

afterEach(() => vi.unstubAllGlobals());

describe('monitor helpers', () => {
  it('sorts broken apps first, then pending, ok, paused; names break ties', () => {
    const list = [
      target({ name: 'b-ok' }), target({ name: 'paused', enabled: false }), target({ name: 'deg', state: 'degraded' }),
      target({ name: 'down', state: 'down' }), target({ name: 'pend', state: 'pending' }), target({ name: 'a-ok' }),
    ];
    expect(sortTargets(list).map((t) => t.name)).toEqual(['down', 'deg', 'pend', 'a-ok', 'b-ok', 'paused']);
  });

  it('labels states, basic and paused', () => {
    expect(stateLabel(target({ name: 'a' }))).toBe('ok');
    expect(stateLabel(target({ name: 'a', basic: true }))).toBe('ok (basic)');
    expect(stateLabel(target({ name: 'a', enabled: false }))).toBe('paused');
    expect(stateLabel(target({ name: 'a', state: 'down' }))).toBe('down');
  });

  it('formats durations', () => {
    const now = new Date('2026-09-27T12:00:00Z');
    expect(sinceLabel('2026-09-27T11:59:30Z', now)).toBe('30s');
    expect(sinceLabel('2026-09-27T11:15:00Z', now)).toBe('45m');
    expect(sinceLabel('2026-09-27T09:10:00Z', now)).toBe('2h 50m');
    expect(sinceLabel('2026-09-25T09:10:00Z', now)).toBe('2d 2h');
  });

  it('reports delivery failure only when the last delivery failed', () => {
    const base: StatusSummary = { checked_at: null, total: 1, ok: 1, degraded: 0, down: 0, pending: 0, paused: 0, problems: [], webhook: { configured: true, last: null } };
    expect(webhookFailing(base)).toBe(false);
    expect(webhookFailing({ ...base, webhook: { configured: true, last: { at: 'x', ok: true } } })).toBe(false);
    expect(webhookFailing({ ...base, webhook: { configured: true, last: { at: 'x', ok: false, error: 'receiver_500' } } })).toBe(true);
    expect(webhookFailing({ ...base, webhook: { configured: false, last: { at: 'x', ok: false, error: 'receiver_500' } } })).toBe(false);
  });
});

describe('monitor client', () => {
  it('throws ApiError with the status and server message', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'Session expired' }), { status: 401 })));
    await expect(getStatus()).rejects.toMatchObject({ status: 401, message: 'Session expired' });
    await expect(getStatus()).rejects.toBeInstanceOf(ApiError);
  });

  it('posts a silence with the CSRF header', async () => {
    document.cookie = 'ky_csrf=abc';
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ silenced_until: null, until_fixed: true }), { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    const out = await silenceTarget('tgt_1', 'until_fixed');
    expect(out.until_fixed).toBe(true);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/targets/tgt_1/silence');
    expect(init.method).toBe('POST');
    expect(new Headers(init.headers).get('X-CSRF-Token')).toBe('abc');
    expect(init.body).toBe(JSON.stringify({ for: 'until_fixed' }));
  });
});
```

- [ ] **Step 6: Run it to verify it fails**

Run: `cd web && npx vitest run src/monitor.test.ts`
Expected: FAIL, cannot resolve `./monitor`.

- [ ] **Step 7: Write the monitor client**

```ts
// web/src/monitor.ts
// Types and calls for the monitoring routes (internal/api/AGENTS.md → Monitoring), plus the
// pure display rules the Status, Alerts and detail screens share.
import { secureFetch } from './api';

export type TargetState = 'pending' | 'ok' | 'degraded' | 'down';

export interface Target {
  id: string;
  name: string;
  url: string;
  interval_sec: number;
  enabled: boolean;
  container?: string;
  state: TargetState;
  state_since: string;
  cause?: string;
  silenced_until?: string | null;
  until_fixed: boolean;
  last_result: string;
  last_polled_at?: string | null;
  last_latency_ms: number;
  created_at: string;
  updated_at: string;
  basic: boolean;
}

export interface HealthCheck {
  name: string;
  status: 'ok' | 'degraded' | 'down';
  reason?: string;
}

export interface LastResult {
  state: TargetState;
  basic?: boolean;
  cause?: string;
  service?: string;
  checks?: HealthCheck[];
}

export interface TargetEvent {
  id: number;
  target_id: string;
  at: string;
  from: string;
  to: string;
  cause?: string;
  reminder: boolean;
  notified: boolean;
  notify_error?: string;
}

export interface Problem {
  id: string;
  name: string;
  state: TargetState;
  since: string;
  cause?: string;
}

export interface DeliveryStatus {
  at: string;
  ok: boolean;
  error?: string;
}

export interface StatusSummary {
  checked_at: string | null;
  total: number;
  ok: number;
  degraded: number;
  down: number;
  pending: number;
  paused: number;
  problems: Problem[] | null;
  webhook: { configured: boolean; last: DeliveryStatus | null };
}

export interface WebhookInfo {
  configured: boolean;
  preset?: string;
  url?: string;
  has_token?: boolean;
  last?: DeliveryStatus | null;
}

export interface TargetInput {
  name: string;
  url: string;
  interval_sec: number;
  enabled: boolean;
}

export type SilenceFor = '1h' | '8h' | 'until_fixed' | 'off';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function readJSON<T>(resp: Response): Promise<T> {
  if (resp.status === 204) return undefined as T;
  const body = await resp.json().catch(() => ({}));
  if (!resp.ok) throw new ApiError(resp.status, (body as { error?: string }).error || `Request failed (${resp.status})`);
  return body as T;
}

const json = (body: unknown): RequestInit => ({
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
});

export const getStatus = () => fetch('/api/status').then((r) => readJSON<StatusSummary>(r));
export const listTargets = () => fetch('/api/targets').then((r) => readJSON<{ targets: Target[] }>(r)).then((b) => b.targets ?? []);
export const getTarget = (id: string) =>
  fetch(`/api/targets/${encodeURIComponent(id)}`).then((r) => readJSON<{ target: Target; last_result: LastResult | null; events: TargetEvent[] | null }>(r));
export const listAlerts = (limit = 100) =>
  fetch(`/api/alerts?limit=${limit}`).then((r) => readJSON<{ events: TargetEvent[] | null; total: number }>(r));
export const createTarget = (input: TargetInput) =>
  secureFetch('/api/targets', { method: 'POST', ...json(input) }).then((r) => readJSON<{ target: Target }>(r)).then((b) => b.target);
export const updateTarget = (id: string, input: TargetInput) =>
  secureFetch(`/api/targets/${encodeURIComponent(id)}`, { method: 'PUT', ...json(input) }).then((r) => readJSON<{ target: Target }>(r)).then((b) => b.target);
export const deleteTarget = (id: string) =>
  secureFetch(`/api/targets/${encodeURIComponent(id)}`, { method: 'DELETE' }).then((r) => readJSON<void>(r));
export const silenceTarget = (id: string, f: SilenceFor) =>
  secureFetch(`/api/targets/${encodeURIComponent(id)}/silence`, { method: 'POST', ...json({ for: f }) })
    .then((r) => readJSON<{ silenced_until: string | null; until_fixed: boolean }>(r));
export const getWebhook = () => fetch('/api/alerts/webhook').then((r) => readJSON<WebhookInfo>(r));
export const saveWebhook = (input: { preset: string; url: string; token: string; clear_token: boolean }) =>
  secureFetch('/api/alerts/webhook', { method: 'PUT', ...json(input) }).then((r) => readJSON<WebhookInfo>(r));
export const deleteWebhook = () => secureFetch('/api/alerts/webhook', { method: 'DELETE' }).then((r) => readJSON<void>(r));
export const testWebhook = () => secureFetch('/api/alerts/webhook/test', { method: 'POST' }).then((r) => readJSON<{ ok: boolean }>(r));

// Display rules.

const rank: Record<string, number> = { down: 0, degraded: 1, pending: 2, ok: 3 };

// sortTargets puts broken apps first, paused last, names as the tie-break.
export function sortTargets(list: Target[]): Target[] {
  return [...list].sort((a, b) => {
    const ra = a.enabled ? rank[a.state] ?? 3 : 9;
    const rb = b.enabled ? rank[b.state] ?? 3 : 9;
    return ra - rb || a.name.localeCompare(b.name);
  });
}

export function stateLabel(t: Target): string {
  if (!t.enabled) return 'paused';
  if (t.state === 'ok' && t.basic) return 'ok (basic)';
  return t.state;
}

// stateClass is the CSS suffix for a dot or badge: ok|degraded|down|pending|paused.
export function stateClass(t: Pick<Target, 'state' | 'enabled'>): string {
  return t.enabled ? t.state : 'paused';
}

export function sinceLabel(iso: string, now: Date = new Date()): string {
  const s = Math.max(0, Math.floor((now.getTime() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

export function timeLabel(iso: string | null | undefined): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

export function webhookFailing(s: StatusSummary): boolean {
  return s.webhook.configured && !!s.webhook.last && !s.webhook.last.ok;
}

export function isSilenced(t: Target, now: Date = new Date()): boolean {
  if (t.until_fixed) return true;
  return !!t.silenced_until && new Date(t.silenced_until).getTime() > now.getTime();
}
```

- [ ] **Step 8: Run both tests and the build**

Run: `cd web && npx vitest run src/router.test.ts src/monitor.test.ts && npm run build`
Expected: PASS (10 tests); build succeeds. If `renderHook` is missing, it is exported by `@testing-library/react` 16 — check the import, not the package.

- [ ] **Step 9: Commit**

```bash
git add web/src/router.ts web/src/router.test.ts web/src/monitor.ts web/src/monitor.test.ts
git commit -m "web: hash router and typed monitor client"
```

---

### Task 2: Alert bar and the routed shell

**Files:**
- Create: `web/src/components/AlertBar.tsx`
- Test: `web/src/components/AlertBar.test.tsx`
- Modify: `web/src/App.tsx` (whole file)
- Modify: `web/src/components/AppHeader.tsx` (whole file)
- Delete: `web/src/pages/Dashboard.tsx`
- Modify: `web/src/styles/theme.css` (append)
- Create: `web/src/pages/Status.tsx` (placeholder, replaced in Task 3)
- Create: `web/src/pages/Alerts.tsx` (placeholder, replaced in Task 4)
- Create: `web/src/pages/AppDetail.tsx` (placeholder, replaced in Task 5)

**Interfaces:**
- Consumes: `useHashRoute`, `navigate`, `hrefFor` (Task 1); `getStatus`, `StatusSummary`, `webhookFailing`, `timeLabel`, `sinceLabel`, `ApiError` (Task 1).
- Produces: `AlertBar({ status, loading, isAdmin })`; page props contract used by Tasks 3–5: `Status({ user, onChanged })`, `Alerts({ user })`, `AppDetail({ id, user, onChanged })` where `user` is `{ role: string }` and `onChanged: () => void` asks App to re-poll status.

- [ ] **Step 1: Write the failing AlertBar test**

```tsx
// web/src/components/AlertBar.test.tsx
import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { AlertBar } from './AlertBar';
import type { StatusSummary } from '../monitor';

const healthy: StatusSummary = {
  checked_at: '2026-09-27T10:00:00Z', total: 3, ok: 3, degraded: 0, down: 0, pending: 0, paused: 0, problems: [],
  webhook: { configured: true, last: { at: '2026-09-27T09:00:00Z', ok: true } },
};

afterEach(cleanup);

describe('AlertBar', () => {
  it('is green with the app count when all are healthy', () => {
    render(<AlertBar status={healthy} loading={false} isAdmin={false} />);
    const bar = screen.getByRole('status');
    expect(bar.className).toContain('alert-bar-ok');
    expect(bar.textContent).toContain('All 3 apps healthy');
  });

  it('is red with one line per problem and a link to Alerts', () => {
    render(<AlertBar status={{ ...healthy, ok: 1, down: 1, degraded: 1, problems: [
      { id: 'tgt_a', name: 'KyVault', state: 'down', since: '2026-09-27T09:30:00Z', cause: 'refused' },
      { id: 'tgt_b', name: 'KyPost', state: 'degraded', since: '2026-09-27T09:40:00Z' },
    ] }} loading={false} isAdmin={false} />);
    const bar = screen.getByRole('alert');
    expect(bar.className).toContain('alert-bar-down');
    expect(screen.getByRole('link', { name: /KyVault down/ }).getAttribute('href')).toBe('#/apps/tgt_a');
    expect(screen.getByRole('link', { name: /KyPost degraded/ })).toBeTruthy();
    expect(screen.getByRole('link', { name: 'Alerts' }).getAttribute('href')).toBe('#/alerts');
  });

  it('says alerts are not being delivered when the last delivery failed', () => {
    render(<AlertBar status={{ ...healthy, webhook: { configured: true, last: { at: 'x', ok: false, error: 'receiver_500' } } }} loading={false} isAdmin={true} />);
    expect(screen.getByText(/Alerts not being delivered/).textContent).toContain('receiver_500');
  });

  it('says nothing is watched yet on an empty install', () => {
    render(<AlertBar status={{ ...healthy, total: 0, ok: 0, problems: [] }} loading={false} isAdmin={true} />);
    expect(screen.getByRole('status').textContent).toMatch(/No apps watched yet/);
    expect(screen.queryByText(/All 0 apps/)).toBeNull();
  });

  it('shows checking while loading', () => {
    render(<AlertBar status={null} loading={true} isAdmin={false} />);
    expect(screen.getByRole('status').textContent).toMatch(/Checking/);
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/components/AlertBar.test.tsx`
Expected: FAIL, cannot resolve `./AlertBar`.

- [ ] **Step 3: Write the AlertBar**

```tsx
// web/src/components/AlertBar.tsx
import React from 'react';
import { AlertTriangle, CheckCircle2, CircleDashed, Send } from 'lucide-react';
import { hrefFor } from '../router';
import { sinceLabel, timeLabel, webhookFailing, type StatusSummary } from '../monitor';

interface AlertBarProps {
  status: StatusSummary | null;
  loading: boolean;
  isAdmin: boolean;
}

// AlertBar sits above every page: red with one line per problem, green when all is well,
// plus a delivery warning when the webhook keeps failing.
export const AlertBar: React.FC<AlertBarProps> = ({ status, loading, isAdmin }) => {
  if (!status) {
    return (
      <div role="status" className="alert-bar alert-bar-neutral">
        <CircleDashed size={16} />
        <span>{loading ? 'Checking…' : 'Status unavailable'}</span>
      </div>
    );
  }
  const delivery = webhookFailing(status) ? (
    <div className="alert-bar-line alert-bar-delivery">
      <Send size={14} />
      <span>Alerts not being delivered ({status.webhook.last?.error || 'failed'}) · {isAdmin ? <a href={hrefFor('/settings')}>Settings</a> : 'tell an admin'}</span>
    </div>
  ) : null;

  const problems = status.problems ?? [];
  if (problems.length > 0) {
    return (
      <div role="alert" className="alert-bar alert-bar-down">
        {problems.map((p) => (
          <div key={p.id} className="alert-bar-line">
            <AlertTriangle size={16} />
            <a href={hrefFor(`/apps/${p.id}`)}>
              {p.name} {p.state} since {timeLabel(p.since)} ({sinceLabel(p.since)}{p.cause ? `, ${p.cause}` : ''})
            </a>
          </div>
        ))}
        <a className="alert-bar-link" href={hrefFor('/alerts')}>Alerts</a>
        {delivery}
      </div>
    );
  }
  if (status.total === 0) {
    return (
      <div role="status" className="alert-bar alert-bar-neutral">
        <CircleDashed size={16} />
        <span>No apps watched yet{isAdmin ? <> · <a href={hrefFor('/status')}>add one on Status</a></> : null}</span>
      </div>
    );
  }
  const active = status.total - status.paused;
  const pendingNote = status.pending > 0 ? `, ${status.pending} awaiting first check` : '';
  return (
    <div role="status" className="alert-bar alert-bar-ok">
      <div className="alert-bar-line">
        <CheckCircle2 size={16} />
        <span>All {active} apps healthy{pendingNote} · last check {timeLabel(status.checked_at)}</span>
      </div>
      {delivery}
    </div>
  );
};
```

- [ ] **Step 4: Run the AlertBar test**

Run: `cd web && npx vitest run src/components/AlertBar.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 5: Append the CSS**

Append to `web/src/styles/theme.css`:

```css
/* ---------- kyPulse: alert bar, status grid, detail ---------- */

.alert-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 16px; padding: 10px 20px; border-bottom: 1px solid var(--line); font-size: 14px; color: var(--ink-strong); }
.alert-bar-line { display: inline-flex; align-items: center; gap: 8px; }
.alert-bar a { color: inherit; font-weight: 600; }
.alert-bar-ok { background: rgba(16, 185, 129, 0.12); color: var(--success); }
.alert-bar-down { background: rgba(239, 68, 68, 0.14); color: var(--danger); }
.alert-bar-neutral { background: var(--panel-hover); color: var(--ink); }
.alert-bar-delivery { color: var(--ink-strong); background: var(--accent-soft); border: 1px solid var(--accent); border-radius: 6px; padding: 2px 8px; font-size: 13px; }
.alert-bar-link { margin-left: auto; }

.status-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 14px; }
.status-tile { display: flex; flex-direction: column; gap: 6px; text-align: left; background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 14px 16px; color: var(--ink-strong); font-weight: 400; }
.status-tile:hover { background: var(--panel-hover); opacity: 1; }
.status-tile-name { display: flex; align-items: center; gap: 8px; font-weight: 600; font-size: 15px; }
.status-tile-meta { font-size: 12px; color: var(--ink); }
.dot { width: 10px; height: 10px; border-radius: 50%; flex: none; background: var(--ink); }
.dot-ok { background: var(--success); }
.dot-degraded { background: #f59e0b; }
.dot-down { background: var(--danger); }
.dot-pending, .dot-paused { background: var(--line); border: 1px solid var(--ink); }
.state-ok { color: var(--success); }
.state-degraded { color: #b45309; }
.state-down { color: var(--danger); }
.state-pending, .state-paused { color: var(--ink); }

.banner { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; padding: 16px 20px; border-radius: 8px; border: 1px solid var(--line); margin-bottom: 20px; }
.banner-ok { background: rgba(16, 185, 129, 0.12); }
.banner-degraded { background: rgba(245, 158, 11, 0.14); }
.banner-down { background: rgba(239, 68, 68, 0.14); }
.banner-pending, .banner-paused { background: var(--panel-hover); }
.banner h2 { font-size: 20px; text-transform: capitalize; }

.events { width: 100%; border-collapse: collapse; font-size: 13px; }
.events th, .events td { text-align: left; padding: 8px 10px; border-top: 1px solid var(--line); vertical-align: top; }
.events th { color: var(--ink); font-weight: 600; font-size: 12px; text-transform: uppercase; letter-spacing: .04em; border-top: 0; }
.form-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; margin-bottom: 12px; }
.form-grid label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; color: var(--ink); }
.form-error { color: var(--danger); font-size: 13px; margin: 0 0 12px; }
.form-ok { color: var(--success); font-size: 13px; margin: 0 0 12px; }
@media (max-width: 720px) { .alert-bar { padding: 8px 12px; } .alert-bar-link { margin-left: 0; } }
```

- [ ] **Step 6: Replace AppHeader with hash-linked navigation**

```tsx
// web/src/components/AppHeader.tsx
import React from 'react';
import { LogOut, Settings as SettingsIcon, Activity, Bell, Archive } from 'lucide-react';
import { ThemeSwitcher } from './ThemeSwitcher';
import { hrefFor } from '../router';

interface AppHeaderProps {
  appName: string;
  activePath: string;
  user: { role: string; display_name?: string; username?: string } | null;
  onLogout: () => void;
}

export const navItems = [
  { path: '/status', label: 'Status', icon: Activity, admin: false },
  { path: '/alerts', label: 'Alerts', icon: Bell, admin: false },
  { path: '/backup', label: 'Backup', icon: Archive, admin: true },
  { path: '/settings', label: 'Settings & DB', icon: SettingsIcon, admin: false },
];

export const AppHeader: React.FC<AppHeaderProps> = ({ appName, activePath, user, onLogout }) => {
  const items = navItems.filter((item) => !item.admin || user?.role === 'admin');
  return (
    <header className="app-header">
      <div className="app-brand">
        <img src="/app-icon.png" width={28} height={28} alt="" />
        <span>{appName || 'kyPulse'}</span>
      </div>

      <nav className="app-nav" aria-label="Primary">
        {items.map((item) => {
          const Icon = item.icon;
          // An app detail page belongs to Status.
          const active = activePath === item.path || (item.path === '/status' && activePath.startsWith('/apps/'));
          return (
            <a
              key={item.path}
              href={hrefFor(item.path)}
              className={active ? 'ky-nav-item active' : 'ky-nav-item'}
              aria-current={active ? 'page' : undefined}
            >
              <Icon size={16} />
              <span>{item.label}</span>
            </a>
          );
        })}
      </nav>
      <div className="app-header-actions">
        <ThemeSwitcher />

        {user && (
          <div className="app-user">
            <div className="app-user-copy">
              <div style={{ fontWeight: 600, color: 'var(--ink-strong)' }}>{user.display_name || user.username}</div>
              <div style={{ fontSize: '11px', color: 'var(--ink)' }}>{user.role}</div>
            </div>
            <button
              className="btn-secondary app-logout"
              onClick={onLogout}
              title="Sign out"
              aria-label="Sign out"
            >
              <LogOut size={16} />
            </button>
          </div>
        )}
      </div>
    </header>
  );
};
```

Then in `web/src/styles/theme.css` change the two selectors `.app-nav button` and `.app-nav button:hover` (line ~125–126) to `.app-nav a, .app-nav button` and `.app-nav a:hover, .app-nav button:hover`, and inside the `@media (max-width: 720px)` block change `.app-nav button { ... }` to `.app-nav a, .app-nav button { ... }` and `.app-nav button.active` to `.app-nav a.active, .app-nav button.active`. Add after them: `.app-nav a { display: inline-flex; align-items: center; gap: 8px; text-decoration: none; font-weight: 600; font-family: var(--font-sans); }`.

- [ ] **Step 7: Create the three page placeholders**

```tsx
// web/src/pages/Status.tsx
import React from 'react';

export interface PageUser { role: string }

export const Status: React.FC<{ user: PageUser; onChanged: () => void }> = () => (
  <div className="dr-page"><h1>Status</h1></div>
);
```

```tsx
// web/src/pages/Alerts.tsx
import React from 'react';
import type { PageUser } from './Status';

export const Alerts: React.FC<{ user: PageUser }> = () => (
  <div className="dr-page"><h1>Alerts</h1></div>
);
```

```tsx
// web/src/pages/AppDetail.tsx
import React from 'react';
import type { PageUser } from './Status';

export const AppDetail: React.FC<{ id: string; user: PageUser; onChanged: () => void }> = ({ id }) => (
  <div className="dr-page"><h1>{id}</h1></div>
);
```

- [ ] **Step 8: Replace App.tsx with the routed shell**

```tsx
// web/src/App.tsx
import React, { useCallback, useEffect, useState } from 'react';
import { AppHeader } from './components/AppHeader';
import { AlertBar } from './components/AlertBar';
import { Status } from './pages/Status';
import { Alerts } from './pages/Alerts';
import { AppDetail } from './pages/AppDetail';
import { Login } from './pages/Login';
import { ChangePassword } from './pages/ChangePassword';
import { Backup } from './pages/Backup';
import { Settings } from './pages/Settings';
import './styles/theme.css';
import './ky-ui/tokens.css';
import './ky-ui/navigation.css';
import { secureFetch } from './api';
import { navigate, useHashRoute } from './router';
import { ApiError, getStatus, type StatusSummary } from './monitor';

// statusEvery is how often the alert bar re-reads /api/status.
const statusEvery = 15_000;

export const App: React.FC = () => {
  const [user, setUser] = useState<any>(null);
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState<boolean>(true);
  const [settings, setSettings] = useState<any>(null);
  const [status, setStatus] = useState<StatusSummary | null>(null);
  const [statusLoading, setStatusLoading] = useState(false);
  const route = useHashRoute();

  useEffect(() => {
    const checkAuth = async () => {
      try {
        const [authResp, setResp] = [await fetch('/api/auth/me'), await fetch('/api/settings')];
        if (setResp.ok) setSettings(await setResp.json());
        if (authResp.ok) {
          const a = await authResp.json();
          if (a.authenticated) setUser(a.user);
        }
      } catch (err) {
        console.error('Initialization error:', err);
      } finally {
        setLoading(false);
      }
    };
    checkAuth();
  }, []);

  // /api/settings returns more fields once authenticated, so re-read it after login.
  const loadSettings = async () => {
    const resp = await fetch('/api/settings');
    if (resp.ok) setSettings(await resp.json());
  };

  const refreshStatus = useCallback(async () => {
    setStatusLoading(true);
    try {
      setStatus(await getStatus());
    } catch (err) {
      // A 401 means the session ended: back to the login screen, no stale data behind it.
      if (err instanceof ApiError && err.status === 401) {
        setUser(null);
        setNotice('Signed out. Sign in again.');
      }
      setStatus(null);
    } finally {
      setStatusLoading(false);
    }
  }, []);

  const signedIn = !!user && !user.must_change_password;
  useEffect(() => {
    if (!signedIn) return;
    void refreshStatus();
    const timer = window.setInterval(() => void refreshStatus(), statusEvery);
    return () => window.clearInterval(timer);
  }, [signedIn, refreshStatus]);

  // A viewer typing #/backup lands on Status; unknown paths do too.
  const isAdmin = user?.role === 'admin';
  useEffect(() => {
    if (!signedIn) return;
    const known = ['status', 'alerts', 'apps', 'settings', 'backup'];
    const head = route.parts[0] ?? 'status';
    if (!known.includes(head) || (head === 'backup' && !isAdmin) || (head === 'apps' && route.parts.length !== 2)) navigate('/status');
  }, [signedIn, isAdmin, route]);

  const handleLogout = async () => {
    await secureFetch('/api/auth/logout', { method: 'POST' });
    setUser(null);
    setStatus(null);
    navigate('/status');
  };

  if (loading) {
    return (
      <div style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'var(--bg)', color: 'var(--ink)' }}>
        Loading {settings?.app_name || 'kyPulse'}...
      </div>
    );
  }

  if (!user) {
    return (
      <>
      {notice && <p role="status" style={{ padding: 16 }}>{notice}</p>}
      <Login
        appName={settings?.app_name || 'kyPulse'}
        onSuccess={(u) => {
          setNotice('');
          setUser(u);
          void loadSettings();
        }}
      />
      </>
    );
  }

  if (user.must_change_password) {
    return <ChangePassword onLogout={handleLogout} onComplete={() => {
      setUser(null);
      setNotice('Password changed. Sign in with your new password.');
    }} />;
  }

  const head = route.parts[0] ?? 'status';
  return (
    <div className="app-shell">
      <AppHeader
        appName={settings?.app_name || 'kyPulse'}
        activePath={route.path}
        user={user}
        onLogout={handleLogout}
      />

      <main className="app-main">
        <AlertBar status={status} loading={statusLoading} isAdmin={isAdmin} />
        {head === 'status' && <Status user={user} onChanged={refreshStatus} />}
        {head === 'alerts' && <Alerts user={user} />}
        {head === 'apps' && route.parts[1] && <AppDetail id={route.parts[1]} user={user} onChanged={refreshStatus} />}
        {head === 'backup' && isAdmin && <Backup />}
        {head === 'settings' && <Settings settings={settings} user={user} onChanged={refreshStatus} />}
      </main>
    </div>
  );
};
```

Then in `web/src/pages/Settings.tsx` change the props interface to

```tsx
interface SettingsProps {
  settings: any;
  user: { role: string };
  onChanged: () => void;
}

export const Settings: React.FC<SettingsProps> = ({ settings }) => {
```

(`user` and `onChanged` are wired in Task 6; until then they are declared but unused in the destructuring, which strict TS permits because they are not destructured.)

Delete `web/src/pages/Dashboard.tsx`.

- [ ] **Step 9: Build and run all web tests**

Run: `cd web && npm run build && npx vitest run`
Expected: build OK; all tests pass (Backup, ChangePassword, router, monitor, AlertBar).

- [ ] **Step 10: Commit**

```bash
git add -A web/src
git commit -m "web: alert bar, hash-routed shell, Status/Alerts/Backup/Settings tabs"
```

---

### Task 3: Status page with target form

**Files:**
- Create: `web/src/components/TargetForm.tsx`
- Modify: `web/src/pages/Status.tsx` (whole file)
- Test: `web/src/pages/Status.test.tsx`

**Interfaces:**
- Consumes: `listTargets`, `createTarget`, `sortTargets`, `stateLabel`, `stateClass`, `sinceLabel`, `timeLabel`, `isSilenced`, `ApiError`, `Target`, `TargetInput` (Task 1); `hrefFor` (Task 1).
- Produces: `TargetForm({ initial?: Target; onSubmit(input: TargetInput): Promise<void>; onCancel(): void; submitLabel: string })`, reused by Task 5 for edit.

- [ ] **Step 1: Write the failing Status test**

```tsx
// web/src/pages/Status.test.tsx
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Status } from './Status';
import type { Target } from '../monitor';

function target(over: Partial<Target>): Target {
  return {
    id: 'tgt_' + over.name, name: 'x', url: 'https://x/', interval_sec: 30, enabled: true, state: 'ok',
    state_since: '2026-09-27T10:00:00Z', until_fixed: false, last_result: '', last_latency_ms: 12,
    created_at: '2026-09-27T09:00:00Z', updated_at: '2026-09-27T09:00:00Z', basic: false, ...over,
  };
}

const list = [target({ name: 'KyPost' }), target({ name: 'KyVault', state: 'down', cause: 'refused' }), target({ name: 'KyYard', basic: true })];

function stub(calls: Array<[RegExp, unknown, number?]>) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    for (const [re, body, status] of calls) if (re.test(`${init?.method ?? 'GET'} ${url}`)) return new Response(JSON.stringify(body), { status: status ?? 200 });
    throw new Error(`unexpected fetch ${init?.method ?? 'GET'} ${url}`);
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('Status', () => {
  it('lists broken apps first with a dot and links to the detail page', async () => {
    stub([[/GET \/api\/targets$/, { targets: list }]]);
    render(<Status user={{ role: 'viewer' }} onChanged={() => {}} />);
    const tiles = await screen.findAllByRole('link', { name: /Ky/ });
    expect(tiles.map((t) => t.textContent)).toEqual([
      expect.stringContaining('KyVault'), expect.stringContaining('KyPost'), expect.stringContaining('KyYard'),
    ]);
    expect(tiles[0].getAttribute('href')).toBe('#/apps/tgt_KyVault');
    expect(tiles[0].querySelector('.dot-down')).toBeTruthy();
    expect(tiles[2].textContent).toContain('ok (basic)');
    expect(screen.queryByRole('button', { name: /Add app/ })).toBeNull();
  });

  it('lets an admin add an app and refreshes', async () => {
    const fetchMock = stub([
      [/GET \/api\/targets$/, { targets: [] }],
      [/POST \/api\/targets$/, { target: target({ name: 'New' }) }, 201],
    ]);
    const onChanged = vi.fn();
    render(<Status user={{ role: 'admin' }} onChanged={onChanged} />);
    fireEvent.click(await screen.findByRole('button', { name: /Add app/ }));
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'New' } });
    fireEvent.change(screen.getByLabelText('Health URL'), { target: { value: 'https://new.lan/healthz' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const post = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'POST')!;
    expect(JSON.parse((post[1] as RequestInit).body as string)).toEqual({ name: 'New', url: 'https://new.lan/healthz', interval_sec: 30, enabled: true });
  });

  it('shows the server message when a write is refused', async () => {
    stub([
      [/GET \/api\/targets$/, { targets: [] }],
      [/POST \/api\/targets$/, { error: 'A target with that name exists' }, 409],
    ]);
    render(<Status user={{ role: 'admin' }} onChanged={() => {}} />);
    fireEvent.click(await screen.findByRole('button', { name: /Add app/ }));
    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'Dup' } });
    fireEvent.change(screen.getByLabelText('Health URL'), { target: { value: 'https://dup.lan/' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('A target with that name exists')).toBeTruthy();
  });

  it('tells an empty install what to do', async () => {
    stub([[/GET \/api\/targets$/, { targets: [] }]]);
    render(<Status user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByText(/No apps are watched yet/)).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/pages/Status.test.tsx`
Expected: FAIL (placeholder has no links/buttons).

- [ ] **Step 3: Write the TargetForm**

```tsx
// web/src/components/TargetForm.tsx
import React, { useState } from 'react';
import { Loader2 } from 'lucide-react';
import { ApiError, type Target, type TargetInput } from '../monitor';

interface TargetFormProps {
  initial?: Target;
  submitLabel: string;
  onSubmit: (input: TargetInput) => Promise<void>;
  onCancel: () => void;
}

// TargetForm is the add and edit form for a watched app. Validation lives on the server; the
// form only keeps the numbers in range and shows the server's message.
export const TargetForm: React.FC<TargetFormProps> = ({ initial, submitLabel, onSubmit, onCancel }) => {
  const [name, setName] = useState(initial?.name ?? '');
  const [url, setUrl] = useState(initial?.url ?? '');
  const [interval, setInterval_] = useState(initial?.interval_sec ?? 30);
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await onSubmit({ name: name.trim(), url: url.trim(), interval_sec: interval, enabled });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save the app');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="panel" aria-label={submitLabel}>
      <div className="form-grid">
        <label>Name<input value={name} onChange={(e) => setName(e.target.value)} required maxLength={64} /></label>
        <label>Health URL<input type="url" value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://app.lan/healthz" /></label>
        <label>Interval (seconds)<input type="number" min={10} max={3600} value={interval} onChange={(e) => setInterval_(Number(e.target.value))} required /></label>
        <label style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <input type="checkbox" style={{ width: 'auto' }} checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />Enabled
        </label>
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      <div className="dr-actions">
        <button type="submit" disabled={busy}>{busy ? <Loader2 size={14} className="animate-spin" /> : null}{busy ? 'Saving…' : submitLabel}</button>
        <button type="button" className="btn-secondary" onClick={onCancel} disabled={busy}>Cancel</button>
      </div>
    </form>
  );
};
```

Note: the submit button's accessible name while idle is exactly `submitLabel` ("Save"), which the tests click.

- [ ] **Step 4: Write the Status page**

```tsx
// web/src/pages/Status.tsx
import React, { useCallback, useEffect, useState } from 'react';
import { Activity, Plus, BellOff } from 'lucide-react';
import { TargetForm } from '../components/TargetForm';
import { hrefFor } from '../router';
import {
  ApiError, createTarget, isSilenced, listTargets, sinceLabel, sortTargets, stateClass, stateLabel, timeLabel,
  type Target, type TargetInput,
} from '../monitor';

export interface PageUser { role: string }

interface StatusProps {
  user: PageUser;
  onChanged: () => void;
}

// Status is the home tab: every watched app as a tile, broken ones first.
export const Status: React.FC<StatusProps> = ({ user, onChanged }) => {
  const [targets, setTargets] = useState<Target[] | null>(null);
  const [error, setError] = useState('');
  const [adding, setAdding] = useState(false);
  const isAdmin = user.role === 'admin';

  const load = useCallback(async () => {
    try {
      setTargets(sortTargets(await listTargets()));
      setError('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not load the apps');
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [load]);

  const add = async (input: TargetInput) => {
    await createTarget(input);
    setAdding(false);
    await load();
    onChanged();
  };

  return (
    <div className="dr-page">
      <div className="dr-header">
        <h1 style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Activity size={24} style={{ color: 'var(--accent)' }} />Status</h1>
        {isAdmin && !adding && (
          <button type="button" onClick={() => setAdding(true)}><Plus size={14} />Add app</button>
        )}
      </div>
      {adding && <div className="dr-section"><TargetForm submitLabel="Save" onSubmit={add} onCancel={() => setAdding(false)} /></div>}
      {error && <p className="form-error" role="alert">{error}</p>}
      {targets && targets.length === 0 && !adding && (
        <p className="dr-hint">No apps are watched yet.{isAdmin ? ' Add one to start polling its /healthz.' : ' Ask an admin to add one.'}</p>
      )}
      {targets && targets.length > 0 && (
        <div className="status-grid">
          {targets.map((t) => (
            <a key={t.id} className="status-tile" href={hrefFor(`/apps/${t.id}`)}>
              <span className="status-tile-name"><span className={`dot dot-${stateClass(t)}`} aria-hidden="true" />{t.name}</span>
              <span className={`state-${stateClass(t)}`}>{stateLabel(t)}{t.cause && t.enabled && t.state !== 'ok' ? ` · ${t.cause}` : ''}</span>
              <span className="status-tile-meta">
                {t.state === 'pending' ? 'awaiting first check' : `for ${sinceLabel(t.state_since)}`}
                {t.last_polled_at ? ` · checked ${timeLabel(t.last_polled_at)}` : ''}
                {t.last_polled_at ? ` · ${t.last_latency_ms} ms` : ''}
                {isSilenced(t) ? <> · <BellOff size={12} style={{ verticalAlign: 'middle' }} /> silenced</> : null}
              </span>
            </a>
          ))}
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 5: Run the tests and build**

Run: `cd web && npx vitest run src/pages/Status.test.tsx && npm run build`
Expected: PASS (4 tests), build OK.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/TargetForm.tsx web/src/pages/Status.tsx web/src/pages/Status.test.tsx
git commit -m "web: Status tab with app tiles and admin add form"
```

---

### Task 4: Alerts page

**Files:**
- Modify: `web/src/pages/Alerts.tsx` (whole file)
- Test: `web/src/pages/Alerts.test.tsx`

**Interfaces:**
- Consumes: `listAlerts`, `listTargets`, `isSilenced`, `timeLabel`, `ApiError`, `Target`, `TargetEvent` (Task 1); `hrefFor` (Task 1); `PageUser` (Task 3).

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/pages/Alerts.test.tsx
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { Alerts } from './Alerts';
import type { Target, TargetEvent } from '../monitor';

const targets: Target[] = [{
  id: 'tgt_a', name: 'KyVault', url: 'https://a/', interval_sec: 30, enabled: true, state: 'down', state_since: '2026-09-27T09:30:00Z',
  cause: 'refused', silenced_until: '2999-01-01T00:00:00Z', until_fixed: false, last_result: '', last_latency_ms: 0,
  created_at: 'x', updated_at: 'x', basic: false,
}];
const events: TargetEvent[] = [
  { id: 3, target_id: 'tgt_a', at: '2026-09-27T10:30:00Z', from: 'down', to: 'down', cause: 'refused', reminder: true, notified: false, notify_error: 'receiver_500' },
  { id: 2, target_id: 'tgt_a', at: '2026-09-27T09:30:00Z', from: 'ok', to: 'down', cause: 'refused', reminder: false, notified: true },
  { id: 1, target_id: 'tgt_gone', at: '2026-09-27T08:00:00Z', from: 'pending', to: 'ok', reminder: false, notified: false },
];

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('Alerts', () => {
  it('lists events newest first with app names, transitions and delivery results', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith('/api/alerts')) return new Response(JSON.stringify({ events, total: 3 }));
      if (url === '/api/targets') return new Response(JSON.stringify({ targets }));
      throw new Error(url);
    }));
    render(<Alerts user={{ role: 'viewer' }} />);
    const rows = await screen.findAllByRole('row');
    expect(rows).toHaveLength(4); // header + 3
    expect(rows[1].textContent).toContain('KyVault');
    expect(rows[1].textContent).toContain('reminder');
    expect(rows[1].textContent).toContain('failed: receiver_500');
    expect(rows[2].textContent).toContain('ok → down');
    expect(rows[2].textContent).toContain('sent');
    expect(rows[3].textContent).toContain('tgt_gone'); // deleted target: id shown, nothing crashes
    expect(rows[3].textContent).toContain('not sent');
    expect(screen.getByRole('link', { name: 'KyVault' }).getAttribute('href')).toBe('#/apps/tgt_a');
    expect(screen.getByText(/Silenced/).parentElement?.textContent).toContain('KyVault');
  });

  it('says so when there are no alerts', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.startsWith('/api/alerts')) return new Response(JSON.stringify({ events: null, total: 0 }));
      return new Response(JSON.stringify({ targets: [] }));
    }));
    render(<Alerts user={{ role: 'viewer' }} />);
    expect(await screen.findByText(/No alerts yet/)).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/pages/Alerts.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Write the Alerts page**

```tsx
// web/src/pages/Alerts.tsx
import React, { useEffect, useState } from 'react';
import { Bell, BellOff } from 'lucide-react';
import { hrefFor } from '../router';
import { ApiError, isSilenced, listAlerts, listTargets, timeLabel, type Target, type TargetEvent } from '../monitor';
import type { PageUser } from './Status';

interface AlertsProps { user: PageUser }

function delivery(e: TargetEvent): string {
  if (e.notified) return 'sent';
  if (e.notify_error) return `failed: ${e.notify_error}`;
  return 'not sent';
}

// Alerts lists recent state changes and reminders with their delivery result, and which apps
// are silenced right now.
export const Alerts: React.FC<AlertsProps> = () => {
  const [events, setEvents] = useState<TargetEvent[] | null>(null);
  const [targets, setTargets] = useState<Record<string, Target>>({});
  const [error, setError] = useState('');

  useEffect(() => {
    const load = async () => {
      try {
        const [a, t] = await Promise.all([listAlerts(100), listTargets()]);
        setEvents(a.events ?? []);
        setTargets(Object.fromEntries(t.map((x) => [x.id, x])));
        setError('');
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Could not load alerts');
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, []);

  const silenced = Object.values(targets).filter((t) => isSilenced(t));

  return (
    <div className="dr-page">
      <div className="dr-header">
        <h1 style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Bell size={24} style={{ color: 'var(--accent)' }} />Alerts</h1>
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      {silenced.length > 0 && (
        <p className="dr-hint"><BellOff size={14} /><strong>Silenced:</strong>{' '}
          {silenced.map((t) => (
            <span key={t.id}><a href={hrefFor(`/apps/${t.id}`)}>{t.name}</a>{t.until_fixed ? ' until fixed' : ` until ${timeLabel(t.silenced_until)}`} </span>
          ))}
        </p>
      )}
      {events && events.length === 0 && <p className="dr-hint">No alerts yet. State changes and reminders will show here.</p>}
      {events && events.length > 0 && (
        <div className="panel" style={{ overflowX: 'auto', padding: 0 }}>
          <table className="events">
            <thead><tr><th>Time</th><th>App</th><th>Change</th><th>Reason</th><th>Delivery</th></tr></thead>
            <tbody>
              {events.map((e) => {
                const t = targets[e.target_id];
                return (
                  <tr key={e.id}>
                    <td>{timeLabel(e.at)}</td>
                    <td>{t ? <a href={hrefFor(`/apps/${t.id}`)}>{t.name}</a> : <span className="dr-mono">{e.target_id}</span>}</td>
                    <td className={`state-${e.to}`}>{e.reminder ? `still ${e.to} (reminder)` : `${e.from} → ${e.to}`}</td>
                    <td className="dr-mono">{e.cause || ''}</td>
                    <td>{delivery(e)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};
```

- [ ] **Step 4: Run the test and build**

Run: `cd web && npx vitest run src/pages/Alerts.test.tsx && npm run build`
Expected: PASS (2 tests), build OK.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/Alerts.tsx web/src/pages/Alerts.test.tsx
git commit -m "web: Alerts tab with delivery results and silences"
```

---

### Task 5: App detail page

**Files:**
- Modify: `web/src/pages/AppDetail.tsx` (whole file)
- Test: `web/src/pages/AppDetail.test.tsx`

**Interfaces:**
- Consumes: `getTarget`, `updateTarget`, `deleteTarget`, `silenceTarget`, `stateClass`, `stateLabel`, `sinceLabel`, `timeLabel`, `isSilenced`, `ApiError`, `Target`, `TargetInput`, `LastResult`, `TargetEvent`, `SilenceFor` (Task 1); `TargetForm` (Task 3); `navigate`, `hrefFor` (Task 1); `PageUser` (Task 3).

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/pages/AppDetail.test.tsx
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { AppDetail } from './AppDetail';
import type { Target } from '../monitor';

const target: Target = {
  id: 'tgt_a', name: 'KyVault', url: 'https://vault.lan/healthz', interval_sec: 30, enabled: true, state: 'down',
  state_since: '2026-09-27T09:30:00Z', cause: 'refused', until_fixed: false, last_result: '', last_polled_at: '2026-09-27T10:00:00Z',
  last_latency_ms: 40, created_at: 'x', updated_at: 'x', basic: false,
};
const detail = {
  target,
  last_result: { state: 'down', cause: 'refused', checks: [{ name: 'database', status: 'ok' }, { name: 'kyyard', status: 'down', reason: 'refused' }] },
  events: [{ id: 2, target_id: 'tgt_a', at: '2026-09-27T09:30:00Z', from: 'ok', to: 'down', cause: 'refused', reminder: false, notified: true }],
};

function stub(extra: Array<[RegExp, unknown, number?]> = []) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${String(input)}`;
    for (const [re, body, status] of extra) if (re.test(key)) return new Response(JSON.stringify(body), { status: status ?? 200 });
    if (/^GET \/api\/targets\/tgt_a$/.test(key)) return new Response(JSON.stringify(detail));
    if (/^GET \/api\/targets\/tgt_gone$/.test(key)) return new Response(JSON.stringify({ error: 'Target not found' }), { status: 404 });
    throw new Error(key);
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); window.location.hash = ''; });

describe('AppDetail', () => {
  it('shows the banner, the failing request, checks and history; a viewer gets no controls', async () => {
    stub();
    render(<AppDetail id="tgt_a" user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByRole('heading', { name: 'KyVault' })).toBeTruthy();
    const banner = screen.getByRole('region', { name: 'Current state' });
    expect(banner.className).toContain('banner-down');
    expect(banner.textContent).toContain('refused');
    expect(banner.textContent).toContain('GET https://vault.lan/healthz');
    expect(screen.getByText('kyyard').parentElement?.textContent).toContain('down');
    expect(screen.getAllByRole('row')).toHaveLength(2);
    expect(screen.queryByRole('button', { name: /Silence/ })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull();
  });

  it('lets an admin silence for 8h', async () => {
    const fetchMock = stub([[/^POST \/api\/targets\/tgt_a\/silence$/, { silenced_until: '2026-09-27T18:00:00Z', until_fixed: false }]]);
    const onChanged = vi.fn();
    render(<AppDetail id="tgt_a" user={{ role: 'admin' }} onChanged={onChanged} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Silence 8h' }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const post = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'POST')!;
    expect((post[1] as RequestInit).body).toBe(JSON.stringify({ for: '8h' }));
  });

  it('deletes after confirmation and goes back to Status', async () => {
    stub([[/^DELETE \/api\/targets\/tgt_a$/, null, 204]]);
    vi.stubGlobal('confirm', vi.fn(() => true));
    render(<AppDetail id="tgt_a" user={{ role: 'admin' }} onChanged={() => {}} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(window.location.hash).toBe('#/status'));
  });

  it('says when the app is gone', async () => {
    stub();
    render(<AppDetail id="tgt_gone" user={{ role: 'admin' }} onChanged={() => {}} />);
    expect(await screen.findByText(/App not found/)).toBeTruthy();
    expect(screen.getByRole('link', { name: /Status/ }).getAttribute('href')).toBe('#/status');
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/pages/AppDetail.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Write the detail page**

```tsx
// web/src/pages/AppDetail.tsx
import React, { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, BellOff, Pencil, Trash2 } from 'lucide-react';
import { TargetForm } from '../components/TargetForm';
import { hrefFor, navigate } from '../router';
import {
  ApiError, deleteTarget, getTarget, isSilenced, silenceTarget, sinceLabel, stateClass, stateLabel, timeLabel, updateTarget,
  type LastResult, type SilenceFor, type Target, type TargetEvent, type TargetInput,
} from '../monitor';
import type { PageUser } from './Status';

interface AppDetailProps {
  id: string;
  user: PageUser;
  onChanged: () => void;
}

// AppDetail is one watched app: state banner, the exact request that fails, the checks the
// app reported, alert history and, for admins, silence, edit and delete.
export const AppDetail: React.FC<AppDetailProps> = ({ id, user, onChanged }) => {
  const [target, setTarget] = useState<Target | null>(null);
  const [last, setLast] = useState<LastResult | null>(null);
  const [events, setEvents] = useState<TargetEvent[]>([]);
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState('');
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const isAdmin = user.role === 'admin';

  const load = useCallback(async () => {
    try {
      const d = await getTarget(id);
      setTarget(d.target);
      setLast(d.last_result);
      setEvents(d.events ?? []);
      setError('');
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) setMissing(true);
      else setError(err instanceof ApiError ? err.message : 'Could not load the app');
    }
  }, [id]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [load]);

  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
      await load();
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Request failed');
    } finally {
      setBusy(false);
    }
  };

  const silence = (f: SilenceFor) => act(() => silenceTarget(id, f));
  const save = async (input: TargetInput) => {
    await updateTarget(id, input);
    setEditing(false);
    await load();
    onChanged();
  };
  const remove = async () => {
    if (!target || !window.confirm(`Stop watching ${target.name}? Its alert history is deleted too.`)) return;
    setBusy(true);
    try {
      await deleteTarget(id);
      onChanged();
      navigate('/status');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not delete the app');
      setBusy(false);
    }
  };

  if (missing) {
    return (
      <div className="dr-page">
        <p className="dr-hint">App not found. It may have been removed. <a href={hrefFor('/status')}>Back to Status</a></p>
      </div>
    );
  }
  if (!target) {
    return <div className="dr-page">{error ? <p className="form-error" role="alert">{error}</p> : <p className="dr-hint">Loading…</p>}</div>;
  }

  const cls = stateClass(target);
  const failing = target.enabled && target.state !== 'ok' && target.state !== 'pending';
  return (
    <div className="dr-page">
      <p className="dr-hint"><a href={hrefFor('/status')}><ArrowLeft size={14} style={{ verticalAlign: 'middle' }} /> Status</a></p>
      <div className="dr-header">
        <h1>{target.name}</h1>
        {isAdmin && !editing && (
          <div className="dr-actions">
            <button type="button" className="btn-secondary" onClick={() => setEditing(true)} disabled={busy}><Pencil size={14} />Edit</button>
            <button type="button" className="btn-danger" onClick={remove} disabled={busy}><Trash2 size={14} />Delete</button>
          </div>
        )}
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      {editing && <div className="dr-section"><TargetForm initial={target} submitLabel="Save changes" onSubmit={save} onCancel={() => setEditing(false)} /></div>}

      <section className={`banner banner-${cls}`} aria-label="Current state">
        <span className={`dot dot-${cls}`} aria-hidden="true" />
        <h2 className={`state-${cls}`}>{stateLabel(target)}</h2>
        <span>
          {target.state === 'pending' ? 'awaiting first check' : `for ${sinceLabel(target.state_since)} (since ${timeLabel(target.state_since)})`}
        </span>
        {failing && (
          <span className="dr-mono" style={{ flexBasis: '100%', fontSize: 13 }}>
            GET {target.url} → {target.cause || last?.cause || 'unknown'}
            {target.last_polled_at ? ` · last tried ${timeLabel(target.last_polled_at)} (${target.last_latency_ms} ms)` : ''}
          </span>
        )}
        {isSilenced(target) && (
          <span className="badge dr-badge-muted"><BellOff size={12} />silenced {target.until_fixed ? 'until fixed' : `until ${timeLabel(target.silenced_until)}`}</span>
        )}
      </section>

      {isAdmin && (
        <div className="dr-section">
          <h3>Silence webhook messages</h3>
          <p className="dr-hint">The alert bar and this page keep showing the problem; only webhook messages stop.</p>
          <div className="dr-actions">
            <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('1h')}>Silence 1h</button>
            <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('8h')}>Silence 8h</button>
            <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('until_fixed')}>Silence until fixed</button>
            {isSilenced(target) && <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('off')}>Unsilence</button>}
          </div>
        </div>
      )}

      <div className="dr-two">
        <div className="dr-section">
          <h3>Checks</h3>
          {last?.checks && last.checks.length > 0 ? (
            <div className="dr-checks">
              {last.checks.map((c) => (
                <div key={c.name} className="dr-check">
                  <span className={`dot dot-${c.status}`} aria-hidden="true" />
                  <span className="dr-mono">{c.name}</span>
                  <span className={`state-${c.status}`}>{c.status}{c.reason ? ` · ${c.reason}` : ''}</span>
                </div>
              ))}
            </div>
          ) : (
            <p className="dr-hint">{last?.basic ? 'This app answers with a plain OK and reports no checks.' : 'No checks reported yet.'}</p>
          )}
        </div>
        <div className="dr-section">
          <h3>Settings</h3>
          <div className="dr-facts">
            <div className="dr-fact"><span className="dr-fact-label">Health URL</span><span className="dr-fact-value dr-mono">{target.url}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Interval</span><span className="dr-fact-value">{target.interval_sec} s</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Polling</span><span className="dr-fact-value">{target.enabled ? 'enabled' : 'paused'}</span></div>
          </div>
        </div>
      </div>

      <div className="dr-section">
        <h3>Alert history</h3>
        {events.length === 0 ? <p className="dr-hint">No state changes yet.</p> : (
          <div className="panel" style={{ overflowX: 'auto', padding: 0 }}>
            <table className="events">
              <thead><tr><th>Time</th><th>Change</th><th>Reason</th><th>Delivery</th></tr></thead>
              <tbody>
                {events.map((e) => (
                  <tr key={e.id}>
                    <td>{timeLabel(e.at)}</td>
                    <td className={`state-${e.to}`}>{e.reminder ? `still ${e.to} (reminder)` : `${e.from} → ${e.to}`}</td>
                    <td className="dr-mono">{e.cause || ''}</td>
                    <td>{e.notified ? 'sent' : e.notify_error ? `failed: ${e.notify_error}` : 'not sent'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};
```

- [ ] **Step 4: Run the test and build**

Run: `cd web && npx vitest run src/pages/AppDetail.test.tsx && npm run build`
Expected: PASS (4 tests), build OK.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/AppDetail.tsx web/src/pages/AppDetail.test.tsx
git commit -m "web: app detail page with silence, edit and delete"
```

---

### Task 6: Webhook form on Settings

**Files:**
- Create: `web/src/components/WebhookForm.tsx`
- Test: `web/src/components/WebhookForm.test.tsx`
- Modify: `web/src/pages/Settings.tsx`

**Interfaces:**
- Consumes: `getWebhook`, `saveWebhook`, `deleteWebhook`, `testWebhook`, `timeLabel`, `ApiError`, `WebhookInfo` (Task 1); Settings props `{ settings, user, onChanged }` (Task 2).

- [ ] **Step 1: Write the failing test**

```tsx
// web/src/components/WebhookForm.test.tsx
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { WebhookForm } from './WebhookForm';

const saved = { configured: true, preset: 'ntfy', url: 'https://ntfy.sh/kypulse', has_token: true, last: { at: '2026-09-27T10:00:00Z', ok: false, error: 'receiver_500' } };

function stub(extra: Array<[RegExp, unknown, number?]> = [], info: unknown = saved) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${String(input)}`;
    for (const [re, body, status] of extra) if (re.test(key)) return new Response(JSON.stringify(body), { status: status ?? 200 });
    if (key === 'GET /api/alerts/webhook') return new Response(JSON.stringify(info));
    throw new Error(key);
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('WebhookForm', () => {
  it('never renders the token and shows the last delivery', async () => {
    stub();
    const { container } = render(<WebhookForm onChanged={() => {}} />);
    expect(await screen.findByDisplayValue('https://ntfy.sh/kypulse')).toBeTruthy();
    const token = screen.getByLabelText(/Token/) as HTMLInputElement;
    expect(token.value).toBe('');
    expect(token.type).toBe('password');
    expect(token.placeholder).toMatch(/saved/i);
    expect(container.textContent).toContain('receiver_500');
  });

  it('keeps the saved token when the field is left empty', async () => {
    const fetchMock = stub([[/^PUT \/api\/alerts\/webhook$/, { ...saved, last: null }]]);
    const onChanged = vi.fn();
    render(<WebhookForm onChanged={onChanged} />);
    await screen.findByDisplayValue('https://ntfy.sh/kypulse');
    fireEvent.click(screen.getByRole('button', { name: 'Save webhook' }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const put = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'PUT')!;
    expect(JSON.parse((put[1] as RequestInit).body as string)).toEqual({ preset: 'ntfy', url: 'https://ntfy.sh/kypulse', token: '', clear_token: false });
  });

  it('sends clear_token when asked', async () => {
    const fetchMock = stub([[/^PUT \/api\/alerts\/webhook$/, { ...saved, has_token: false }]]);
    render(<WebhookForm onChanged={() => {}} />);
    await screen.findByDisplayValue('https://ntfy.sh/kypulse');
    fireEvent.click(screen.getByLabelText(/Remove the saved token/));
    fireEvent.click(screen.getByRole('button', { name: 'Save webhook' }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => (init as RequestInit)?.method === 'PUT')).toBe(true));
    const put = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'PUT')!;
    expect(JSON.parse((put[1] as RequestInit).body as string).clear_token).toBe(true);
  });

  it('shows the reason when a test send fails', async () => {
    stub([[/^POST \/api\/alerts\/webhook\/test$/, { error: 'receiver_500' }, 502]]);
    render(<WebhookForm onChanged={() => {}} />);
    await screen.findByDisplayValue('https://ntfy.sh/kypulse');
    fireEvent.click(screen.getByRole('button', { name: 'Send test' }));
    expect(await screen.findByText(/Test failed: receiver_500/)).toBeTruthy();
  });

  it('starts empty when nothing is configured', async () => {
    stub([], { configured: false });
    render(<WebhookForm onChanged={() => {}} />);
    expect(await screen.findByText(/No webhook configured/)).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Remove webhook' })).toBeNull();
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd web && npx vitest run src/components/WebhookForm.test.tsx`
Expected: FAIL, cannot resolve `./WebhookForm`.

- [ ] **Step 3: Write the form**

```tsx
// web/src/components/WebhookForm.tsx
import React, { useEffect, useState } from 'react';
import { Send, Trash2 } from 'lucide-react';
import { ApiError, deleteWebhook, getWebhook, saveWebhook, testWebhook, timeLabel, type WebhookInfo } from '../monitor';

const presets = [
  { id: 'ntfy', label: 'ntfy', hint: 'Topic URL, e.g. https://ntfy.sh/kypulse. Token is optional (access token).' },
  { id: 'gotify', label: 'Gotify', hint: 'Server URL. The app token goes in the Token field, never in the URL.' },
  { id: 'discord', label: 'Discord', hint: 'Channel webhook URL. No token.' },
  { id: 'generic', label: 'Generic JSON', hint: 'Any HTTPS endpoint; token is sent as a Bearer header when set.' },
];

interface WebhookFormProps { onChanged: () => void }

// WebhookForm configures the one outbound alert webhook. The token is write-only: the server
// only reports whether one is saved, and an empty field keeps it.
export const WebhookForm: React.FC<WebhookFormProps> = ({ onChanged }) => {
  const [info, setInfo] = useState<WebhookInfo | null>(null);
  const [preset, setPreset] = useState('ntfy');
  const [url, setUrl] = useState('');
  const [token, setToken] = useState('');
  const [clearToken, setClearToken] = useState(false);
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const apply = (w: WebhookInfo) => {
    setInfo(w);
    if (w.configured) {
      setPreset(w.preset ?? 'ntfy');
      setUrl(w.url ?? '');
    }
    setToken('');
    setClearToken(false);
  };

  useEffect(() => {
    getWebhook().then(apply).catch((err) => setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not load the webhook' }));
  }, []);

  const run = async (fn: () => Promise<WebhookInfo | void>, ok: string) => {
    setBusy(true);
    setMessage(null);
    try {
      const out = await fn();
      if (out) apply(out);
      setMessage({ kind: 'ok', text: ok });
      onChanged();
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Request failed' });
    } finally {
      setBusy(false);
    }
  };

  const save = (e: React.FormEvent) => {
    e.preventDefault();
    void run(() => saveWebhook({ preset, url: url.trim(), token, clear_token: clearToken }), 'Webhook saved');
  };
  const test = () => run(async () => {
    try {
      await testWebhook();
    } catch (err) {
      throw new ApiError(err instanceof ApiError ? err.status : 0, `Test failed: ${err instanceof ApiError ? err.message : 'network'}`);
    }
    return getWebhook();
  }, 'Test message delivered');
  const remove = () => {
    if (!window.confirm('Remove the webhook? Alerts stop being delivered.')) return;
    void run(async () => { await deleteWebhook(); return { configured: false }; }, 'Webhook removed');
  };

  const hint = presets.find((p) => p.id === preset)?.hint;
  return (
    <form className="panel" onSubmit={save} aria-label="Alert webhook">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <Send size={20} style={{ color: 'var(--accent)' }} />
        <h3 style={{ fontSize: 16 }}>Alert webhook</h3>
      </div>
      {info && !info.configured && <p className="dr-hint">No webhook configured. Alerts show here but are not delivered anywhere.</p>}
      {info?.configured && info.last && (
        <p className={info.last.ok ? 'form-ok' : 'form-error'}>
          Last delivery {timeLabel(info.last.at)}: {info.last.ok ? 'delivered' : `failed (${info.last.error || 'unknown'})`}
        </p>
      )}
      <div className="form-grid">
        <label>Preset
          <select value={preset} onChange={(e) => setPreset(e.target.value)}>
            {presets.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
          </select>
        </label>
        <label>URL<input type="url" value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://" /></label>
        <label>Token (write-only)
          <input type="password" autoComplete="off" value={token} onChange={(e) => setToken(e.target.value)} disabled={clearToken}
            placeholder={info?.has_token ? 'saved; leave empty to keep' : 'none'} />
        </label>
        {info?.has_token && (
          <label style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <input type="checkbox" style={{ width: 'auto' }} checked={clearToken} onChange={(e) => setClearToken(e.target.checked)} />Remove the saved token
          </label>
        )}
      </div>
      {hint && <p className="dr-hint">{hint}</p>}
      {message && <p className={message.kind === 'ok' ? 'form-ok' : 'form-error'} role={message.kind === 'ok' ? 'status' : 'alert'}>{message.text}</p>}
      <div className="dr-actions">
        <button type="submit" disabled={busy}>Save webhook</button>
        {info?.configured && <button type="button" className="btn-secondary" disabled={busy} onClick={() => void test()}><Send size={14} />Send test</button>}
        {info?.configured && <button type="button" className="btn-danger" disabled={busy} onClick={remove}><Trash2 size={14} />Remove webhook</button>}
      </div>
    </form>
  );
};
```

- [ ] **Step 4: Add the form to Settings for admins**

In `web/src/pages/Settings.tsx`: import `{ WebhookForm } from '../components/WebhookForm'`, destructure `({ settings, user, onChanged })`, and add as the first child of the grid `div`:

```tsx
        {user.role === 'admin' && <WebhookForm onChanged={onChanged} />}
```

- [ ] **Step 5: Run the tests and build**

Run: `cd web && npx vitest run && npm run build`
Expected: all PASS, build OK.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/WebhookForm.tsx web/src/components/WebhookForm.test.tsx web/src/pages/Settings.tsx
git commit -m "web: alert webhook form on Settings"
```

---

### Task 7: Browser regressions, dist, docs

**Files:**
- Modify: `web/browser/ui.spec.mjs` (the "Welcome" assertion and the navigation clicks)
- Create: `web/browser/monitor.spec.mjs`
- Rebuild: `web/dist/`
- Modify: `UI-VERIFICATION.md`, `docs/*.png`
- Modify: `AGENTS.md` (root), `web/AGENTS.md`, `internal/monitor/AGENTS.md`, `README.md`

The browser suite needs an app to watch. The egress guard refuses loopback but admits LAN addresses, so the spec starts its own fake health server on `0.0.0.0` and targets it through the host's first non-internal IPv4. Runners without one skip the monitor spec with a message.

- [ ] **Step 1: Update the existing shell spec**

In `web/browser/ui.spec.mjs`:
- replace `await expect(page.getByRole('heading', { name: /Welcome/ })).toBeVisible();` with `await expect(page.getByRole('heading', { name: 'Status' })).toBeVisible();`
- replace both `nav.getByRole('button')` occurrences with `nav.getByRole('link')` (three places: `.first().focus()`, the two `.nth(1)` assertions) and `nav.getByRole('button', { name: 'Settings & DB' })` with `nav.getByRole('link', { name: 'Settings & DB' })` (two places).

Run: `cd web && npm run build && cd .. && go build -o .browser/server ./cmd/server && cd web && npx playwright test ui.spec.mjs --project=light-1280`
Expected: PASS.

- [ ] **Step 2: Write the monitor spec**

```js
// web/browser/monitor.spec.mjs
import { test, expect } from '@playwright/test';
import { createServer } from 'node:http';
import { networkInterfaces } from 'node:os';

// The egress guard refuses loopback, so the fake app listens on every interface and is
// reached through the host's LAN address, which the guard admits.
function lanAddress() {
  for (const list of Object.values(networkInterfaces())) {
    for (const i of list ?? []) if (i.family === 'IPv4' && !i.internal && /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(i.address)) return i.address;
  }
  return null;
}

const state = { health: 503, hook: 500, hooks: 0 };
let server;
let base;

test.beforeAll(async () => {
  const ip = lanAddress();
  test.skip(!ip, 'no private IPv4 interface for the fake app');
  server = createServer((req, res) => {
    if (req.url === '/healthz') {
      res.writeHead(state.health, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ schema: 'ky.health/1', service: 'fake', status: state.health === 200 ? 'ok' : 'down', checks: [{ name: 'database', status: state.health === 200 ? 'ok' : 'down', reason: 'refused' }] }));
      return;
    }
    if (req.url === '/hook') { state.hooks++; res.writeHead(state.hook); res.end(); return; }
    res.writeHead(404); res.end();
  });
  await new Promise((r) => server.listen(0, '0.0.0.0', r));
  base = `http://${ip}:${server.address().port}`;
});
test.afterAll(async () => { if (server) await new Promise((r) => server.close(r)); });

async function signIn(page) {
  await page.goto('/');
  await page.getByPlaceholder('admin', { exact: true }).fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
}

test('status, alerts, detail page and the alert bar in every state', async ({ page }, testInfo) => {
  test.setTimeout(240_000);
  await signIn(page);
  // Empty install.
  await expect(page.getByRole('status')).toContainText('No apps watched yet');
  // Add the fake app (interval at the 10 s floor so the state machine settles quickly).
  await page.getByRole('button', { name: 'Add app' }).click();
  await page.getByLabel('Name').fill('Fake app');
  await page.getByLabel('Health URL').fill(`${base}/healthz`);
  await page.getByLabel('Interval (seconds)').fill('10');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  const tile = page.getByRole('link', { name: /Fake app/ });
  await expect(tile).toBeVisible();
  // Three down polls → down: red bar with the problem line linking to the detail page.
  await expect(page.getByRole('alert')).toContainText('Fake app down', { timeout: 90_000 });
  await page.screenshot({ path: testInfo.outputPath('status-down.png'), fullPage: true });
  await page.getByRole('alert').getByRole('link', { name: /Fake app down/ }).click();
  await expect(page).toHaveURL(/#\/apps\/tgt_/);
  const banner = page.getByRole('region', { name: 'Current state' });
  await expect(banner).toContainText('down');
  await expect(banner).toContainText(`GET ${base}/healthz`);
  await expect(page.getByText('database')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Status' })).toHaveAttribute('aria-current', 'page');
  await page.screenshot({ path: testInfo.outputPath('detail-down.png'), fullPage: true });
  // Silence until fixed: still shown, marked silenced.
  await page.getByRole('button', { name: 'Silence until fixed' }).click();
  await expect(banner).toContainText('silenced until fixed');
  // Alerts tab lists the transition and the silence.
  await page.getByRole('link', { name: 'Alerts' }).first().click();
  await expect(page.getByRole('row').nth(1)).toContainText('pending → down');
  await expect(page.getByText('Silenced:')).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('alerts.png'), fullPage: true });
  // Webhook that fails → "Alerts not being delivered".
  await page.getByRole('link', { name: 'Settings & DB' }).click();
  await page.getByLabel('Preset').selectOption('generic');
  await page.getByLabel('URL').fill(`${base}/hook`);
  await page.getByRole('button', { name: 'Save webhook' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Webhook saved' })).toBeVisible();
  await page.getByRole('button', { name: 'Send test' }).click();
  await expect(page.getByRole('alert').filter({ hasText: /Test failed/ })).toBeVisible({ timeout: 60_000 });
  await expect(page.getByRole('alert').filter({ hasText: 'Alerts not being delivered' })).toBeVisible({ timeout: 30_000 });
  await page.screenshot({ path: testInfo.outputPath('settings-webhook-failing.png'), fullPage: true });
  expect(await page.evaluate(() => document.body.textContent)).not.toContain('token=');
  // Recovery: two ok polls → green bar, silence cleared.
  state.health = 200;
  await page.getByRole('link', { name: 'Status' }).first().click();
  await expect(page.getByRole('status').filter({ hasText: 'All 1 apps healthy' })).toBeVisible({ timeout: 90_000 });
  await page.screenshot({ path: testInfo.outputPath('status-ok.png'), fullPage: true });
  await tile.click();
  await expect(banner).toContainText('ok');
  await expect(banner).not.toContainText('silenced');
  // Delete goes back to Status.
  page.once('dialog', (d) => d.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page).toHaveURL(/#\/status$/);
  await expect(page.getByRole('status')).toContainText('No apps watched yet');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
```

The generic preset with an empty token sends no Authorization header; the fake hook answers 500 so the notifier's retries (about 20 s) end in `receiver_500`.

- [ ] **Step 3: Run the browser suite**

Run: `cd web && npx playwright test`
Expected: PASS for all four projects (both specs). The monitor spec is long (state machine settling: ~30 s to down, ~20 s test send, ~20 s to ok) — if a step times out, check the fake app is reachable from the server: `curl http://<lan-ip>:<port>/healthz` while the test runs, and that `KYPULSE_POLL_WORKERS` is unset. If the workstation has no private IPv4, the spec skips; CI runners have one.

- [ ] **Step 4: Rebuild dist and capture evidence**

```bash
cd web && npm run build && cd ..
cp web/test-results/monitor-status-alerts-detail-page-and-the-alert-bar-in-every-state-light-1280/status-down.png docs/status-down-light-desktop.png
cp web/test-results/monitor-status-alerts-detail-page-and-the-alert-bar-in-every-state-dark-390/status-ok.png docs/status-ok-dark-mobile.png
cp web/test-results/monitor-status-alerts-detail-page-and-the-alert-bar-in-every-state-light-1280/detail-down.png docs/detail-down-light-desktop.png
cp web/test-results/monitor-status-alerts-detail-page-and-the-alert-bar-in-every-state-dark-1280/alerts.png docs/alerts-dark-desktop.png
```

(Playwright names the result directory after the spec file, test title and project; `ls web/test-results` if the names differ.) Delete the four scaffold captures `docs/ky-ui-*.png` and `docs/browser-settings-light-mobile.png`.

- [ ] **Step 5: Rewrite UI-VERIFICATION.md**

```markdown
# UI verification

kyPulse's own screens (step 2c): alert bar, Status, Alerts, app detail, webhook form.

## Capture conditions

Captured by `web/browser/monitor.spec.mjs` against the real Go server with disposable SQLite
data, production CSP and real login, at 1280×900 and 390×900 in Busnes Light and Dark. The
watched app is a fake `ky.health/1` server on the runner's LAN address; the webhook is the
same server answering 500.

## Checks

`npm test` (vitest: router, monitor helpers and client, AlertBar, Status, Alerts, AppDetail,
WebhookForm, Backup, ChangePassword) and `npm run test:browser` (shell spec plus the monitor
spec: empty install, add app, down after three polls, red bar linking to the detail page,
checks, silence until fixed, Alerts table and silences, webhook saved, test send failing,
"Alerts not being delivered", recovery to green with the silence cleared, delete back to
Status, no horizontal overflow). Viewer-role rendering is covered by the component tests;
there is no route that creates a viewer account for the browser suite.

## Screenshots

| | |
| --- | --- |
| ![Status, one app down, light desktop](docs/status-down-light-desktop.png) | ![Status, all healthy, dark mobile](docs/status-ok-dark-mobile.png) |
| ![App detail while down, light desktop](docs/detail-down-light-desktop.png) | ![Alerts, dark desktop](docs/alerts-dark-desktop.png) |

## Reproduce

Build the frontend, run `go build -o .browser/server ./cmd/server` at the repo root, then
`cd web && npx playwright install chromium && npm run test:browser`. The monitor spec skips on
a machine with no private IPv4 interface.
```

- [ ] **Step 6: DOX and README pass**

- Root `AGENTS.md` Local Contracts: replace the sentence starting "Today a viewer sees Overview" with: "A viewer sees Status, Alerts, app detail pages and Settings & DB (read-only, no webhook form); Backup and every write are admin-only."
- `web/AGENTS.md`: under Local Contracts add "Navigation is hash-routed (`#/status`, `#/alerts`, `#/apps/<id>`, `#/backup`, `#/settings`) by `src/router.ts`; there is no other navigation state. `App` polls `/api/status` every 15 s for the `AlertBar` shown above every page; pages fetch their own data through `src/monitor.ts` and call `onChanged` after a write. Admin-only controls render only for `role === 'admin'`; the server enforces it." Under Verification add the monitor spec and its LAN-address requirement. Update the sidebar sentence: navigation items are links.
- `web/browser/AGENTS.md` Local Contracts: add "`monitor.spec.mjs` runs its own fake health/webhook server on `0.0.0.0` and targets it by the host's private IPv4, because the egress guard refuses loopback; it skips when no such interface exists."
- `internal/monitor/AGENTS.md`: change "Messages link to `AppURL/#/apps/<id>`; step 2c makes that route exist." to "Messages link to `AppURL/#/apps/<id>`, the app detail page."
- `README.md`: add a short "Screens" section after the intro: Status (grid, broken first), Alerts, app detail with silence 1h/8h/until fixed, alert bar on every page, webhook on Settings (admin), roles.

- [ ] **Step 7: Full local CI**

Run: `make ci`
Expected: passes, including the `web/dist` diff check if `make ci` includes it (otherwise `git status` shows `web/dist` changes staged and nothing unbuilt).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "web: browser regressions for status, alerts and detail; dist; UI verification and docs"
```
