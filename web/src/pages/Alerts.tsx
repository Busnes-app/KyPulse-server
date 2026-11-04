import React, { useEffect, useRef, useState } from 'react';
import { BellOff } from 'lucide-react';
import { hrefFor } from '../router';
import { ApiError, causeLabel, isSilenced, listAlerts, listTargets, stateWord, timeLabel, type Target, type TargetEvent } from '../monitor';
import type { PageUser } from './Status';
import { PageHead } from '../components/PageHead';

interface AlertsProps { user: PageUser }

// merge joins two newest-first event lists, one row per id, newest first.
function merge(a: TargetEvent[], b: TargetEvent[]): TargetEvent[] {
  const byId = new Map<number, TargetEvent>();
  for (const e of [...b, ...a]) byId.set(e.id, e);
  return [...byId.values()].sort((x, y) => y.id - x.id);
}

function delivery(e: TargetEvent): string {
  if (e.notified) return 'sent';
  if (e.notify_error) return `failed: ${e.notify_error}`;
  return 'not sent';
}

// Alerts lists recent state changes and reminders with their delivery result, and which apps
// are silenced right now.
export const Alerts: React.FC<AlertsProps> = () => {
  // Newest first. Once older pages are loaded, the 15-second refresh merges the newest page in
  // instead of replacing the list, so nothing between it and the loaded pages drops out.
  const [events, setEvents] = useState<TargetEvent[] | null>(null);
  const [paged, setPaged] = useState(false);
  // Read when a refresh resolves, not when it started: a refresh in flight during "Load older
  // alerts" must merge, not replace the page just loaded.
  const pagedRef = useRef(false);
  const [cursor, setCursor] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [targets, setTargets] = useState<Record<string, Target>>({});
  const [error, setError] = useState('');

  useEffect(() => {
    const load = async () => {
      try {
        const [a, t] = await Promise.all([listAlerts(100), listTargets()]);
        const head = a.events ?? [];
        if (pagedRef.current) {
          setEvents((prev) => merge(head, prev ?? []));
        } else {
          setEvents(head);
          setCursor(a.next_before_id);
        }
        setTargets(Object.fromEntries(t.map((x) => [x.id, x])));
        setError('');
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Could not load alerts');
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [paged]);

  const loadMore = async () => {
    setLoadingMore(true);
    try {
      const page = await listAlerts(100, cursor);
      setEvents((prev) => merge(prev ?? [], page.events ?? []));
      setCursor(page.next_before_id);
      pagedRef.current = true;
      setPaged(true);
      setError('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not load older alerts');
    } finally {
      setLoadingMore(false);
    }
  };

  const silenced = Object.values(targets).filter((t) => isSilenced(t));
  const shown = events;

  return (
    <div className="dr-page">
      <PageHead title="Alerts" lede="State changes and reminders for every watched app, newest first." />
      {error && <p className="form-error" role="alert">{error}</p>}
      {silenced.length > 0 && (
        <p className="dr-hint"><BellOff size={14} /><strong>Silenced:</strong>{' '}
          {silenced.map((t) => (
            <span key={t.id}><a href={hrefFor(`/apps/${t.id}`)}>{t.name}</a>{t.until_fixed ? ' until fixed' : ` until ${timeLabel(t.silenced_until)}`} </span>
          ))}
        </p>
      )}
      {shown && shown.length === 0 && <p className="dr-hint">No alerts yet. State changes and reminders will show here.</p>}
      {shown && shown.length > 0 && (
        <div className="panel" style={{ overflowX: 'auto', padding: 0 }}>
          <table className="events events-with-app" role="table" aria-label="Recent alerts">
            <thead role="rowgroup"><tr role="row"><th scope="col">Time</th><th scope="col">App</th><th scope="col">Change</th><th scope="col">Reason</th><th scope="col">Delivery</th></tr></thead>
            <tbody role="rowgroup">
              {shown.map((e) => {
                const t = targets[e.target_id];
                return (
                  <tr key={e.id} role="row">
                    <td role="cell" data-label="Time">{timeLabel(e.at)}</td>
                    <td role="cell" data-label="App">{t ? <a href={hrefFor(`/apps/${t.id}`)}>{t.name}</a> : <span className="dr-mono">{e.target_id}</span>}</td>
                    <td role="cell" data-label="Change" className={`state-${e.to}`}>{e.reminder ? `Still ${stateWord(e.to)} (reminder)` : `${stateWord(e.from)} → ${stateWord(e.to)}`}</td>
                    <td role="cell" data-label="Reason" className="dr-mono">{causeLabel(e.cause) || '—'}</td>
                    <td role="cell" data-label="Delivery">{delivery(e)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {shown && cursor > 0 && (
        <button type="button" className="btn-secondary" style={{ marginTop: 12 }} disabled={loadingMore} onClick={() => void loadMore()}>
          {loadingMore ? 'Loading…' : 'Load older alerts'}
        </button>
      )}
    </div>
  );
};
