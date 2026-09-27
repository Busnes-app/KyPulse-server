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
    const links = screen.getAllByRole('link', { name: 'KyVault' });
    expect(links.length).toBe(3); // silenced line + two rows
    for (const l of links) expect(l.getAttribute('href')).toBe('#/apps/tgt_a');
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
