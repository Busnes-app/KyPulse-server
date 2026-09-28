import { KyYardCollection } from '../components/KyYardCollectors';
import { useState } from 'react';
import { getActivity, type LogUser } from '../logs';
import { PageControls, useLogPage } from '../components/LogTimeline';

const empty = { app: '', actor: '', outcome: '', from: '', to: '' };
export function Activity({ user }: { user: LogUser }) {
  return user.role === 'admin' ? <AdminActivity key={user.id ?? ''} /> : null;
}
function AdminActivity() {
  const [filters, setFilters] = useState(empty);
  const [appliedApp, setAppliedApp] = useState('');
  const [rangeError, setRangeError] = useState('');
  const page = useLogPage(getActivity);
  const apply = () => {
    if (filters.from && filters.to && filters.from > filters.to) { setRangeError('From must be before To.'); return; }
    const query = new URLSearchParams({ limit: '100' });
    for (const [key, value] of Object.entries(filters)) if (value) query.set(key, key === 'from' || key === 'to' ? new Date(`${value}Z`).toISOString() : value);
    setRangeError(''); setAppliedApp(filters.app); page.apply(query.toString());
  };
  return <div className="dr-page"><h1>Activity</h1><p className="dr-hint">Audit events retained for up to 7 days, subject to the shared size limit. Imported chains are not verified. Times are UTC.</p>
    <KyYardCollection />
    <form className="log-filters" onSubmit={event => { event.preventDefault(); apply(); }}>
      <label>Activity app<input value={filters.app} maxLength={256} onChange={e => setFilters({ ...filters, app: e.target.value })} /></label>
      <label>Actor<input value={filters.actor} maxLength={256} onChange={e => setFilters({ ...filters, actor: e.target.value })} /></label>
      <label>Outcome<input value={filters.outcome} onChange={e => setFilters({ ...filters, outcome: e.target.value })} /></label>
      <label>From (UTC)<input type="datetime-local" value={filters.from} onChange={e => setFilters({ ...filters, from: e.target.value })} /></label>
      <label>To (UTC)<input type="datetime-local" value={filters.to} onChange={e => setFilters({ ...filters, to: e.target.value })} /></label>
      <button type="submit">Apply</button><button type="button" onClick={() => { setFilters(empty); setAppliedApp(''); setRangeError(''); page.apply('limit=100'); }}>Clear</button>
    </form>
    {rangeError && <p role="alert">{rangeError}</p>}
    <PageControls page={page} />
    {!!page.bursts.length && <section className="dr-section" aria-label="Failed sign-in bursts"><h2>Failed sign-in bursts</h2>
      <p>Screen-only triage hint: 5 or more known failed sign-ins for the same app and actor or IP within 5 minutes. This is not a security verdict and sends no webhook.</p>
      <ul>{page.bursts.map(b => <li key={JSON.stringify([b.app, b.actor, b.ip])}>{b.count} failed sign-ins · {b.app || 'Unspecified app'} · {b.actor ? `actor ${b.actor}` : `IP ${b.ip}`} · {b.from} – {b.to}</li>)}</ul>
    </section>}
    {!page.loading && !page.error && !page.items.length && <p>{appliedApp && !page.has_app_events ? 'no audit events: not on shared logging' : 'No audit events match these filters.'} Within the retained 7-day window; filters may exclude retained events.</p>}
    <ol className="log-lines">{page.items.map(event => <li key={event.id}>
      <div><time>{event.time}</time> · {event.app || 'Unspecified app'} · {event.actor || 'Unknown actor'} · {event.action}</div>
      <div>Target: {event.target || '—'} · Outcome: {event.outcome || '—'} · IP: {event.ip || '—'}</div>
    </li>)}</ol>
  </div>;
}
