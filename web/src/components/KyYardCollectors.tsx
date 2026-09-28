import { useEffect, useState } from 'react';
import { getKyYard, sinceLabel, type KyYardStatus } from '../monitor';

export function KyYardCollectors({ status }: { status: KyYardStatus }) {
  if (!status.paired) return <p className="dr-hint">KyYard is not paired. Pair it in Settings to collect linked container logs and activity.</p>;
  const collectors = [
    { label: 'Inventory', state: status.inventory },
    { label: 'Container logs', state: status.logs },
    { label: 'Audit feed', state: status.audit },
  ];
  return <section aria-label="KyYard collection" className="dr-section">
    <h3>KyYard collection</h3>
    <ul>{collectors.map(({ label, state }) => <li key={label}>{label}: {state?.last_success ? `last success ${sinceLabel(state.last_success)} ago` : 'no successful collection yet'} · {state?.stale === false ? 'fresh' : 'stale'}{state?.error ? ` (${state.error})` : ''}</li>)}</ul>
    <p className="dr-hint">Container pulls keep the newest 1,000 lines and replay the last timestamp inclusively. History may have gaps at high volume; use file/Docker sender input or your external collector for complete collection.</p>
    {status.audit?.error === 'audit_cursor_unsupported' && <p className="form-error">This KyYard needs an audit cursor API update. Container logs and health polling continue independently.</p>}
  </section>;
}

// Admin Logs/Activity own a cancellable status subscription, independently of filters.
export function KyYardCollection() {
  const [status, setStatus] = useState<KyYardStatus | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let active = true;
    let request: AbortController | undefined;
    const load = () => {
      request?.abort();
      request = new AbortController();
      getKyYard(request.signal).then(value => { if (active) { setStatus(value); setFailed(false); } }).catch(error => {
        if (active && !(error instanceof DOMException && error.name === 'AbortError')) setFailed(true);
      });
    };
    load();
    const timer = window.setInterval(load, 15_000);
    return () => { active = false; request?.abort(); window.clearInterval(timer); };
  }, []);
  return <>{failed && <p className="form-error">KyYard collection status unavailable.</p>}{status && <KyYardCollectors status={status} />}</>;
}
