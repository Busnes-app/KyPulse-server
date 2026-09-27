// AppDetail must remount when the route's id changes (App.tsx renders it with key={id}); this
// pins the state-reset that `key` provides, since a bare rerender with a new `id` prop alone
// would otherwise keep the previous target's state (missing=true) around.
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';
import { AppDetail } from './pages/AppDetail';
import type { Target } from './monitor';

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
    for (const [re, body, status] of extra) if (re.test(key)) return new Response(status === 204 ? null : JSON.stringify(body), { status: status ?? 200 });
    if (/^GET \/api\/targets\/tgt_a$/.test(key)) return new Response(JSON.stringify(detail));
    if (/^GET \/api\/targets\/tgt_gone$/.test(key)) return new Response(JSON.stringify({ error: 'Target not found' }), { status: 404 });
    throw new Error(key);
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('AppDetail remount on id change', () => {
  it('resets state instead of keeping the previous target\'s "not found"', async () => {
    stub();
    const { rerender } = render(<AppDetail key="a" id="tgt_gone" user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByText(/App not found/)).toBeTruthy();

    rerender(<AppDetail key="b" id="tgt_a" user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByRole('heading', { name: 'KyVault' })).toBeTruthy();
  });
});
