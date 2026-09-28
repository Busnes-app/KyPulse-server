import { render, screen, cleanup, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { KyYardCollection, KyYardCollectors } from './KyYardCollectors';

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

test('each collector retains its own age and failure', () => {
  render(<KyYardCollectors status={{ paired: true, stale: false,
    inventory: { stale: false, last_success: new Date().toISOString() },
    logs: { stale: true, last_success: new Date(Date.now() - 600_000).toISOString(), error: 'unauthorized' },
    audit: { stale: true, error: 'audit_cursor_unsupported' },
  }} />);
  expect(screen.getByText(/Inventory: last success.*fresh/)).toBeTruthy();
  expect(screen.getByText(/Container logs: last success.*stale.*unauthorized/)).toBeTruthy();
  expect(screen.getByText(/Audit feed: no successful collection yet.*audit_cursor_unsupported/)).toBeTruthy();
  expect(screen.getByText(/History may have gaps/)).toBeTruthy();
});

test('subscription aborts on unmount and surfaces status request failure', async () => {
  let signal: AbortSignal | null | undefined;
  vi.stubGlobal('fetch', vi.fn((_url: string, init: RequestInit) => { signal = init.signal; return Promise.reject(new Error('offline')); }));
  const view = render(<KyYardCollection />);
  await waitFor(() => expect(screen.getByText('KyYard collection status unavailable.')).toBeTruthy());
  view.unmount();
  expect(signal?.aborted).toBe(true);
});
