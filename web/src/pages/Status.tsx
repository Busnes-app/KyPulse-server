import React, { useCallback, useEffect, useState } from 'react';
import { Plus, BellOff } from 'lucide-react';
import { TargetDialog } from '../components/TargetDialog';
import { Heartbeat } from '../components/Heartbeat';
import { PageHead } from '../components/PageHead';
import { hrefFor } from '../router';
import {
  ApiError, createTarget, hostPath, isSilenced, kyYardContainers, listTargets, sinceLabel, sortTargets, stateClass, stateLabel, uptimeLabel, causeLabel,
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
      <PageHead title="Status" lede="Every watched app, problems first; open one for its checks, history and alert controls."
        action={isAdmin && <button type="button" className="ky-page-action" onClick={() => setAdding(true)}><Plus aria-hidden="true" />Add app</button>} />
      {targets === null && !error && <p role="status" className="dr-hint">Loading watched apps…</p>}
      {adding && <TargetDialog title="Add app" submitLabel="Save" suggestions={suggestions} onSubmit={add} onClose={() => setAdding(false)} />}
      {error && <p className="form-error" role="alert">{error}</p>}
      {targets && targets.length === 0 && (
        <p className="dr-hint">No apps are watched yet.{isAdmin ? ' Add one to start polling its /healthz.' : ' Ask an admin to add one.'}</p>
      )}
      {targets && targets.length > 0 && (
        <section className="status-list" aria-label="Watched applications">
          <div className="status-list-heading" aria-hidden="true">
            <span>Application</span><span>Last 24 checks</span><span className="num">Uptime (24 h)</span><span>Status</span><span>Checked</span><span className="num">Latency</span>
          </div>
          <ul>
            {targets.map((t) => <li key={t.id}><StatusRow t={t} /></li>)}
          </ul>
        </section>
      )}
    </div>
  );
};

// One 52px row: name and endpoint, recent polls, the day's uptime, state, check age, latency.
function StatusRow({ t }: { t: Target }) {
  const cls = stateClass(t);
  const cause = t.cause && t.enabled && t.state !== 'ok' ? causeLabel(t.cause) : '';
  const age = !t.last_polled_at ? 'awaiting first check' : t.state === 'pending' ? 'not yet classified' : `for ${sinceLabel(t.state_since)}`;
  return (
    <a className="status-row" href={hrefFor(`/apps/${t.id}`)}>
      <span className="status-app">
        <span className={`dot dot-${cls}`} aria-hidden="true" />
        <span className="status-name">{t.name}</span>
        <span className="status-endpoint" title={t.url}>{hostPath(t.url)}</span>
      </span>
      <Heartbeat beats={t.recent} />
      <span className="status-uptime num" title="Share of the past 24 hours (or since the app was added) it was OK">{uptimeLabel(t.uptime_24h)}</span>
      <span className="status-state" title={cause || undefined}>
        <span className={`state-${cls}`}>{stateLabel(t)}</span>
        {cause && <span className="status-cause"> · {cause}</span>}
        <span className="status-meta"> {age}</span>
        {isSilenced(t) && <span className="status-meta"> · <BellOff size={12} aria-hidden="true" /> silenced</span>}
      </span>
      <span className="status-checked">{t.last_polled_at ? `${sinceLabel(t.last_polled_at)} ago` : '—'}</span>
      <span className="status-latency num">{t.last_polled_at ? `${t.last_latency_ms} ms` : '—'}</span>
    </a>
  );
}
