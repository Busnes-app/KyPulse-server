import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, act } from '@testing-library/react';
import { LogSources } from './LogSources';
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); });
it('binds before pairing, shows command and confirms revocation', async () => {
 let revoked = false;vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
 const url = String(input);if (url === '/api/targets') return new Response(JSON.stringify({ targets: [{ id: 't1', name: 'Vault' }] }));
 if (url.endsWith('/pairing')) {expect(JSON.parse(String(init?.body))).toEqual({ target_id: 't1' });return new Response(JSON.stringify({ code: '123456', expires_at: new Date(Date.now() + 60000).toISOString() }));}
 if (init?.method === 'DELETE') {revoked = true;return new Response(JSON.stringify({ revoked: true }));}
 return new Response(JSON.stringify({ sources: [{ id: 's1', name: 'sender', target_id: 't1', created_at: new Date().toISOString(), revoked_at: revoked ? new Date().toISOString() : null }] }));
 }));vi.stubGlobal('confirm', vi.fn(() => false));render(<LogSources />);await screen.findByText('sender');fireEvent.change(screen.getByLabelText('Bind log source to watched app'), { target: { value: 't1' } });fireEvent.click(screen.getByRole('button', { name: 'Add source' }));await screen.findByText(/--code 123456 --name/);
 fireEvent.click(screen.getByRole('button', { name: 'Revoke sender' }));expect(revoked).toBe(false);vi.stubGlobal('confirm', vi.fn(() => true));fireEvent.click(screen.getByRole('button', { name: 'Revoke sender' }));await screen.findByText(/Revoked/);
});
it('clears the code at expiry', async () => {
 vi.useFakeTimers();vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(String(input).endsWith('/pairing') ? { code: '123456', expires_at: new Date(Date.now() + 1000).toISOString() } : String(input) === '/api/targets' ? { targets: [] } : { sources: [] }))));
 render(<LogSources />);await act(async () => {});fireEvent.click(screen.getByRole('button', { name: 'Add source' }));await act(async () => {});expect(screen.getByText(/--code 123456/)).toBeTruthy();await act(async () => {vi.advanceTimersByTime(1001);});expect(screen.queryByText(/--code 123456/)).toBeNull();
});
it('retries source-list errors', async () => {
 let failed = true;vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => new Response(JSON.stringify(String(input) === '/api/targets' ? { targets: [] } : failed ? { error: 'Sources unavailable' } : { sources: [] }), { status: String(input) !== '/api/targets' && failed ? 500 : 200 })));render(<LogSources />);await screen.findByText('Sources unavailable');failed = false;fireEvent.click(screen.getByRole('button', { name: 'Refresh sources' }));await waitFor(() => expect(screen.queryByText('Sources unavailable')).toBeNull());
});
it('ignores a pairing response after unmount and never persists its code', async () => {
 let finish: ((response: Response) => void) | undefined;
 vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => String(input).endsWith('/pairing') ? new Promise<Response>(resolve => { finish = resolve; }) : new Response(JSON.stringify(String(input) === '/api/targets' ? { targets: [] } : { sources: [] }))));
 const storage = vi.spyOn(Storage.prototype, 'setItem');const { unmount } = render(<LogSources />);
 await screen.findByText('No paired sources.');fireEvent.click(screen.getByRole('button', { name: 'Add source' }));unmount();
 await act(async () => {finish?.(new Response(JSON.stringify({ code: '123456', expires_at: new Date(Date.now() + 60000).toISOString() })));});
 expect(screen.queryByText(/123456/)).toBeNull();expect(storage).not.toHaveBeenCalled();storage.mockRestore();
});
