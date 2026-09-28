import { KyYardCollectors } from './KyYardCollectors';
import React, { useEffect, useState } from 'react';
import { Database, Link2, Unlink } from 'lucide-react';
import { ApiError, getKyYard, kyYardPending, pairKyYard, sinceLabel, timeLabel, unpairKyYard, type KyYardStatus } from '../monitor';

const POLL_MS = 5_000;

// KyYardCard pairs this kyPulse to one KyYard organization with a code a KyYard
// administrator generated (Members → Service tokens → Pair kyPulse). Unpairing here deletes
// the URL and token here only; revoking the token is KyYard's step.
export const KyYardCard: React.FC<{ onChanged: () => void }> = ({ onChanged }) => {
  const [status, setStatus] = useState<KyYardStatus | null>(null);
  const [url, setUrl] = useState('');
  const [code, setCode] = useState('');
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => getKyYard().then(setStatus).catch(() => setStatus(null));
  useEffect(() => { void load(); }, []);

  // Keep collector freshness visible after the first inventory pull too.
  useEffect(() => {
    if (!status?.paired) return;
    const id = window.setInterval(() => { getKyYard().then(setStatus).catch(() => {}); }, POLL_MS);
    return () => window.clearInterval(id);
  }, [status?.paired]);

  const pair = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMessage(null);
    try {
      const s = await pairKyYard({ url: url.trim(), pairing_code: code.trim() });
      setStatus(s);
      setCode('');
      setMessage({ kind: 'ok', text: `Paired to ${s.organization ?? 'KyYard'}` });
      onChanged();
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not reach the server' });
    } finally {
      setBusy(false);
    }
  };
  const unpair = async () => {
    if (!window.confirm('Unpair from KyYard? This deletes the URL and token here. A KyYard administrator must also revoke the token on the Members page; unpairing here does not.')) return;
    setBusy(true);
    try {
      await unpairKyYard();
      await load();
      setMessage({ kind: 'ok', text: 'Unpaired. Ask a KyYard administrator to revoke the token as well.' });
      onChanged();
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not reach the server' });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="panel">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <Database size={20} style={{ color: 'var(--accent)' }} />
        <h3 style={{ fontSize: 16 }}>KyYard</h3>
      </div>
      {status?.paired ? (
        <>
          <div className="dr-facts">
            <div className="dr-fact"><span className="dr-fact-label">Organization</span><span className="dr-fact-value">{status.organization}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">URL</span><span className="dr-fact-value dr-mono">{status.url}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Last pull</span><span className="dr-fact-value">{status.fetched_at ? `${timeLabel(status.fetched_at)} (${sinceLabel(status.fetched_at)} ago)` : 'never'}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">State</span><span className={kyYardPending(status) ? 'dr-fact-value' : status.stale ? 'dr-fact-value dr-danger' : 'dr-fact-value dr-ok'}>{kyYardPending(status) ? 'first pull pending' : status.stale ? `stale${status.error ? ` (${status.error})` : ''}` : 'fresh'}</span></div>
          </div>
          <KyYardCollectors status={status} />
          {status.error === 'unauthorized' && <p className="form-error">KyYard refused the token: it was revoked. Unpair and pair again with a new code.</p>}
          <p className="dr-hint">Revoking the token happens in KyYard (Members → Service tokens); unpairing here only forgets it.</p>
          <div className="dr-actions"><button type="button" className="btn-danger" disabled={busy} onClick={unpair}><Unlink size={14} />Unpair</button></div>
        </>
      ) : (
        <form onSubmit={pair} aria-label="Pair KyYard">
          <p className="dr-hint">A KyYard administrator generates a six-digit code on the organization's Members page (Service tokens → Pair kyPulse). It is valid for 15 minutes.</p>
          <div className="form-grid">
            <label>KyYard URL<input type="url" value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://kyyard.lan" /></label>
            <label>Pairing code<input inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} required autoComplete="one-time-code" /></label>
          </div>
          <div className="dr-actions"><button type="submit" disabled={busy}><Link2 size={14} />Pair</button></div>
        </form>
      )}
      {message && <p className={message.kind === 'ok' ? 'form-ok' : 'form-error'} role={message.kind === 'ok' ? 'status' : 'alert'}>{message.text}</p>}
    </div>
  );
};
