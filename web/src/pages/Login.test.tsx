import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createHash } from 'node:crypto';
import { Login } from './Login';

afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.useRealTimers(); });

it('solves a fresh CAPTCHA after a rejected login, since each solution is single-use', async () => {
  let round = 0;
  const tokens: string[] = [];
  const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input) === '/api/auth/pow-challenge') {
      const salt = `salt${++round}`;
      const challenge = createHash('sha256').update(`${salt}1`).digest('hex');
      return new Response(JSON.stringify({ algorithm: 'SHA-256', salt, challenge, maxnumber: 1, expires_at: 0, signature: 'sig' }));
    }
    if (String(input) === '/api/auth/login') {
      tokens.push(JSON.parse(init?.body as string).captcha_token);
      return new Response(JSON.stringify({ error: 'Invalid credentials or too many attempts' }), { status: 401 });
    }
    throw new Error(String(input));
  });
  vi.stubGlobal('fetch', fetcher);
  render(<Login onSuccess={vi.fn()} appName="KyPulse" />);
  await screen.findByText('Browser check passed');
  // Typing must not throw away the solved challenge.
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'alice' } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'WrongPassword123!' } });
  expect(round).toBe(1);
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  expect(await screen.findByText('Invalid credentials or too many attempts')).toBeTruthy();
  await waitFor(() => expect(round).toBe(2));
  await screen.findByText('Browser check passed');
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  await waitFor(() => expect(tokens).toHaveLength(2));
  expect(atob(tokens[0])).toContain('"salt":"salt1"');
  expect(atob(tokens[1])).toContain('"salt":"salt2"');
});

it('offers KySignOn only when the server reports a configured issuer', () => {
  vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>(() => {})));
  const { rerender } = render(<Login onSuccess={vi.fn()} appName="KyPulse" />);
  expect(screen.queryByRole('link', { name: 'Continue with single sign-on' })).toBeNull();
  expect(screen.queryByRole('separator')).toBeNull();
  // Until the browser check solves, an early submit would only fail and restart it.
  const submit = screen.getByRole('button', { name: 'Checking this browser…' });
  expect(submit.querySelector('svg')).not.toBeNull();
  expect((submit as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText('Health and alerts for your Ky apps.')).toBeTruthy();
  rerender(<Login onSuccess={vi.fn()} appName="KyPulse" ssoEnabled />);
  expect(screen.getByRole('link', { name: 'Continue with single sign-on' }).getAttribute('href')).toBe('/api/sso/kysignon/login');
  expect(screen.getAllByText(/single sign-on/i)).toHaveLength(1);
});

it('signs in with the password alone; there is no local second-factor step', async () => {
  const user = { id: 'u1', username: 'alice', totp_enabled: true };
  const fetcher = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url === '/api/auth/pow-challenge') {
      const salt = 'salt';
      const challenge = createHash('sha256').update(`${salt}1`).digest('hex');
      return new Response(JSON.stringify({ algorithm: 'SHA-256', salt, challenge, maxnumber: 1, expires_at: 0, signature: 'sig' }));
    }
    if (url === '/api/auth/login') return new Response(JSON.stringify({ authenticated: true, user }));
    throw new Error(url);
  });
  vi.stubGlobal('fetch', fetcher);
  const onSuccess = vi.fn();
  render(<Login onSuccess={onSuccess} appName="KyPulse" />);
  await screen.findByText('Browser check passed');
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'alice' } });
  fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'RightPassword123!' } });
  fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
  await waitFor(() => expect(onSuccess).toHaveBeenCalledWith(user));
  expect(fetcher.mock.calls.map(([input]) => String(input)).filter((u) => u.includes('/mfa/'))).toEqual([]);
});

it('still offers sign-in when the browser check fails', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) === '/api/auth/pow-challenge') return new Response('{}', { status: 503 });
    return new Response(JSON.stringify({ authenticated: true, user: { id: 'u1' } }));
  }));
  render(<Login onSuccess={vi.fn()} appName="KyPulse" />);
  const submit = await screen.findByRole('button', { name: 'Sign in' });
  expect((submit as HTMLButtonElement).disabled).toBe(false);
});

it('solves a fresh check before an idle page\'s solution expires', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  let round = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
    if (String(input) === '/api/auth/pow-challenge') {
      const salt = `salt${++round}`;
      const challenge = createHash('sha256').update(`${salt}1`).digest('hex');
      return new Response(JSON.stringify({ algorithm: 'SHA-256', salt, challenge, maxnumber: 1, expires_at: 0, signature: 'sig' }));
    }
    throw new Error(String(input));
  }));
  render(<Login onSuccess={vi.fn()} appName="KyPulse" />);
  await screen.findByText('Browser check passed');
  expect(round).toBe(1);
  vi.advanceTimersByTime(3 * 60_000);
  expect(round).toBe(1);
  vi.advanceTimersByTime(60_000);
  await waitFor(() => expect(round).toBe(2));
  await screen.findByRole('button', { name: 'Sign in' });
});
