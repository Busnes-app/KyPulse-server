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

  it('treats a malformed 2xx body as an error, not an empty object', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>', { status: 200 })));
    await expect(getStatus()).rejects.toMatchObject({ status: 200, message: 'Unexpected response (not JSON)' });
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
