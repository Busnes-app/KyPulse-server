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
    for (const [re, body, status] of extra) if (re.test(key)) return new Response(status === 204 ? null : JSON.stringify(body), { status: status ?? 200 });
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
