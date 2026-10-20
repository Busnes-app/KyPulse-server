import React, { useCallback, useEffect, useState } from 'react';
import { Activity, Plus, BellOff } from 'lucide-react';
import { TargetForm } from '../components/TargetForm';
import { hrefFor } from '../router';
import {
  ApiError, createTarget, isSilenced, kyYardContainers, listTargets, sinceLabel, sortTargets, stateClass, stateLabel, timeLabel,
  type Suggestion, type Target, type TargetInput,
} from '../monitor';

export interface PageUser { role: string }

interface StatusProps {
  user: PageUser;
  onChanged: () => void;
}

// Status keeps the existing failure-first ordering in a compact, responsive list.
export const Status: React.FC<StatusProps> = ({ user, onChanged }) => {
  const [targets, setTargets] = useState<Target[] | null>(null);
  const [error, setError] = useState('');
  const [adding, setAdding] = useState(false);
  const [suggestions, setSuggestions] = useState<Suggestion[]>([]);
  const isAdmin = user.role === 'admin';

  const load = useCallback(async () => {
    try {
      setTargets(sortTargets(await listTargets()));
      setError('');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not load the apps');
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = window.setInterval(() => void load(), 15_000);
    return () => window.clearInterval(timer);
  }, [load]);

  useEffect(() => {
    if (!isAdmin) return;
    kyYardContainers().then(setSuggestions).catch(() => setSuggestions([]));
  }, [isAdmin]);

  const add = async (input: TargetInput) => {
    await createTarget(input);
    setAdding(false);
    await load();
    onChanged();
  };

  return (
    <div className="dr-page">
      <div className="dr-header">
        <h1 style={{ display: 'flex', alignItems: 'center', gap: 10 }}><Activity size={24} style={{ color: 'var(--accent)' }} />Status</h1>
        {isAdmin && !adding && (
          <button type="button" onClick={() => setAdding(true)}><Plus size={14} />Add app</button>
        )}
      </div>
      <p className="status-intro">Every watched app, with problems first. Open an app for checks, history, and alert controls.</p>
      {targets === null && !error && <p role="status" className="dr-hint">Loading watched apps…</p>}
      {adding && <div className="dr-section"><TargetForm submitLabel="Save" suggestions={suggestions} onSubmit={add} onCancel={() => setAdding(false)} /></div>}
      {error && <p className="form-error" role="alert">{error}</p>}
      {targets && targets.length === 0 && !adding && (
        <p className="dr-hint">No apps are watched yet.{isAdmin ? ' Add one to start polling its /healthz.' : ' Ask an admin to add one.'}</p>
      )}
      {targets && targets.length > 0 && (
        <section className="status-list" aria-label="Watched applications">
          <div className="status-list-heading" aria-hidden="true"><span>Application</span><span>Status</span><span>Latest check</span></div>
          <ul>
            {targets.map((t) => (
              <li key={t.id}>
                <a className="status-row" href={hrefFor(`/apps/${t.id}`)}>
                  <span className="status-app"><span className="status-tile-name"><span className={`dot dot-${stateClass(t)}`} aria-hidden="true" />{t.name}</span><span className="status-endpoint">{t.url}</span></span>
                  <span className="status-state"><span className={`state-${stateClass(t)}`}>{stateLabel(t)}{t.cause && t.enabled && t.state !== 'ok' ? ` · ${t.cause}` : ''}</span><span className="status-tile-meta">{!t.last_polled_at ? 'awaiting first check' : t.state === 'pending' ? 'not yet classified' : `for ${sinceLabel(t.state_since)}`}{isSilenced(t) ? <> · <BellOff size={12} aria-hidden="true" /> silenced</> : null}</span></span>
                  <span className="status-check">{t.last_polled_at ? <>checked {timeLabel(t.last_polled_at)}<span>{t.last_latency_ms} ms</span></> : 'No check yet'}</span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
};
