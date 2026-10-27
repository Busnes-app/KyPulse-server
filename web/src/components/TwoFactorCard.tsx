import { useState } from 'react';
import type { FormEvent } from 'react';
import { KeyRound } from 'lucide-react';
import { secureFetch } from '../api';

interface TwoFactorCardProps {
  enabled: boolean;
  // Reports a turned-on or turned-off factor so the signed-in user the shell holds stays current.
  onEnabledChange?: (enabled: boolean) => void;
}

interface Pending {
  secret: string;
  otpauth_uri: string;
}

async function post(path: string, body?: unknown): Promise<any> {
  const response = await secureFetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data: any = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(typeof data?.error === 'string' ? data.error : 'Request failed. Please try again.');
  return data;
}

// TwoFactorCard lets a signed-in local account turn TOTP on and off. There is no QR library in
// this bundle, so the otpauth URI and the secret are shown as text to paste or type in.
export function TwoFactorCard({ enabled: initial, onEnabledChange }: TwoFactorCardProps) {
  const [enabled, setEnabled] = useState(initial);
  const [pending, setPending] = useState<Pending | null>(null);
  const [recovery, setRecovery] = useState<string[] | null>(null);
  const [code, setCode] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function run(action: () => Promise<void>) {
    setError('');
    setBusy(true);
    try { await action(); } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Request failed. Please try again.');
    } finally { setBusy(false); }
  }

  const start = () => run(async () => {
    setRecovery(null);
    setPending(await post('/api/auth/mfa/enroll'));
    setCode('');
  });

  const confirm = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    void run(async () => {
      const data = await post('/api/auth/mfa/enroll/confirm', { password, code: code.trim() });
      setPending(null);
      setPassword('');
      setCode('');
      setEnabled(true);
      onEnabledChange?.(true);
      setRecovery(Array.isArray(data.recovery_codes) ? data.recovery_codes : []);
    });
  };

  const disable = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    void run(async () => {
      await post('/api/auth/mfa/disable', { password, code: code.trim() });
      setPassword('');
      setCode('');
      setRecovery(null);
      setEnabled(false);
      onEnabledChange?.(false);
    });
  };

  return (
    <div className="panel">
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
        <KeyRound size={20} style={{ color: 'var(--ink)' }} />
        <h3 style={{ fontSize: '16px' }}>Two-factor authentication</h3>
        <span className={enabled ? 'badge badge-accent' : 'badge dr-badge-muted'}>{enabled ? 'On' : 'Off'}</span>
      </div>

      {recovery && (
        <div role="status" style={{ marginBottom: '16px' }}>
          <p style={{ fontSize: '13px' }}>
            Two-factor authentication is on and your other sessions were signed out. Save these recovery codes now; each works once and they are not shown again.
          </p>
          <ul aria-label="Recovery codes" className="font-mono" style={{ fontSize: '13px', columns: 2, margin: '8px 0' }}>
            {recovery.map((c) => <li key={c}>{c}</li>)}
          </ul>
          <button type="button" className="btn-secondary" onClick={() => setRecovery(null)}>I have saved them</button>
        </div>
      )}

      {!enabled && !pending && (
        <>
          <p style={{ color: 'var(--ink)', fontSize: '13px', marginBottom: '16px' }}>
            Ask for a code from an authenticator app after your password when you sign in.
          </p>
          <button type="button" className="btn-secondary" disabled={busy} onClick={() => void start()}>Set up two-factor authentication</button>
        </>
      )}

      {!enabled && pending && (
        <form onSubmit={confirm}>
          <p style={{ fontSize: '13px' }}>Add this account to your authenticator app with the setup link or the key, then enter the code it shows and your password.</p>
          <label htmlFor="totp-uri">Setup link</label>
          <input id="totp-uri" className="font-mono" readOnly value={pending.otpauth_uri} onFocus={(e) => e.currentTarget.select()} />
          <label htmlFor="totp-secret">Key</label>
          <input id="totp-secret" className="font-mono" readOnly value={pending.secret} onFocus={(e) => e.currentTarget.select()} />
          <label htmlFor="totp-confirm-code">Code from the app</label>
          <input id="totp-confirm-code" inputMode="numeric" autoComplete="one-time-code" required maxLength={10}
            value={code} onChange={(e) => setCode(e.target.value)} />
          <label htmlFor="totp-confirm-password">Password</label>
          <input id="totp-confirm-password" type="password" autoComplete="current-password" required maxLength={1024}
            value={password} onChange={(e) => setPassword(e.target.value)} />
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginTop: 16 }}>
            <button type="submit" disabled={busy}>Turn on</button>
            <button type="button" className="btn-secondary" disabled={busy} onClick={() => { setPending(null); setCode(''); setPassword(''); }}>Cancel</button>
          </div>
        </form>
      )}

      {enabled && !recovery && (
        <form onSubmit={disable}>
          <p style={{ color: 'var(--ink)', fontSize: '13px' }}>To turn it off, enter your password and a current code or an unused recovery code.</p>
          <label htmlFor="totp-disable-password">Password</label>
          <input id="totp-disable-password" type="password" autoComplete="current-password" required maxLength={1024}
            value={password} onChange={(e) => setPassword(e.target.value)} />
          <label htmlFor="totp-disable-code">Code or recovery code</label>
          <input id="totp-disable-code" autoComplete="one-time-code" required maxLength={64}
            value={code} onChange={(e) => setCode(e.target.value)} />
          <div style={{ marginTop: 16 }}>
            <button type="submit" className="btn-secondary" disabled={busy}>Turn off</button>
          </div>
        </form>
      )}

      {error && <p role="alert" style={{ marginTop: 12 }}>{error}</p>}
    </div>
  );
}
