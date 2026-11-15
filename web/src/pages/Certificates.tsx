import React, { useCallback, useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { PageHead } from '../components/PageHead';
import { hrefFor } from '../router';
import {
  ApiError, causeLabel, certStatus, certWord, daysLeftLabel, hostPath, listCertificates, setCertThresholds, type Target,
} from '../monitor';
import type { PageUser } from './Status';

interface CertificatesProps { user: PageUser }

// Certificates lists every watched certificate, soonest expiry first (the server's order),
// and lets an administrator change the two thresholds.
export const Certificates: React.FC<CertificatesProps> = ({ user }) => {
  const [rows, setRows] = useState<Target[] | null>(null);
  const [thresholds, setThresholds] = useState({ warn: 21, critical: 7 });
  // The form's own values: a refresh never overwrites what an administrator is typing.
  const [warn, setWarn] = useState<number | null>(null);
  const [critical, setCritical] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [saved, setSaved] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const b = await listCertificates();
      setRows(b.certificates ?? []);
      setThresholds({ warn: b.warn_days, critical: b.critical_days });
      setError('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not load certificates');
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [load]);

  const save = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setSaved('');
    setError('');
    try {
      const saved = await setCertThresholds(warn ?? thresholds.warn, critical ?? thresholds.critical);
      setThresholds({ warn: saved.warn_days, critical: saved.critical_days });
      setWarn(null);
      setCritical(null);
      setSaved('Saved. The next poll of each app uses the new thresholds.');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save the thresholds');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="dr-page">
      <PageHead title="Certificates" lede={`TLS certificates of every https app and TLS target, soonest expiry first. Warning at ${thresholds.warn} days left, critical at ${thresholds.critical}.`} />
      {error && <p className="form-error" role="alert">{error}</p>}
      {rows && rows.length === 0 && <p className="dr-hint">No certificates yet. An https app or a TLS target shows its certificate here after its first check.</p>}
      {rows && rows.length > 0 && (
        <div className="panel" style={{ overflowX: 'auto', padding: 0 }}>
          <table className="events events-with-app" role="table" aria-label="Certificates">
            <thead role="rowgroup"><tr role="row"><th scope="col">App</th><th scope="col">State</th><th scope="col">Expires</th><th scope="col">Issuer</th></tr></thead>
            <tbody role="rowgroup">
              {rows.map((t) => (
                <tr key={t.id} role="row">
                  <td role="cell" data-label="App"><a href={hrefFor(`/apps/${t.id}`)}>{t.name}</a><br /><span className="dr-mono dr-hint">{hostPath(t.url)}</span></td>
                  <td role="cell" data-label="State">
                    <span className="ky-status" data-ky-status={certStatus(t.cert_state)}>{certWord(t.cert_state)}</span>
                    {t.cert_state !== 'ok' && t.cert_cause && <> {causeLabel(t.cert_cause)}</>}
                  </td>
                  <td role="cell" data-label="Expires">{daysLeftLabel(t.cert_days_left)}<br /><span className="dr-hint">{t.cert ? new Date(t.cert.not_after).toLocaleDateString() : '—'}</span></td>
                  <td role="cell" data-label="Issuer">{t.cert?.issuer || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {user.role === 'admin' && (
        <section className="dr-section">
          <h2>Thresholds</h2>
          <form onSubmit={save} aria-label="Certificate thresholds">
            <div className="form-grid">
              <label>Warning at (days left)<input type="number" min={2} max={365} value={warn ?? thresholds.warn} onChange={(e) => setWarn(Number(e.target.value))} required /></label>
              <label>Critical at (days left)<input type="number" min={1} max={364} value={critical ?? thresholds.critical} onChange={(e) => setCritical(Number(e.target.value))} required /></label>
            </div>
            {saved && <p className="dr-hint" role="status">{saved}</p>}
            <div className="dr-actions">
              <button type="submit" disabled={busy}>{busy ? <Loader2 size={14} className="animate-spin" /> : null}{busy ? 'Saving…' : 'Save thresholds'}</button>
            </div>
          </form>
          <p className="dr-hint">Expired, wrong-name and untrusted certificates always fail. Reminders go out hourly; an app's silence covers its certificate.</p>
        </section>
      )}
    </div>
  );
};
