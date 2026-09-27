import { useEffect, useRef, useState } from 'react';
import { createPairing, getSources, getSourceTargets, logError, revokeSource, type LogSource, type Pairing } from '../logs';

export function LogSources() {
  const [sources, setSources] = useState<LogSource[]>([]);
  const [targets, setTargets] = useState<{ id: string; name: string }[]>([]);
  const [target, setTarget] = useState('');
  const [name, setName] = useState('my-app');
  const [pairing, setPairing] = useState<Pairing | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [refresh, setRefresh] = useState(0);
  const lifetime = useRef<AbortController | null>(null);

  useEffect(() => {
    const controller = new AbortController(); lifetime.current = controller;
    return () => { controller.abort(); lifetime.current = null; };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true); setError('');
    Promise.all([getSources(controller.signal), getSourceTargets(controller.signal)]).then(([next, apps]) => {
      if (!controller.signal.aborted) { setSources(next); setTargets(apps); }
    }).catch(err => { if (!controller.signal.aborted) { setSources([]); setError(logError(err)); } })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [refresh]);
  useEffect(() => {
    if (!pairing) return;
    const timer = window.setTimeout(() => setPairing(null), Math.max(0, Date.parse(pairing.expires_at) - Date.now()));
    const poll = window.setInterval(() => setRefresh(r => r + 1), 5000);
    return () => { window.clearTimeout(timer); window.clearInterval(poll); };
  }, [pairing]);

  const generate = async () => {
    const controller = lifetime.current;
    if (!controller) return;
    setBusy(true); setError(''); setPairing(null);
    try {
      const next = await createPairing(target, controller.signal);
      if (!controller.signal.aborted && Date.parse(next.expires_at) > Date.now()) setPairing(next);
    } catch (err) { if (!controller.signal.aborted) setError(logError(err)); }
    finally { if (!controller.signal.aborted) setBusy(false); }
  };
  const revoke = async (source: LogSource) => {
    if (!window.confirm(`Revoke ${source.name}? This sender will no longer be able to send logs.`)) return;
    const controller = lifetime.current;
    if (!controller) return;
    setBusy(true); setError('');
    try {
      await revokeSource(source.id, controller.signal);
      if (!controller.signal.aborted) setRefresh(r => r + 1);
    } catch (err) { if (!controller.signal.aborted) setError(logError(err)); }
    finally { if (!controller.signal.aborted) setBusy(false); }
  };
  return <section className="dr-section" aria-label="Log sources"><h2>Log sources</h2>
    <p className="dr-hint">Choose an optional watched app before creating a single-use, 15-minute code. The sender supplies its name when it claims the code.</p>
    <form className="log-filters" onSubmit={e => { e.preventDefault(); void generate(); }}>
      <label>Bind log source to watched app<select value={target} disabled={busy || !!pairing || loading} onChange={e => setTarget(e.target.value)}>
        <option value="">Unbound</option>{targets.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
      </select></label>
      <label>Log source name<input value={name} disabled={busy || !!pairing} required pattern="[A-Za-z0-9][A-Za-z0-9_.\-]{0,63}" maxLength={64} onChange={e => setName(e.target.value)} /></label>
      <button type="submit" disabled={busy || loading || !!pairing}>Add source</button>
      <button type="button" disabled={busy || loading} onClick={() => setRefresh(r => r + 1)}>Refresh sources</button>
    </form>
    {pairing && <div role="status">
      <p>Pairing code: <strong>{pairing.code}</strong> · expires {pairing.expires_at}</p>
      <pre className="log-command">kypulse-send pair --url {window.location.origin} --code {pairing.code} --name {name}</pre>
      <button type="button" onClick={() => setPairing(null)}>Hide code</button>
    </div>}
    {error && <p className="form-error" role="alert">{error}</p>}
    {loading && <p>Loading sources…</p>}
    {!loading && !error && !sources.length && <p>No paired sources.</p>}
    <ul className="log-lines">{sources.map(source => <li key={source.id}>
      <strong>{source.name}</strong> · {targets.find(t => t.id === source.target_id)?.name || (source.target_id ? source.target_id : 'Unbound')} · paired {source.created_at}
      {source.revoked_at ? <span> · Revoked {source.revoked_at}</span> : <button type="button" disabled={busy} onClick={() => void revoke(source)}>Revoke {source.name}</button>}
    </li>)}</ul>
  </section>;
}
