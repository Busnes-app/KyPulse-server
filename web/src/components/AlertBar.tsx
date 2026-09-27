import React from 'react';
import { AlertTriangle, CheckCircle2, CircleDashed, Database, Send } from 'lucide-react';
import { hrefFor } from '../router';
import { kyYardPending, sinceLabel, timeLabel, webhookFailing, type StatusSummary } from '../monitor';

interface AlertBarProps {
  status: StatusSummary | null;
  loading: boolean;
  isAdmin: boolean;
}

// AlertBar sits above every page: red with one line per problem, green when all is well,
// plus a delivery warning when the webhook keeps failing.
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
    <div className="alert-bar-line alert-bar-delivery">
      <Send size={14} />
      <span>Alerts not being delivered ({status.webhook.last?.error || 'failed'}) · {isAdmin ? <a href={hrefFor('/settings')}>Settings</a> : 'tell an admin'}</span>
    </div>
  ) : null;
  const yardPending = !!status.kyyard?.paired && kyYardPending(status.kyyard);
  const yard = status.kyyard?.paired && status.kyyard.stale && !yardPending ? (
    <div className="alert-bar-line alert-bar-delivery">
      <Database size={14} />
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
    return (
      <div role="alert" className="alert-bar alert-bar-down">
        {problems.map((p) => (
          <div key={p.id} className="alert-bar-line">
            <AlertTriangle size={16} />
            <a href={hrefFor(`/apps/${p.id}`)}>
              {p.name} {p.state} since {timeLabel(p.since)} ({sinceLabel(p.since)}{p.cause ? `, ${p.cause}` : ''})
            </a>
          </div>
        ))}
        <a className="alert-bar-link" href={hrefFor('/alerts')}>Alerts</a>
        {delivery}
        {yard}
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
        <span>All {status.ok} app{status.ok === 1 ? '' : 's'} healthy{pendingNote} · last check {timeLabel(status.checked_at)}</span>
      </div>
      {delivery}
      {yard}
    </div>
  );
};
