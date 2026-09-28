import { useEffect, useState } from 'react';
import { ApiError } from '../monitor';
import { getLogs, logError, type LogLine, type Page } from '../logs';

const maxRows = 1000;
export function useLogPage<T>(read: (query: string, signal: AbortSignal) => Promise<Page<T>>, initial = 'limit=100') {
  const [request, setRequest] = useState({ query: initial, before: 0, attempt: 0 });
  const [page, setPage] = useState<Page<T>>({ items: [], next_before_id: 0, bursts: [], has_app_events: false });
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError('');
    if (!request.before) setPage({ items: [], next_before_id: 0, bursts: [], has_app_events: false });
    const query = request.query + (request.before ? `&before_id=${request.before}` : '');
    read(query, controller.signal).then(result => {
      if (controller.signal.aborted) return;
      setPage(previous => ({ ...result, items: (request.before ? [...previous.items, ...result.items] : result.items).slice(0, maxRows) }));
    }).catch(err => {
      if (controller.signal.aborted) return;
      if (err instanceof ApiError && (err.status === 401 || err.status === 403)) setPage({ items: [], next_before_id: 0, bursts: [], has_app_events: false });
      setError(logError(err));
    }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [read, request]);
  return { ...page, loading, error, capped: page.items.length >= maxRows,
    apply: (query: string) => setRequest(r => ({ query, before: 0, attempt: r.attempt + 1 })),
    retry: () => setRequest(r => ({ ...r, attempt: r.attempt + 1 })),
    more: () => setRequest(r => ({ ...r, before: page.next_before_id })),
  };
}

export function LogLines({ items }: { items: LogLine[] }) {
  return <ol className="log-lines">{items.map(line => <li key={line.id}>
    <div className="dr-hint">{line.app || 'Unspecified app'} · {line.source} · <time>{line.time}</time>
      {line.received_at !== line.time && <span> · received {line.received_at}</span>} · {line.level || 'unstructured'}
      {line.truncated && <strong> · <span>truncated</span></strong>}
    </div>
    <pre>{line.message}</pre>
    <details><summary>Raw line</summary><pre>{line.raw}</pre></details>
  </li>)}</ol>;
}

export function RecentLogs({ id }: { id: string }) {
  const page = useLogPage(getLogs, `target_id=${encodeURIComponent(id)}&limit=20`);
  return <section className="dr-section" aria-label="Recent logs"><h3>Recent logs</h3>
    {page.error && <p role="alert">{page.error} <button onClick={page.retry}>Retry recent logs</button></p>}
    {page.loading && <p role="status">Loading recent logs…</p>}
    {!page.loading && !page.error && !page.items.length && <p>No retained logs for this app.</p>}
    <LogLines items={page.items} />
  </section>;
}

export function PageControls({ page }: { page: { loading: boolean; error: string; next_before_id: number; capped: boolean; retry: () => void; more: () => void } }) {
  return <>
    {page.error && <p className="form-error" role="alert">{page.error} <button type="button" onClick={page.retry}>Retry</button></p>}
    {page.loading && <p role="status">Loading…</p>}
    {page.capped ? <p>Showing the newest 1,000 matches. Narrow the filters to see more.</p> : page.next_before_id > 0 && <button type="button" disabled={page.loading || !!page.error} onClick={page.more}>Load more</button>}
  </>;
}
