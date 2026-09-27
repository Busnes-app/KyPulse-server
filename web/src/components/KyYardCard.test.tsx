import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { KyYardCard } from './KyYardCard';

const unpaired = { paired: false, stale: false };
const revoked = { paired: true, stale: true, error: 'unauthorized', url: 'https://kyyard.lan', organization: 'Acme', fetched_at: '2026-09-27T09:00:00Z' };

function stub(extra: Array<[RegExp, unknown, number?]> = [], status: unknown = unpaired) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${String(input)}`;
    for (const [re, body, code] of extra) if (re.test(key)) return new Response(code === 204 ? null : JSON.stringify(body), { status: code ?? 200 });
    if (key === 'GET /api/kyyard') return new Response(JSON.stringify(status));
    throw new Error(key);
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

describe('KyYardCard', () => {
  it('renders the pairing form and posts url + pairing_code, never the token', async () => {
    const fetchMock = stub([[/^POST \/api\/kyyard\/pair$/, { paired: true, stale: true, organization: 'Acme' }]]);
    const onChanged = vi.fn();
    render(<KyYardCard onChanged={onChanged} />);
    await screen.findByLabelText('Pair KyYard');
    fireEvent.change(screen.getByLabelText('KyYard URL'), { target: { value: 'https://kyyard.lan' } });
    fireEvent.change(screen.getByLabelText('Pairing code'), { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: /Pair/ }));
    await waitFor(() => expect(onChanged).toHaveBeenCalled());
    const post = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'POST')!;
    const body = JSON.parse((post[1] as RequestInit).body as string);
    expect(body).toEqual({ url: 'https://kyyard.lan', pairing_code: '123456' });
    expect(JSON.stringify(fetchMock.mock.calls)).not.toContain('api_token');
    expect(await screen.findByText('Paired to Acme')).toBeTruthy();
  });

  it('shows the revoked hint and unpairs on confirmation', async () => {
    stub([[/^DELETE \/api\/kyyard$/, null, 204]], revoked);
    vi.stubGlobal('confirm', vi.fn(() => true));
    render(<KyYardCard onChanged={() => {}} />);
    expect(await screen.findByText(/KyYard refused the token: it was revoked/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: /Unpair/ }));
    expect(await screen.findByText(/Unpaired/)).toBeTruthy();
  });

  it('flips from "first pull pending" to "fresh" once the first pull lands, polling every 5s', async () => {
    vi.useFakeTimers();
    let calls = 0;
    const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${String(input)}`;
      if (key === 'GET /api/kyyard') {
        calls++;
        if (calls === 1) return new Response(JSON.stringify(unpaired));
        return new Response(JSON.stringify({ paired: true, stale: false, organization: 'Acme', url: 'https://kyyard.lan', fetched_at: '2026-09-27T10:00:02Z' }));
      }
      if (key === 'POST /api/kyyard/pair') return new Response(JSON.stringify({ paired: true, stale: true, organization: 'Acme' }));
      throw new Error(key);
    });
    vi.stubGlobal('fetch', fn);
    try {
      render(<KyYardCard onChanged={() => {}} />);
      await act(async () => { await vi.advanceTimersByTimeAsync(0); });
      fireEvent.change(screen.getByLabelText('KyYard URL'), { target: { value: 'https://kyyard.lan' } });
      fireEvent.change(screen.getByLabelText('Pairing code'), { target: { value: '123456' } });
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /Pair/ }));
        await vi.advanceTimersByTimeAsync(0);
      });
      expect(screen.getByText('first pull pending')).toBeTruthy();
      await act(async () => { await vi.advanceTimersByTimeAsync(4999); });
      expect(screen.getByText('first pull pending')).toBeTruthy();
      await act(async () => { await vi.advanceTimersByTimeAsync(1); });
      expect(screen.getByText('fresh')).toBeTruthy();
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps polling past 30s while pending, and stops on unpair', async () => {
    vi.useFakeTimers();
    let paired = true;
    const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${String(input)}`;
      if (key === 'GET /api/kyyard') return new Response(JSON.stringify(paired ? { paired: true, stale: true, fetched_at: null } : unpaired));
      if (key === 'DELETE /api/kyyard') { paired = false; return new Response(null, { status: 204 }); }
      throw new Error(key);
    });
    vi.stubGlobal('fetch', fn);
    vi.stubGlobal('confirm', vi.fn(() => true));
    const gets = () => fn.mock.calls.filter(([u, init]) => String(u) === '/api/kyyard' && !(init as RequestInit | undefined)?.method).length;
    try {
      render(<KyYardCard onChanged={() => {}} />);
      await act(async () => { await vi.advanceTimersByTimeAsync(0); });
      await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
      expect(gets()).toBe(1 + 12);
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /Unpair/ }));
        await vi.advanceTimersByTimeAsync(0);
      });
      expect(screen.getByText(/Unpaired/)).toBeTruthy();
      const after = gets();
      await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
      expect(gets()).toBe(after);
    } finally {
      vi.useRealTimers();
    }
  });

  it('stops polling once unmounted', async () => {
    vi.useFakeTimers();
    const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${String(input)}`;
      if (key === 'GET /api/kyyard') return new Response(JSON.stringify({ paired: true, stale: true, fetched_at: null }));
      throw new Error(key);
    });
    vi.stubGlobal('fetch', fn);
    try {
      const { unmount } = render(<KyYardCard onChanged={() => {}} />);
      await act(async () => { await vi.advanceTimersByTimeAsync(0); });
      const before = fn.mock.calls.length;
      unmount();
      await vi.advanceTimersByTimeAsync(10_000);
      expect(fn.mock.calls.length).toBe(before);
    } finally {
      vi.useRealTimers();
    }
  });
});
