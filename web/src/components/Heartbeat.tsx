import { beatCount, timeLabel, type Beat } from '../monitor';

// Heartbeat draws the last polls as thin bars, oldest left; empty slots fill the start until
// enough polls have run since KyPulse started.
export function Heartbeat({ beats }: { beats?: Beat[] | null }) {
  const list = (beats ?? []).slice(-beatCount);
  const tally = (['ok', 'degraded', 'down'] as const)
    .map((state) => [state, list.filter((b) => b.state === state).length] as const)
    .filter(([, n]) => n > 0).map(([state, n]) => `${n} ${state}`).join(', ');
  return (
    <span className="beats" role="img" aria-label={list.length ? `Last ${list.length} checks: ${tally}` : 'No checks since KyPulse started'}>
      {Array.from({ length: beatCount - list.length }, (_, i) => <span key={`empty-${i}`} className="beat" />)}
      {list.map((b) => <span key={b.at} className={`beat beat-${b.state}`} title={`${b.state} · ${b.latency_ms} ms · ${timeLabel(b.at)}`} />)}
    </span>
  );
}
