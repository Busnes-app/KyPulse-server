import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { Logs } from './Logs';
import { Activity } from './Activity';
import { decodeLogs } from '../logs';
const stamp = '2026-09-27T12:00:00Z';
const line = { id: 9, time: stamp, received_at: stamp, source_id: 's', source: 'sender', target_id: '', app: 'vault', level: 'error', event: 'event', message: '<img src=x onerror=alert(1)>', raw: 'raw line', truncated: true };
const response = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
function stub(read?: (url: string) => Promise<Response>) {
 const f = vi.fn(async (input: RequestInfo | URL) => {
  const url = String(input);
  if (url === '/api/log-sources') return response({ sources: [] });
  if (url === '/api/targets') return response({ targets: [] });
  return read ? read(url) : response({ items: [line], next_before_id: 0 });
 });vi.stubGlobal('fetch', f);return f;
}
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
it('checks JSON before accepting it', () => {
 expect(() => decodeLogs({ items: [{ ...line, id: '9' }], next_before_id: 0 })).toThrow();
 expect(() => decodeLogs({ items: [line], next_before_id: -1 })).toThrow();
 expect(decodeLogs({ items: [line], next_before_id: 0 }).items[0].message).toBe(line.message);
});
it('renders plain text and applies/clears literal and UTC filters', async () => {
 const f = stub();const { container } = render(<Logs user={{ role: 'admin' }} />);
 await screen.findByText(line.message);expect(container.querySelector('img')).toBeNull();expect(screen.getByText('truncated')).toBeTruthy();fireEvent.click(screen.getByText('Raw line'));expect(screen.getByText('raw line')).toBeTruthy();
 fireEvent.change(screen.getByLabelText('Log app'), { target: { value: 'vault' } });fireEvent.change(screen.getByLabelText('Log level'), { target: { value: 'error' } });fireEvent.change(screen.getByLabelText('Literal text'), { target: { value: '%_' } });fireEvent.change(screen.getByLabelText('From (UTC)'), { target: { value: '2026-09-27T10:00' } });fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
 await waitFor(() => expect(f.mock.calls.some(([url]) => String(url).includes('text=%25_') && String(url).includes('from=2026-09-27T10%3A00%3A00.000Z'))).toBe(true));
 fireEvent.click(screen.getByRole('button', { name: 'Clear' }));await waitFor(() => expect(String(f.mock.calls.at(-1)?.[0])).not.toContain('text='));
});
it('paginates, resets cursor, and distinguishes failure from empty results', async () => {
 let fail = false;const f = stub(async url => fail ? response({ error: 'Unavailable' }, 503) : response({ items: [{ ...line, id: url.includes('before_id') ? 8 : 9, message: url.includes('before_id') ? 'older' : 'newer' }], next_before_id: url.includes('before_id') ? 0 : 9 }));
 render(<Logs user={{ role: 'admin' }} />);await screen.findByText('newer');fireEvent.click(screen.getByRole('button', { name: 'Load more' }));await screen.findByText('older');
 fail = true;fireEvent.click(screen.getByRole('button', { name: 'Apply' }));await screen.findByText('Unavailable');expect(screen.queryByText('No logs found.')).toBeNull();
 fail = false;fireEvent.click(screen.getByRole('button', { name: 'Retry' }));await screen.findByText('newer');expect(screen.queryByText('older')).toBeNull();expect(String(f.mock.calls.at(-1)?.[0])).not.toContain('before_id');
});
it('ignores stale requests and clears data on role/session change', async () => {
 let finish: ((r: Response) => void) | undefined;stub(async url => url.includes('app=fresh') ? response({ items: [{ ...line, message: 'fresh' }], next_before_id: 0 }) : new Promise<Response>(resolve => { finish = resolve; }));
 const { rerender } = render(<Logs user={{ role: 'admin', id: 'one' }} />);fireEvent.change(screen.getByLabelText('Log app'), { target: { value: 'fresh' } });fireEvent.click(screen.getByRole('button', { name: 'Apply' }));await screen.findByText('fresh');finish?.(response({ items: [line], next_before_id: 0 }));await waitFor(() => expect(screen.queryByText(line.message)).toBeNull());
 rerender(<Logs user={{ role: 'viewer', id: 'one' }} />);expect(screen.queryByText('fresh')).toBeNull();
});
it('uses backend bursts and activity filters', async () => {
 const f = stub(async () => response({ items: [], next_before_id: 0, has_app_events: true, bursts: [{ app: 'vault', actor: 'alice', ip: '', count: 5, from: stamp, to: stamp }] }));render(<Activity user={{ role: 'admin' }} />);await screen.findByText(/5 failed sign-ins/);expect(screen.getByText(/screen-only triage hint/i)).toBeTruthy();
 fireEvent.change(screen.getByLabelText('Activity app'), { target: { value: 'vault' } });fireEvent.change(screen.getByLabelText('Actor'), { target: { value: 'alice' } });fireEvent.change(screen.getByLabelText('Outcome'), { target: { value: 'failure' } });fireEvent.click(screen.getByRole('button', { name: 'Apply' }));await waitFor(() => expect(String(f.mock.calls.at(-1)?.[0])).toContain('actor=alice&outcome=failure'));
});
it('never requests timelines for viewers', () => {const f = stub();render(<><Logs user={{ role: 'viewer' }} /><Activity user={{ role: 'viewer' }} /></>);expect(f).not.toHaveBeenCalled();});

it('clears an administrator timeline when the signed-in identity changes', async () => {
 const f = stub();const { rerender } = render(<Logs user={{ role: 'admin', id: 'first' }} />);
 await screen.findByText(line.message);
 f.mockImplementation(async () => new Promise<Response>(() => {}));
 rerender(<Logs user={{ role: 'admin', id: 'second' }} />);
 expect(screen.queryByText(line.message)).toBeNull();
});
it('bounds rendered rows to 1000 even with more pages', async () => {
 stub(async url => {
  const before = Number(new URL(url, 'http://localhost').searchParams.get('before_id')) || 1200;
  return response({ items: Array.from({ length: 100 }, (_, i) => ({ ...line, id: before - i - 1, message: `row-${before - i - 1}` })), next_before_id: before - 100 });
 });
 const { container } = render(<Logs user={{ role: 'admin' }} />);
 for (let page = 1; page <= 10; page++) {
  await waitFor(() => expect(container.querySelectorAll('.log-lines:first-of-type > li')).toHaveLength(page * 100));
  if (page < 10) fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
 }
 expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull();expect(screen.getByText(/Showing the newest 1,000 matches/)).toBeTruthy();
});
it('clears retained rows and signals shell sign-out on authorization failure', async () => {
 let denied = false;const listener = vi.fn();window.addEventListener('logs-auth-denied', listener);
 stub(async () => denied ? response({ error: 'Administrator role required' }, 403) : response({ items: [line], next_before_id: 9 }));
 try {
  render(<Logs user={{ role: 'admin' }} />);await screen.findByText(line.message);denied = true;
  fireEvent.click(screen.getByRole('button', { name: 'Load more' }));await screen.findByText('Administrator role required');expect(screen.queryByText(line.message)).toBeNull();expect(listener).toHaveBeenCalledOnce();
 } finally {window.removeEventListener('logs-auth-denied', listener);}
});
it('distinguishes an unlogged app from an empty filtered activity page', async () => {
 let retained = true;stub(async () => response({ items: [], next_before_id: 0, bursts: [], has_app_events: retained }));
 render(<Activity user={{ role: 'admin' }} />);fireEvent.change(screen.getByLabelText('Activity app'), { target: { value: 'vault' } });fireEvent.click(screen.getByRole('button', { name: 'Apply' }));await screen.findByText(/No audit events match/);expect(screen.queryByText(/no audit events: not on shared logging/)).toBeNull();
 retained = false;fireEvent.click(screen.getByRole('button', { name: 'Apply' }));expect(await screen.findByText(/no audit events: not on shared logging/)).toBeTruthy();
});
