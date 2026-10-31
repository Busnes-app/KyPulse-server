import React from 'react';
import { AlertTriangle, CheckCircle2, CircleDashed, Database, Send } from 'lucide-react';
import { hrefFor } from '../router';
import { kyYardPending, sinceLabel, webhookFailing, type StatusSummary } from '../monitor';

interface AlertBarProps {
  status: StatusSummary | null;
  loading: boolean;
  isAdmin: boolean;
}

// AlertBar sits above every page as one fixed-height line: the problem counts and a pill per
// problem app (exact times and causes live on its detail page), green when all is well, plus
// chips for failing alert delivery and stale KyYard data.
export const AlertBar: React.FC<AlertBarProps> = ({ status, loading, isAdmin }) => {
  if (!status) {
    return (
      <div role="status" className="alert-bar alert-bar-neutral">
        <CircleDashed size={16} />
        <span>{loading ? 'Checking…' : 'Status unavailable'}</span>
      </div>
    );
  }
  const delivery = webhookFailing(status) ? (
    <div className="ky-status" data-ky-status="warning">
      <Send aria-hidden="true" />
      <span>Alerts not being delivered ({status.webhook.last?.error || 'failed'}) · {isAdmin ? <a href={hrefFor('/settings')}>Settings</a> : 'tell an admin'}</span>
    </div>
  ) : null;
  const yardPending = !!status.kyyard?.paired && kyYardPending(status.kyyard);
  const yard = status.kyyard?.paired && status.kyyard.stale && !yardPending ? (
    <div className="ky-status" data-ky-status="warning">
      <Database aria-hidden="true" />
      <span>KyYard data stale{status.kyyard.error ? ` (${status.kyyard.error})` : ''}{status.kyyard.fetched_at ? `, last pull ${sinceLabel(status.kyyard.fetched_at)} ago` : ', never pulled'} · {isAdmin ? <a href={hrefFor('/settings')}>Settings</a> : 'tell an admin'}</span>
    </div>
  ) : yardPending ? (
    <div className="alert-bar-line">
      <Database size={14} />
      <span>KyYard: first pull pending</span>
    </div>
  ) : null;

  const problems = status.problems ?? [];
  if (problems.length > 0) {
    const counts = [status.down && `${status.down} down`, status.degraded && `${status.degraded} degraded`].filter(Boolean).join(' · ');
    return (
      <div role="alert" className={`alert-bar alert-bar-down${status.down ? '' : ' alert-bar-degraded'}`}>
        <AlertTriangle size={16} aria-hidden="true" />
        <strong className="alert-bar-count">{counts || `${problems.length} problem${problems.length === 1 ? '' : 's'}`}</strong>
        <ul className="alert-bar-apps">
          {problems.map((p) => (
            <li key={p.id}>
              <a className="alert-pill" href={hrefFor(`/apps/${p.id}`)} aria-label={`${p.name} ${p.state} for ${sinceLabel(p.since)}`}>
                <span className={`dot dot-${p.state}`} aria-hidden="true" />{p.name}<span className="alert-pill-state">{p.state}</span>
              </a>
            </li>
          ))}
        </ul>
        {delivery}
        {yard}
        <a className="alert-bar-link" href={hrefFor('/alerts')}>Alerts</a>
      </div>
    );
  }
  if (status.total === 0) {
    return (
      <div role="status" className="alert-bar alert-bar-neutral">
        <CircleDashed size={16} />
        <span>No apps watched yet{isAdmin ? <> · <a href={hrefFor('/status')}>add one on Status</a></> : null}</span>
      </div>
    );
  }
  if (status.ok === 0) {
    return (
      <div role="status" className="alert-bar alert-bar-neutral">
        <CircleDashed size={16} />
        <span>{status.pending > 0 ? 'No app has been classified yet' : 'All apps paused'}</span>
        {delivery}
        {yard}
      </div>
    );
  }
  const pendingNote = status.pending > 0 ? `, ${status.pending} not yet classified` : '';
  return (
    <div role="status" className="alert-bar alert-bar-ok">
      <div className="alert-bar-line">
        <CheckCircle2 size={16} />
        <span>All {status.ok} app{status.ok === 1 ? '' : 's'} healthy{pendingNote}{status.checked_at ? ` · last check ${sinceLabel(status.checked_at)} ago` : ''}</span>
      </div>
      {delivery}
      {yard}
    </div>
  );
};
