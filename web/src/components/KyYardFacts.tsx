import React from 'react';
import type { ContainerFacts } from '../monitor';
import { sinceLabel, timeLabel } from '../monitor';

const mib = (n: number) => `${Math.round(n / 1048576)} MiB`;

export const KyYardFacts: React.FC<{ facts: ContainerFacts | null; link?: string; paired: boolean; pending?: boolean; isAdmin: boolean }> = ({ facts, link, paired, pending, isAdmin }) => {
  if (!link) {
    return <p className="dr-hint">Not linked to a KyYard container.{isAdmin ? ' Edit the app and pick one under Container.' : ''}</p>;
  }
  if (!paired) return <p className="dr-hint">KyYard is not paired.</p>;
  if (!facts) {
    if (pending) return <p className="dr-hint">KyYard: first pull pending.</p>;
    return <p className="dr-hint">Not seen in KyYard: <span className="dr-mono">{link}</span> is not in the latest inventory.</p>;
  }
  return (
    <div className="dr-facts" aria-label="KyYard container">
      <div className="dr-fact"><span className="dr-fact-label">Container</span><span className="dr-fact-value dr-mono">{facts.name} on {facts.endpoint_name}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">State</span><span className={`dr-fact-value state-${facts.state === 'running' ? 'ok' : 'down'}`}>{facts.state}{facts.exit_code !== undefined ? ` (exit ${facts.exit_code})` : ''}</span><span className="dr-fact-note">{facts.status}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Docker health</span><span className="dr-fact-value">{facts.health}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Image</span><span className="dr-fact-value dr-mono">{facts.image}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Memory</span><span className="dr-fact-value">{facts.memory_limit > 0 ? `${mib(facts.memory_bytes)} of ${mib(facts.memory_limit)}` : mib(facts.memory_bytes)}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Restarts</span><span className="dr-fact-value">{facts.restarts_last_hour} in the last hour</span><span className="dr-fact-note">{facts.restart_count} total</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Observed</span><span className="dr-fact-value">{timeLabel(facts.observed_at)} ({sinceLabel(facts.observed_at)} ago){facts.stale ? ' · stale' : ''}</span></div>
    </div>
  );
};
