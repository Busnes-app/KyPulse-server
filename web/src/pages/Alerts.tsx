import React, { useEffect, useState } from 'react';
import { Bell, BellOff } from 'lucide-react';
import { hrefFor } from '../router';
import { ApiError, isSilenced, listAlerts, listTargets, timeLabel, type Target, type TargetEvent } from '../monitor';
import type { PageUser } from './Status';

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
  const [targets, setTargets] = useState<Record<string, Target>>({});
  const [error, setError] = useState('');

  useEffect(() => {
    const load = async () => {
      try {
        const [a, t] = await Promise.all([listAlerts(100), listTargets()]);
        setEvents(a.events ?? []);
        setTargets(Object.fromEntries(t.map((x) => [x.id, x])));
        setError('');
      } catch (err) {
        setError(err instanceof ApiError ? err.message : 'Could not load alerts');
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, []);

  const silenced = Object.values(targets).filter((t) => isSilenced(t));

  return (
    <div className="dr-page">
      <div className="dr-header">
        <h1 style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Bell size={24} style={{ color: 'var(--accent)' }} />Alerts</h1>
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      {silenced.length > 0 && (
        <p className="dr-hint"><BellOff size={14} /><strong>Silenced:</strong>{' '}
          {silenced.map((t) => (
            <span key={t.id}><strong>{t.name}</strong>{t.until_fixed ? ' until fixed' : ` until ${timeLabel(t.silenced_until)}`} </span>
          ))}
        </p>
      )}
      {events && events.length === 0 && <p className="dr-hint">No alerts yet. State changes and reminders will show here.</p>}
      {events && events.length > 0 && (
        <div className="panel" style={{ overflowX: 'auto', padding: 0 }}>
          <table className="events">
            <thead><tr><th>Time</th><th>App</th><th>Change</th><th>Reason</th><th>Delivery</th></tr></thead>
            <tbody>
              {events.map((e, i) => {
                const t = targets[e.target_id];
                const prevTargetId = i > 0 ? events[i - 1].target_id : null;
                const showApp = e.target_id !== prevTargetId;
                return (
                  <tr key={e.id}>
                    <td>{timeLabel(e.at)}</td>
                    <td>{showApp ? (t ? <a href={hrefFor(`/apps/${t.id}`)}>{t.name}</a> : <span className="dr-mono">{e.target_id}</span>) : ''}</td>
                    <td className={`state-${e.to}`}>{e.reminder ? `still ${e.to} (reminder)` : `${e.from} → ${e.to}`}</td>
                    <td className="dr-mono">{e.cause || ''}</td>
                    <td>{delivery(e)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};
