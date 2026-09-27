import React from 'react';
import { AlertTriangle, CheckCircle2, CircleDashed, Send } from 'lucide-react';
import { hrefFor } from '../router';
import { sinceLabel, timeLabel, webhookFailing, type StatusSummary } from '../monitor';

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
  const active = status.total - status.paused;
  const pendingNote = status.pending > 0 ? `, ${status.pending} awaiting first check` : '';
  return (
    <div role="status" className="alert-bar alert-bar-ok">
      <div className="alert-bar-line">
        <CheckCircle2 size={16} />
        <span>All {active} apps healthy{pendingNote} · last check {timeLabel(status.checked_at)}</span>
      </div>
      {delivery}
    </div>
  );
};
