import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { Certificates } from './Certificates';
import type { Target } from '../monitor';

const base = { interval_sec: 30, enabled: true, state: 'ok' as const, state_since: 'x', until_fixed: false, last_result: '', last_latency_ms: 0, created_at: 'x', updated_at: 'x', basic: false };
const certificates: Target[] = [
  { ...base, id: 'a', name: 'Edge', url: 'tls://edge.lan:443', cert: { not_after: '2026-11-18T00:00:00Z', issuer: 'Test CA', name_match: true }, cert_state: 'critical', cert_cause: 'cert_expiring', cert_days_left: 2 },
  { ...base, id: 'b', name: 'KyPost', url: 'https://post.lan/healthz', cert: { not_after: '2027-02-01T00:00:00Z', issuer: 'Other CA', name_match: true }, cert_state: 'ok', cert_days_left: 78 },
];

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

it('lists certificates in the server order with level and days left', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ certificates, warn_days: 21, critical_days: 7 }))));
  render(<Certificates user={{ role: 'viewer' }} />);
  const rows = await screen.findAllByRole('row');
  expect(rows).toHaveLength(3);
  expect(rows[1].textContent).toContain('Edge');
  expect(rows[1].textContent).toContain('Critical');
  expect(rows[1].textContent).toContain('certificate expiring');
  expect(rows[1].textContent).toContain('2 days left');
  expect(rows[2].textContent).toContain('78 days left');
  expect(screen.queryByRole('form', { name: 'Certificate thresholds' })).toBeNull();
});

it('lets an administrator save the thresholds and shows a refusal', async () => {
  const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PUT') return new Response(JSON.stringify({ error: 'thresholds must satisfy 1 <= critical_days < warn_days <= 365' }), { status: 400 });
    return new Response(JSON.stringify({ certificates: [], warn_days: 21, critical_days: 7 }));
  });
  vi.stubGlobal('fetch', fetchMock);
  render(<Certificates user={{ role: 'admin' }} />);
  await screen.findByText(/No certificates yet/);
  fireEvent.change(screen.getByLabelText('Critical at (days left)'), { target: { value: '30' } });
  fireEvent.submit(screen.getByRole('form', { name: 'Certificate thresholds' }));
  expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringContaining('critical_days < warn_days'));
  const put = fetchMock.mock.calls.find((c) => c[1]?.method === 'PUT');
  expect(JSON.parse(String(put?.[1]?.body))).toEqual({ warn_days: 21, critical_days: 30 });
});
