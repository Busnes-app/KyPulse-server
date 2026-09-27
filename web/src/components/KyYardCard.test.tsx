import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
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
});
