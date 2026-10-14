import { afterEach, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createHash } from 'node:crypto';
import { Login } from './Login';

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

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
  render(<Login onSuccess={vi.fn()} appName="kyPulse" />);
  await screen.findByText('Security check verified.');
  // Typing must not throw away the solved challenge.
  fireEvent.change(screen.getByPlaceholderText('admin'), { target: { value: 'alice' } });
  fireEvent.change(screen.getByPlaceholderText('••••••••••••'), { target: { value: 'WrongPassword123!' } });
  expect(round).toBe(1);
  fireEvent.click(screen.getByRole('button', { name: 'Sign In' }));
  expect(await screen.findByText('Invalid credentials or too many attempts')).toBeTruthy();
  await waitFor(() => expect(round).toBe(2));
  await screen.findByText('Security check verified.');
  fireEvent.click(screen.getByRole('button', { name: 'Sign In' }));
  await waitFor(() => expect(tokens).toHaveLength(2));
  expect(atob(tokens[0])).toContain('"salt":"salt1"');
  expect(atob(tokens[1])).toContain('"salt":"salt2"');
});
