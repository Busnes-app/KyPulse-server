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

  it('distinguishes an unpolled pending target from one already polled', async () => {
    const list = [
      target({ name: 'Unpolled', state: 'pending', last_polled_at: null }),
      target({ name: 'Polled', state: 'pending', last_polled_at: '2026-09-27T10:00:00Z' }),
    ];
    stub([[/GET \/api\/targets$/, { targets: list }]]);
    render(<Status user={{ role: 'viewer' }} onChanged={() => {}} />);
    const tiles = await screen.findAllByRole('link', { name: /Ky|Unpolled|Polled/ });
    const unpolled = tiles.find((t) => t.textContent?.includes('Unpolled'))!;
    const polled = tiles.find((t) => t.textContent?.includes('Polled') && !t.textContent.includes('Unpolled'))!;
    expect(unpolled.textContent).toContain('awaiting first check');
    expect(polled.textContent).toContain('not yet classified');
  });
});
