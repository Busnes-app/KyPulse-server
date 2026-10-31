import { beatCount, type Beat } from '../monitor';

const W = 80;
const H = 24;
const PAD = 3; // room for the end dot and the 2px line

// Sparkline draws the latency of the last polls on a scale from zero, in Heartbeat's slots so
// the two line up. Down polls break the line: their timing says nothing about speed.
export function Sparkline({ beats }: { beats?: Beat[] | null }) {
  const list = (beats ?? []).slice(-beatCount);
  const answered = list.map((b) => (b.state === 'down' ? null : b.latency_ms));
  const values = answered.filter((v): v is number => v !== null);
  const max = Math.max(1, ...values);
  const offset = beatCount - list.length;
  const x = (i: number) => PAD + ((offset + i) * (W - 2 * PAD)) / (beatCount - 1);
  const y = (ms: number) => H - PAD - (ms / max) * (H - 2 * PAD);
  let d = '';
  answered.forEach((v, i) => {
    if (v === null) return;
    d += `${i > 0 && answered[i - 1] !== null ? 'L' : 'M'}${x(i).toFixed(1)} ${y(v).toFixed(1)}`;
  });
  const last = answered.length - 1;
  const end = last >= 0 && answered[last] !== null ? answered[last] : null;
  return (
    <svg className="sparkline" width={W} height={H} viewBox={`0 0 ${W} ${H}`} role="img">
      <title>{latencySummary(values)}</title>
      <line className="sparkline-base" x1={PAD} x2={W - PAD} y1={H - PAD + 0.5} y2={H - PAD + 0.5} />
      {d && <path d={d} />}
      {end !== null && <circle cx={x(last)} cy={y(end)} r={2.5} />}
    </svg>
  );
}

// latencySummary is the sparkline's text alternative: range and median of the answered polls.
export function latencySummary(values: number[]): string {
  if (values.length === 0) return 'No latency recorded yet';
  const sorted = [...values].sort((a, b) => a - b);
  const median = sorted[Math.floor((sorted.length - 1) / 2)];
  const span = sorted[0] === sorted[sorted.length - 1] ? `${sorted[0]} ms` : `${sorted[0]} to ${sorted[sorted.length - 1]} ms`;
  return `Latency over the last ${values.length} answered check${values.length === 1 ? '' : 's'}: ${span}, median ${median} ms`;
}
