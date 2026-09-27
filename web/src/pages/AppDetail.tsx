import React, { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, BellOff, Pencil, Trash2 } from 'lucide-react';
import { TargetForm } from '../components/TargetForm';
import { hrefFor, navigate } from '../router';
import {
  ApiError, deleteTarget, getTarget, isSilenced, silenceTarget, sinceLabel, stateClass, stateLabel, timeLabel, updateTarget,
  type LastResult, type SilenceFor, type Target, type TargetEvent, type TargetInput,
} from '../monitor';
import type { PageUser } from './Status';

interface AppDetailProps {
  id: string;
  user: PageUser;
  onChanged: () => void;
}

// AppDetail is one watched app: state banner, the exact request that fails, the checks the
// app reported, alert history and, for admins, silence, edit and delete.
export const AppDetail: React.FC<AppDetailProps> = ({ id, user, onChanged }) => {
  const [target, setTarget] = useState<Target | null>(null);
  const [last, setLast] = useState<LastResult | null>(null);
  const [events, setEvents] = useState<TargetEvent[]>([]);
  const [missing, setMissing] = useState(false);
  const [error, setError] = useState('');
  const [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false);
  const isAdmin = user.role === 'admin';

  const load = useCallback(async () => {
    try {
      const d = await getTarget(id);
      setTarget(d.target);
      setLast(d.last_result);
      setEvents(d.events ?? []);
      setError('');
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) setMissing(true);
      else setError(err instanceof ApiError ? err.message : 'Could not load the app');
    }
  }, [id]);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [load]);

  const act = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setError('');
    try {
      await fn();
      await load();
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Request failed');
    } finally {
      setBusy(false);
    }
  };

  const silence = (f: SilenceFor) => act(() => silenceTarget(id, f));
  const save = async (input: TargetInput) => {
    await updateTarget(id, input);
    setEditing(false);
    await load();
    onChanged();
  };
  const remove = async () => {
    if (!target || !window.confirm(`Stop watching ${target.name}? Its alert history is deleted too.`)) return;
    setBusy(true);
    try {
      await deleteTarget(id);
      onChanged();
      navigate('/status');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not delete the app');
      setBusy(false);
    }
  };

  if (missing) {
    return (
      <div className="dr-page">
        <p className="dr-hint">App not found. It may have been removed. <a href={hrefFor('/status')}>Back to Status</a></p>
      </div>
    );
  }
  if (!target) {
    return <div className="dr-page">{error ? <p className="form-error" role="alert">{error}</p> : <p className="dr-hint">Loading…</p>}</div>;
  }

  const cls = stateClass(target);
  const failing = target.enabled && !!target.last_polled_at && target.state !== 'ok';
  return (
    <div className="dr-page">
      <p className="dr-hint"><a href={hrefFor('/status')}><ArrowLeft size={14} style={{ verticalAlign: 'middle' }} /> Status</a></p>
      <div className="dr-header">
        <h1>{target.name}</h1>
        {isAdmin && !editing && (
          <div className="dr-actions">
            <button type="button" className="btn-secondary" onClick={() => setEditing(true)} disabled={busy}><Pencil size={14} />Edit</button>
            <button type="button" className="btn-danger" onClick={remove} disabled={busy}><Trash2 size={14} />Delete</button>
          </div>
        )}
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      {editing && <div className="dr-section"><TargetForm initial={target} submitLabel="Save changes" onSubmit={save} onCancel={() => setEditing(false)} /></div>}

      <section className={`banner banner-${cls}`} aria-label="Current state">
        <span className={`dot dot-${cls}`} aria-hidden="true" />
        <h2 className={`state-${cls}`}>{stateLabel(target)}</h2>
        <span>
          {!target.last_polled_at
            ? 'awaiting first check'
            : target.state === 'pending'
              ? 'not yet classified'
              : `for ${sinceLabel(target.state_since)} (since ${timeLabel(target.state_since)})`}
        </span>
        {failing && (
          <span className="dr-mono" style={{ flexBasis: '100%', fontSize: 13 }}>
            GET {target.url} → {target.cause || last?.cause || 'unknown'}
            {target.last_polled_at ? ` · last tried ${timeLabel(target.last_polled_at)} (${target.last_latency_ms} ms)` : ''}
          </span>
        )}
        {isSilenced(target) && (
          <span className="badge dr-badge-muted"><BellOff size={12} />silenced {target.until_fixed ? 'until fixed' : `until ${timeLabel(target.silenced_until)}`}</span>
        )}
      </section>

      {isAdmin && (
        <div className="dr-section">
          <h3>Silence webhook messages</h3>
          <p className="dr-hint">The alert bar and this page keep showing the problem; only webhook messages stop.</p>
          <div className="dr-actions">
            <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('1h')}>Silence 1h</button>
            <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('8h')}>Silence 8h</button>
            <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('until_fixed')}>Silence until fixed</button>
            {isSilenced(target) && <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('off')}>Unsilence</button>}
          </div>
        </div>
      )}

      <div className="dr-two">
        <div className="dr-section">
          <h3>Checks</h3>
          {last?.checks && last.checks.length > 0 ? (
            <div className="dr-checks">
              {last.checks.map((c) => (
                <div key={c.name} className="dr-check">
                  <span className={`dot dot-${c.status}`} aria-hidden="true" />
                  <span className="dr-mono">{c.name}</span>
                  <span className={`state-${c.status}`}>{c.status}{c.reason ? ` · ${c.reason}` : ''}</span>
                </div>
              ))}
            </div>
          ) : (
            <p className="dr-hint">{last?.basic ? 'This app answers with a plain OK and reports no checks.' : 'No checks reported yet.'}</p>
          )}
        </div>
        <div className="dr-section">
          <h3>Settings</h3>
          <div className="dr-facts">
            <div className="dr-fact"><span className="dr-fact-label">Health URL</span><span className="dr-fact-value dr-mono">{target.url}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Interval</span><span className="dr-fact-value">{target.interval_sec} s</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Polling</span><span className="dr-fact-value">{target.enabled ? 'enabled' : 'paused'}</span></div>
          </div>
        </div>
      </div>

      <div className="dr-section">
        <h3>Alert history</h3>
        {events.length === 0 ? <p className="dr-hint">No state changes yet.</p> : (
          <div className="panel" style={{ overflowX: 'auto', padding: 0 }}>
            <table className="events">
              <thead><tr><th>Time</th><th>Change</th><th>Reason</th><th>Delivery</th></tr></thead>
              <tbody>
                {events.map((e) => (
                  <tr key={e.id}>
                    <td>{timeLabel(e.at)}</td>
                    <td className={`state-${e.to}`}>{e.reminder ? `still ${e.to} (reminder)` : `${e.from} → ${e.to}`}</td>
                    <td className="dr-mono">{e.cause || ''}</td>
                    <td>{e.notified ? 'sent' : e.notify_error ? `failed: ${e.notify_error}` : 'not sent'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
};
