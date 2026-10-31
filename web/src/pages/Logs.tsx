import { KyYardCollection } from '../components/KyYardCollectors';
import { useState } from 'react';
import { getLogs, type LogUser } from '../logs';
import { LogLines, PageControls, useLogPage } from '../components/LogTimeline';
import { LogSources } from '../components/LogSources';
import { PageHead } from '../components/PageHead';

const empty = { app: '', level: '', text: '', from: '', to: '' };
export function Logs({ user }: { user: LogUser }) {
  return user.role === 'admin' ? <AdminLogs key={user.id ?? ''} /> : null;
}
function AdminLogs() {
  const [filters, setFilters] = useState(empty);
  const [hasAppliedFilters, setHasAppliedFilters] = useState(false);
  const page = useLogPage(getLogs);
  const [rangeError, setRangeError] = useState('');
  const apply = () => {
    if (filters.from && filters.to && filters.from > filters.to) { setRangeError('From must be before To.'); return; }
    const query = new URLSearchParams({ limit: '100' });
    for (const [key, value] of Object.entries(filters)) if (value) query.set(key, key === 'from' || key === 'to' ? new Date(`${value}Z`).toISOString() : value);
    setRangeError(''); setHasAppliedFilters(Object.values(filters).some(Boolean)); page.apply(query.toString());
  };
  return <div className="dr-page"><PageHead title="Logs" lede="Retained for up to 7 days, subject to the shared size limit. Times are UTC." />
    <details className="log-filter-panel"><summary>Filters{hasAppliedFilters && <span className="filter-applied"> Applied</span>}</summary>
    <form className="log-filters" onSubmit={event => { event.preventDefault(); apply(); }}>
      <label>Log app<input value={filters.app} maxLength={256} onChange={e => setFilters({ ...filters, app: e.target.value })} /></label>
      <label>Log level<input value={filters.level} onChange={e => setFilters({ ...filters, level: e.target.value })} /></label>
      <label>Literal text<input value={filters.text} maxLength={256} onChange={e => setFilters({ ...filters, text: e.target.value })} /></label>
      <label>From (UTC)<input type="datetime-local" value={filters.from} onChange={e => setFilters({ ...filters, from: e.target.value })} /></label>
      <label>To (UTC)<input type="datetime-local" value={filters.to} onChange={e => setFilters({ ...filters, to: e.target.value })} /></label>
      <div className="filter-actions"><button type="submit">Apply</button><button type="button" className="btn-secondary" onClick={() => { setFilters(empty); setHasAppliedFilters(false); setRangeError(''); page.apply('limit=100'); }}>Clear</button></div>
    </form></details>
    {rangeError && <p role="alert">{rangeError}</p>}
    <PageControls page={page} />
    {!page.loading && !page.error && !page.items.length && <p className="dr-hint log-empty">No logs found.</p>}
    <KyYardCollection />
    <LogLines items={page.items} />
    <LogSources />
  </div>;
}
