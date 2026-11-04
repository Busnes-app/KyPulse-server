import React, { useEffect, useState } from 'react';
import { BellOff } from 'lucide-react';
import { hrefFor } from '../router';
import { ApiError, causeLabel, isSilenced, listAlerts, listTargets, stateWord, timeLabel, type Target, type TargetEvent } from '../monitor';
import type { PageUser } from './Status';
import { PageHead } from '../components/PageHead';

interface AlertsProps { user: PageUser }

function delivery(e: TargetEvent): string {
  if (e.notified) return 'sent';
  if (e.notify_error) return `failed: ${e.notify_error}`;
  return 'not sent';
}

// Alerts lists recent state changes and reminders with their delivery result, and which apps
// are silenced right now.
export const Alerts: React.FC<AlertsProps> = () => {
  const [events, setEvents] = useState<TargetEvent[] | null>(null);
  // Older pages the user asked for, kept across the 15-second refresh of the newest page.
  const [older, setOlder] = useState<TargetEvent[]>([]);
  const [cursor, setCursor] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [targets, setTargets] = useState<Record<string, Target>>({});
  const [error, setError] = useState('');
  const olderLoaded = older.length > 0;

  useEffect(() => {
    const load = async () => {
      try {
        const [a, t] = await Promise.all([listAlerts(100), listTargets()]);
        setEvents(a.events ?? []);
        if (!olderLoaded) setCursor(a.next_before_id);
        setTargets(Object.fromEntries(t.map((x) => [x.id, x])));
        setError('');
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Could not load alerts');
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [olderLoaded]);

  const loadMore = async () => {
    setLoadingMore(true);
    try {
      const page = await listAlerts(100, cursor);
      setOlder((o) => [...o, ...(page.events ?? [])]);
      setCursor(page.next_before_id);
      setError('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not load older alerts');
    } finally {
      setLoadingMore(false);
    }
  };

  const silenced = Object.values(targets).filter((t) => isSilenced(t));
  const shown = events && [...events, ...older.filter((o) => !events.some((e) => e.id === o.id))];

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
