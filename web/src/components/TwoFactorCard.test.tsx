import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { TwoFactorCard } from './TwoFactorCard';

const uri = 'otpauth://totp/kyPulse:erin?secret=JBSWY3DPEHPK3PXP&issuer=kyPulse';

function stub(routes: Record<string, [unknown, number?]>) {
  const fn = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${String(input)}`;
    const hit = routes[key];
    if (!hit) throw new Error(key);
    return new Response(JSON.stringify(hit[0]), { status: hit[1] ?? 200 });
  });
  vi.stubGlobal('fetch', fn);
  return fn;
}

const sent = (fn: ReturnType<typeof stub>, path: string) => {
  const call = fn.mock.calls.find(([input]) => String(input) === path)!;
  return { body: JSON.parse(((call[1] as RequestInit).body as string) ?? '{}'), headers: new Headers((call[1] as RequestInit).headers) };
};

afterEach(() => { cleanup(); vi.unstubAllGlobals(); document.cookie = 'ky_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT'; });

describe('TwoFactorCard', () => {
  it('enrols: shows the setup link and key, confirms a code with CSRF, then shows recovery codes once', async () => {
    document.cookie = 'ky_csrf=tok123';
    const fn = stub({
      'POST /api/auth/mfa/enroll': [{ secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: uri }],
      'POST /api/auth/mfa/enroll/confirm': [{ totp_enabled: true, recovery_codes: ['aaaa-bbbb', 'cccc-dddd'] }],
    });
    render(<TwoFactorCard enabled={false} />);
    fireEvent.click(screen.getByRole('button', { name: 'Set up two-factor authentication' }));
    expect(((await screen.findByLabelText('Setup link')) as HTMLInputElement).value).toBe(uri);
    expect((screen.getByLabelText('Key') as HTMLInputElement).value).toBe('JBSWY3DPEHPK3PXP');
    expect(sent(fn, '/api/auth/mfa/enroll').headers.get('X-CSRF-Token')).toBe('tok123');

    fireEvent.change(screen.getByLabelText('Code from the app'), { target: { value: ' 123456 ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
    const list = await screen.findByRole('list', { name: 'Recovery codes' });
    expect(list.textContent).toContain('aaaa-bbbb');
    const confirm = sent(fn, '/api/auth/mfa/enroll/confirm');
    expect(confirm.body).toEqual({ code: '123456' });
    expect(confirm.headers.get('X-CSRF-Token')).toBe('tok123');
    expect(screen.getByText('On')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'I have saved them' }));
    expect(screen.queryByRole('list', { name: 'Recovery codes' })).toBeNull();
    expect(screen.getByLabelText('Password')).toBeTruthy();
  });

  it('keeps enrolment pending and shows the server error on a wrong code', async () => {
    stub({
      'POST /api/auth/mfa/enroll': [{ secret: 'S', otpauth_uri: uri }],
      'POST /api/auth/mfa/enroll/confirm': [{ error: 'Invalid TOTP verification code' }, 401],
    });
    render(<TwoFactorCard enabled={false} />);
    fireEvent.click(screen.getByRole('button', { name: 'Set up two-factor authentication' }));
    fireEvent.change(await screen.findByLabelText('Code from the app'), { target: { value: '000000' } });
    fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
    expect((await screen.findByRole('alert')).textContent).toBe('Invalid TOTP verification code');
    expect(screen.getByText('Off')).toBeTruthy();
    expect(screen.getByLabelText('Setup link')).toBeTruthy();
  });

  it('disables with the password and a code', async () => {
    const fn = stub({ 'POST /api/auth/mfa/disable': [{ totp_enabled: false }] });
    const changed = vi.fn();
    render(<TwoFactorCard enabled onEnabledChange={changed} />);
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'SuperSecretPass123!' } });
    fireEvent.change(screen.getByLabelText('Code or recovery code'), { target: { value: '654321' } });
    fireEvent.click(screen.getByRole('button', { name: 'Turn off' }));
    await waitFor(() => expect(screen.getByText('Off')).toBeTruthy());
    expect(sent(fn, '/api/auth/mfa/disable').body).toEqual({ password: 'SuperSecretPass123!', code: '654321' });
    expect(changed).toHaveBeenCalledWith(false);
  });
});
