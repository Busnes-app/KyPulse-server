import { useState } from 'react';
import type { FormEvent } from 'react';
import { LockKeyhole } from 'lucide-react';
import { secureFetch } from '../api';

interface PasswordCardProps {
  // Called once the password changed; every session, this one included, has ended.
  onChanged: () => void;
}

// PasswordCard lets a signed-in local account replace its own password after proving the
// current one. The server signs every session out, so the shell returns to sign-in.
export function PasswordCard({ onChanged }: PasswordCardProps) {
  const [open, setOpen] = useState(false);
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmation, setConfirmation] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const close = () => { setOpen(false); setCurrentPassword(''); setNewPassword(''); setConfirmation(''); setError(''); };

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError('');
    if (newPassword !== confirmation) { setError('The new passwords do not match.'); return; }
    if (currentPassword === newPassword) { setError('Choose a different password.'); return; }
    setBusy(true);
    try {
      const response = await secureFetch('/api/auth/change-password', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
      });
      if (!response.ok) {
        const body: any = await response.json().catch(() => ({}));
        throw new Error(typeof body?.error === 'string' ? body.error : 'Password change failed. Please try again.');
      }
      close();
      onChanged();
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'Password change failed. Please try again.');
    } finally { setBusy(false); }
  }

  return (
    <div className="panel">
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
        <LockKeyhole size={20} style={{ color: 'var(--ink)' }} aria-hidden="true" />
        <h3 style={{ fontSize: '16px' }}>Password</h3>
      </div>
      {!open && (
        <>
          <p style={{ color: 'var(--ink)', fontSize: '13px', marginBottom: '16px' }}>
            Changing it signs you out everywhere, this browser included.
          </p>
          <button type="button" className="btn-secondary" onClick={() => setOpen(true)}>Change password</button>
        </>
      )}
      {open && (
        <form onSubmit={submit}>
          <p style={{ fontSize: '13px' }}>Use at least 12 characters. You will sign in again afterward.</p>
          <label htmlFor="account-current-password">Current password</label>
          <input id="account-current-password" type="password" autoComplete="current-password" required maxLength={1024}
            value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} />
          <label htmlFor="account-new-password">New password</label>
          <input id="account-new-password" type="password" autoComplete="new-password" required minLength={12} maxLength={1024}
            value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
          <label htmlFor="account-confirm-password">Confirm new password</label>
          <input id="account-confirm-password" type="password" autoComplete="new-password" required minLength={12} maxLength={1024}
            value={confirmation} onChange={(e) => setConfirmation(e.target.value)} />
          {error && <p role="alert" className="form-error">{error}</p>}
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, marginTop: 16 }}>
            <button type="submit" disabled={busy}>{busy ? 'Changing password…' : 'Change password'}</button>
            <button type="button" className="btn-secondary" disabled={busy} onClick={close}>Cancel</button>
          </div>
        </form>
      )}
    </div>
  );
}
