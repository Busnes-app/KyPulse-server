import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { AppDetail } from './AppDetail';
import type { Target } from '../monitor';

const target: Target = {
  id: 'tgt_a', name: 'KyVault', url: 'https://vault.lan/healthz', interval_sec: 30, enabled: true, state: 'down',
  state_since: '2026-09-27T09:30:00Z', cause: 'refused', until_fixed: false, last_result: '', last_polled_at: '2026-09-27T10:00:00Z',
  last_latency_ms: 40, created_at: 'x', updated_at: 'x', basic: false, container: 'kyvault',
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
    const fetchMock = stub();
    render(<AppDetail id="tgt_a" user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByRole('heading', { name: 'KyVault' })).toBeTruthy();
    const banner = screen.getByRole('region', { name: 'Current state' });
    expect(banner.className).toContain('banner-down');
    expect(banner.textContent).toContain('refused');
    expect(banner.textContent).toContain('GET https://vault.lan/healthz');
    expect(screen.getByText('kyyard').parentElement?.textContent).toContain('down');
    expect(screen.getAllByRole('row')).toHaveLength(2);
    expect(fetchMock.mock.calls.some(([url]) => String(url).startsWith('/api/logs'))).toBe(false);
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

  it('shows the failing request for a pending target once it has been polled', async () => {
    const pendingDetail = { ...detail, target: { ...target, state: 'pending', cause: undefined } };
    const fn = vi.fn(async (input: RequestInfo | URL) => {
      if (/tgt_a$/.test(String(input))) return new Response(JSON.stringify(pendingDetail));
      throw new Error(String(input));
    });
    vi.stubGlobal('fetch', fn);
    render(<AppDetail id="tgt_a" user={{ role: 'viewer' }} onChanged={() => {}} />);
    const banner = await screen.findByRole('region', { name: 'Current state' });
    expect(banner.textContent).toContain('not yet classified');
    expect(banner.textContent).toContain('GET https://vault.lan/healthz');
  });

  it('renders KyYard container facts', async () => {
    const facts = {
      link: 'kyvault', endpoint_id: 'ep_1', endpoint_name: 'ep_1', container_id: 'c1', name: 'kyvault', image: 'kyvault:latest',
      state: 'exited', status: 'Exited (1) 2 minutes ago', health: 'unhealthy', exit_code: 1, observed_at: '2026-09-27T09:59:00Z',
      endpoint_offline: false, has_sample: true, sample_at: new Date(Date.now() - 3 * 60_000).toISOString(),
      memory_bytes: 104857600, memory_limit: 0, restart_count: 5, restarts_last_hour: 2, history_minutes: 60, stale: false,
    };
    const fn = vi.fn(async (input: RequestInfo | URL) => {
      const key = String(input);
      if (/^\/api\/kyyard$/.test(key)) return new Response(JSON.stringify({ paired: true, stale: false, fetched_at: '2026-09-27T10:00:00Z' }));
      if (/tgt_a$/.test(key)) return new Response(JSON.stringify({ ...detail, kyyard: facts }));
      throw new Error(key);
    });
    vi.stubGlobal('fetch', fn);
    render(<AppDetail id="tgt_a" user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByText(/kyvault on ep_1/)).toBeTruthy();
    expect(screen.getByText('unhealthy')).toBeTruthy();
    expect(screen.getByText(/exit 1/)).toBeTruthy();
    expect(screen.getByText(/in the last hour/)).toBeTruthy();
    expect(screen.getAllByText(/sampled 3m ago/).length).toBeGreaterThan(0);
  });

  it('says the linked container was not seen in KyYard when paired but no facts', async () => {
    const fn = vi.fn(async (input: RequestInfo | URL) => {
      const key = String(input);
      if (/^\/api\/kyyard$/.test(key)) return new Response(JSON.stringify({ paired: true, stale: false, fetched_at: '2026-09-27T10:00:00Z' }));
      if (/tgt_a$/.test(key)) return new Response(JSON.stringify({ ...detail, kyyard: null }));
      throw new Error(key);
    });
    vi.stubGlobal('fetch', fn);
    render(<AppDetail id="tgt_a" user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByText(/Not seen in KyYard/)).toBeTruthy();
  });

  it('says first pull pending, not "not seen", right after pairing', async () => {
    const fn = vi.fn(async (input: RequestInfo | URL) => {
      const key = String(input);
      if (/^\/api\/kyyard$/.test(key)) return new Response(JSON.stringify({ paired: true, stale: true, fetched_at: null }));
      if (/tgt_a$/.test(key)) return new Response(JSON.stringify({ ...detail, kyyard: null }));
      throw new Error(key);
    });
    vi.stubGlobal('fetch', fn);
    render(<AppDetail id="tgt_a" user={{ role: 'viewer' }} onChanged={() => {}} />);
    expect(await screen.findByText(/first pull pending/)).toBeTruthy();
    expect(screen.queryByText(/Not seen in KyYard/)).toBeNull();
  });

  it('shows a hint for an unlinked target', async () => {
    const unlinked = { ...target, container: undefined };
    const fn = vi.fn(async (input: RequestInfo | URL) => {
      const key = String(input);
      if (/^\/api\/kyyard$/.test(key)) return new Response(JSON.stringify({ paired: false, stale: false }));
      if (/tgt_a$/.test(key)) return new Response(JSON.stringify({ ...detail, target: unlinked, kyyard: null }));
      throw new Error(key);
    });
    vi.stubGlobal('fetch', fn);
    render(<AppDetail id="tgt_a" user={{ role: 'admin' }} onChanged={() => {}} />);
    expect(await screen.findByText(/Not linked to a KyYard container/)).toBeTruthy();
  });

  it('keeps the container when editing and saving', async () => {
    const fetchMock = stub([[/^PUT \/api\/targets\/tgt_a$/, { target }]]);
    render(<AppDetail id="tgt_a" user={{ role: 'admin' }} onChanged={() => {}} />);
    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }));
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => (init as RequestInit)?.method === 'PUT')).toBe(true));
    const put = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'PUT')!;
    expect(JSON.parse((put[1] as RequestInit).body as string).container).toBe('kyvault');
  });
});
