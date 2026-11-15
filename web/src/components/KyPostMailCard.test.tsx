import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { KyPostMailCard } from './KyPostMailCard';

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it('says why the channel is hidden without KyIdentity sign-in', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ available: false, reason: 'no_kyidentity' }))));
  render(<KyPostMailCard onChanged={vi.fn()} />);
  expect(await screen.findByText(/Available when KyPulse signs in through KyIdentity/)).toBeTruthy();
  expect(screen.queryByLabelText('Mailbox owner')).toBeNull();
});

it('saves owner and recipients, warns about the same host and shows a refusal code', async () => {
  const info = { available: true, paired: true, owners: [{ id: 'u1', name: 'Ops' }], configured: true, owner_id: 'u1', recipients: ['ops@example.com'],
    last: { at: '2026-11-15T10:00:00Z', ok: false, error: 'recipient_unconfirmed' } };
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PUT') return new Response(JSON.stringify({ ...info, recipients: ['ops@example.com', 'oncall@example.com'] }));
    return new Response(JSON.stringify(info));
  });
  vi.stubGlobal('fetch', fetchMock);
  render(<KyPostMailCard onChanged={vi.fn()} />);
  expect(await screen.findByText(/refused \(recipient_unconfirmed\)/)).toBeTruthy();
  expect(screen.getByText(/Same host:/)).toBeTruthy();
  fireEvent.change(screen.getByLabelText(/Recipients/), { target: { value: 'ops@example.com\noncall@example.com' } });
  expect(screen.getByRole('button', { name: /Send test/ })).toHaveProperty('disabled', true);
  fireEvent.submit(screen.getByRole('form', { name: 'Email through KyPost' }));
  await screen.findByText('KyPost channel saved');
  const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT');
  expect(JSON.parse(String(put?.[1]?.body))).toEqual({ owner_id: 'u1', recipients: ['ops@example.com', 'oncall@example.com'] });
});

it('names an unpaired reason', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ available: true, paired: false, reason: 'no_peer', owners: [], configured: false }))));
  render(<KyPostMailCard onChanged={vi.fn()} />);
  expect(await screen.findByText(/KyIdentity has no KyPost paired with KyPulse/)).toBeTruthy();
});
