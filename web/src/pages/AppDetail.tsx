import { RecentLogs } from '../components/LogTimeline';
import React, { useCallback, useEffect, useState } from 'react';
import { ArrowLeft, BellOff, Pencil, Trash2 } from 'lucide-react';
import { TargetDialog } from '../components/TargetDialog';
import { KyYardFacts } from '../components/KyYardFacts';
import { PageHead } from '../components/PageHead';
import { hrefFor, navigate } from '../router';
import {
  ApiError, deleteTarget, getKyYard, getTarget, isSilenced, kyYardContainers, kyYardPending, silenceTarget, sinceLabel, stateClass, stateLabel, stateWord, timeLabel, updateTarget, uptimeLabel, causeLabel,
  type ContainerFacts, type KyYardStatus, type LastResult, type SilenceFor, type Suggestion, type Target, type TargetEvent, type TargetInput,
} from '../monitor';
import type { PageUser } from './Status';

interface AppDetailProps {
  id: string;
  user: PageUser;
  onChanged: () => void;
}

// AppDetail is one watched app: its state and failing check under the name, three stats, the
// incident (the exact request, times and silence actions), checks, history and, for admins,
// edit and delete.
export const AppDetail: React.FC<AppDetailProps> = ({ id, user, onChanged }) => {
  const [target, setTarget] = useState<Target | null>(null);
  const [last, setLast] = useState<LastResult | null>(null);
  const [events, setEvents] = useState<TargetEvent[]>([]);
  const [facts, setFacts] = useState<ContainerFacts | null>(null);
  const [yard, setYard] = useState<KyYardStatus | null>(null);
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
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
      setFacts(d.kyyard ?? null);
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

  useEffect(() => {
    getKyYard().then(setYard).catch(() => setYard(null));
  }, []);

  useEffect(() => {
    if (!isAdmin) return;
    kyYardContainers().then(setSuggestions).catch(() => setSuggestions([]));
  }, [isAdmin]);

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
  const silenced = isSilenced(target);
  // The reason leads: the worst check the app reported, else the poll's own cause.
  // Unhealthy checks first; the reason that leads is the worst one, else the poll's own cause.
  const order: Record<string, number> = { down: 0, degraded: 1, ok: 2 };
  const checks = [...(last?.checks ?? [])].sort((a, b) => (order[a.status] ?? 2) - (order[b.status] ?? 2));
  const worst = checks.find((c) => c.status !== 'ok');
  const reason = worst ? `${worst.name}: ${worst.reason || stateWord(worst.status)}` : causeLabel(target.cause || last?.cause);
  const forLabel = { ok: 'Up for', degraded: 'Degraded for', down: 'Down for', pending: 'Pending for', paused: 'Paused for' }[cls] ?? 'For';
  const yardSection = <KyYardFacts facts={facts} link={target.container} paired={!!yard?.paired} pending={!!yard && kyYardPending(yard)} isAdmin={isAdmin} />;
  return (
    <div className="dr-page app-detail-page">
      <p className="dr-hint"><a href={hrefFor('/status')}><ArrowLeft size={14} aria-hidden="true" />Status</a></p>
      <PageHead title={target.name}
        action={isAdmin && <button type="button" className="btn-secondary" onClick={() => setEditing(true)} disabled={busy}><Pencil size={14} aria-hidden="true" />Edit</button>} />
      {error && <p className="form-error" role="alert">{error}</p>}
      {editing && <TargetDialog title={`Edit ${target.name}`} initial={target} submitLabel="Save changes" suggestions={suggestions} onSubmit={save} onClose={() => setEditing(false)} />}

      <section className={`detail-state detail-${cls}`} aria-label="Current state">
        <p className="detail-headline">
          <span className={`dot dot-${cls}`} aria-hidden="true" />
          <strong className={`state-${cls}`}>{stateLabel(target)}</strong>
          {!target.last_polled_at ? <span>awaiting first check</span> : target.state === 'pending' && <span>not yet classified</span>}
          {failing && reason && <span className="detail-reason">{reason}</span>}
        </p>
        <dl className="detail-stats">
          <div><dt>{forLabel}</dt><dd>{target.last_polled_at ? sinceLabel(target.state_since) : '—'}</dd></div>
          <div><dt>Last checked</dt><dd>{target.last_polled_at ? `${sinceLabel(target.last_polled_at)} ago` : 'Not yet'}</dd>
            {target.last_polled_at && <dd className="detail-stat-note">{target.last_latency_ms} ms</dd>}</div>
          <div><dt>Incidents in 24h</dt><dd>{target.incidents_24h ?? 0}</dd>
            <dd className="detail-stat-note">{target.uptime_24h == null ? 'no uptime yet' : `${uptimeLabel(target.uptime_24h)} uptime (24 h)`}</dd></div>
        </dl>
        {(failing || silenced || isAdmin) && (
          <div className="detail-incident">
            {failing && (
              <div className="detail-incident-facts">
                <p className="dr-mono">GET {target.url} → {causeLabel(target.cause || last?.cause) || stateWord(last?.state ?? 'unknown')}</p>
                <p>Since {timeLabel(target.state_since)}</p>
              </div>
            )}
            {silenced && (
              <p><span className="badge dr-badge-muted"><BellOff size={12} aria-hidden="true" />Silenced {target.until_fixed ? 'until fixed' : `until ${timeLabel(target.silenced_until)}`}</span></p>
            )}
            {isAdmin && (
              <div className="detail-silence">
                <div className="dr-actions silence-actions">
                  <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('1h')}>Silence 1h</button>
                  <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('8h')}>Silence 8h</button>
                  <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('until_fixed')}>Silence until fixed</button>
                  {silenced && <button type="button" className="btn-secondary" disabled={busy} onClick={() => silence('off')}>Unsilence</button>}
                </div>
                <p className="dr-hint">Silencing stops webhook messages only; the alert bar and this page keep showing the problem.</p>
              </div>
            )}
          </div>
        )}
      </section>

      <div className="dr-two app-detail-diagnostics">
        {checks.length > 0 && (
          <div className="dr-section">
            <h2>Checks</h2>
            <div className="dr-checks">
              {checks.map((c) => (
                <div key={c.name} className="dr-check">
                  <span className={`dot dot-${c.status}`} aria-hidden="true" />
                  <span className="dr-mono">{c.name}</span>
                  <span className={`state-${c.status}`}>{stateWord(c.status)}{c.reason ? ` · ${c.reason}` : ''}</span>
                </div>
              ))}
            </div>
          </div>
        )}
        <div className="dr-section">
          <h2>Settings</h2>
          <div className="dr-facts">
            <div className="dr-fact"><span className="dr-fact-label">Health URL</span><span className="dr-fact-value dr-mono">{target.url}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Interval</span><span className="dr-fact-value">{target.interval_sec} s</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Polling</span><span className="dr-fact-value">{target.enabled ? 'enabled' : 'paused'}</span></div>
          </div>
        </div>
      </div>

      <div className="dr-section">
        <h2>Alert history</h2>
        {events.length === 0 ? <p className="dr-hint">No state changes yet.</p> : (
          <div className="panel" style={{ overflowX: 'auto', padding: 0 }}>
            <table className="events" role="table" aria-label="Alert history">
              <thead role="rowgroup"><tr role="row"><th scope="col">Time</th><th scope="col">Change</th><th scope="col">Reason</th><th scope="col">Delivery</th></tr></thead>
              <tbody role="rowgroup">
                {events.map((e) => (
                  <tr key={e.id} role="row">
                    <td role="cell" data-label="Time">{timeLabel(e.at)}</td>
                    <td role="cell" data-label="Change" className={`state-${e.to}`}>{e.reminder ? `Still ${stateWord(e.to)} (reminder)` : `${stateWord(e.from)} → ${stateWord(e.to)}`}</td>
                    <td role="cell" data-label="Reason" className="dr-mono">{causeLabel(e.cause) || '—'}</td>
                    <td role="cell" data-label="Delivery">{e.notified ? 'sent' : e.notify_error ? `failed: ${e.notify_error}` : 'not sent'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* An unlinked app with no logs gets one muted line instead of two empty sections. */}
      {isAdmin ? <RecentLogs key={id} id={id} kyYard={target.container ? yardSection : null} />
        : <div className="dr-section">{target.container && <h2>KyYard</h2>}{yardSection}</div>}

      {isAdmin && (
        <div className="dr-section dr-remove">
          <h2>Stop watching</h2>
          <p className="dr-hint">Its alert history is deleted too.</p>
          <button type="button" className="btn-danger" onClick={remove} disabled={busy}><Trash2 size={14} aria-hidden="true" />Delete</button>
        </div>
      )}
    </div>
  );
};
