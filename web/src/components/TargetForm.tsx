import React, { useState } from 'react';
import { Loader2 } from 'lucide-react';
import { ApiError, TLS_SCHEME, type Suggestion, type Target, type TargetInput } from '../monitor';

interface TargetFormProps {
  initial?: Target;
  submitLabel: string;
  suggestions?: Suggestion[];
  onSubmit: (input: TargetInput) => Promise<void>;
  onCancel: () => void;
}

// TargetForm is the add and edit form for a watched app. Validation lives on the server; the
// form only keeps the numbers in range and shows the server's message.
export const TargetForm: React.FC<TargetFormProps> = ({ initial, submitLabel, suggestions, onSubmit, onCancel }) => {
  const [name, setName] = useState(initial?.name ?? '');
  const initialTLS = !!initial?.url.startsWith(TLS_SCHEME);
  const [tlsOnly, setTlsOnly] = useState(initialTLS);
  const [url, setUrl] = useState(initialTLS ? '' : initial?.url ?? '');
  const [hostPort, setHostPort] = useState(initialTLS ? initial!.url.slice(TLS_SCHEME.length) : '');
  const [sni, setSni] = useState(initial?.sni ?? '');
  const [allowLocal, setAllowLocal] = useState(initial?.allow_local ?? false);
  const [interval, setInterval_] = useState(initial?.interval_sec ?? 30);
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [container, setContainer] = useState(initial?.container ?? '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  // Commit on blur: an exact link can also be a prefix of the one still being typed.
  const pickContainer = (value: string) => {
    const s = suggestions?.find((x) => x.link === value);
    if (s && !name.trim()) setName(s.name);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const target = tlsOnly
        ? { url: TLS_SCHEME + hostPort.trim(), sni: sni.trim() || undefined, allow_local: allowLocal }
        : { url: url.trim() };
      await onSubmit({ name: name.trim(), ...target, interval_sec: interval, enabled, container: container.trim() || undefined });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save the app');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} aria-label={submitLabel}>
      <div className="form-grid">
        <label>Name<input value={name} onChange={(e) => setName(e.target.value)} required maxLength={64} /></label>
        <fieldset className="form-choice">
          <legend>Check</legend>
          <label><input type="radio" name="check-kind" checked={!tlsOnly} onChange={() => setTlsOnly(false)} />Health check</label>
          <label><input type="radio" name="check-kind" checked={tlsOnly} onChange={() => setTlsOnly(true)} />TLS certificate only</label>
        </fieldset>
        {!tlsOnly && <label>Health URL<input type="url" value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://app.lan/healthz" /></label>}
        {tlsOnly && <>
          <label>Host and port<input value={hostPort} onChange={(e) => setHostPort(e.target.value)} required placeholder="edge.lan:443" /></label>
          <label>Server name (optional)<input value={sni} onChange={(e) => setSni(e.target.value)} placeholder="defaults to the host" /></label>
          <label style={{ flexDirection: 'row', alignItems: 'center', gap: 8, minHeight: 40 }}>
            <input type="checkbox" style={{ width: 'auto' }} checked={allowLocal} onChange={(e) => setAllowLocal(e.target.checked)} />Allow a loopback or link-local address
          </label>
        </>}
        <label>Interval (seconds)<input type="number" min={10} max={3600} value={interval} onChange={(e) => setInterval_(Number(e.target.value))} required /></label>
        <label style={{ flexDirection: 'row', alignItems: 'center', alignSelf: 'end', gap: 8, minHeight: 40 }}>
          <input type="checkbox" style={{ width: 'auto' }} checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />Enabled
        </label>
        <label>KyYard container
          <input list="kyyard-suggestions" value={container} onChange={(e) => setContainer(e.target.value)} onBlur={(e) => pickContainer(e.currentTarget.value)} placeholder="endpoint/name" />
        </label>
        {suggestions && suggestions.length > 0 && (
          <datalist id="kyyard-suggestions">
            {suggestions.map((s) => <option key={s.link} value={s.link}>{s.endpoint_name}/{s.name} ({s.image})</option>)}
          </datalist>
        )}
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      <div className="dr-actions">
        <button type="submit" disabled={busy}>{busy ? <Loader2 size={14} className="animate-spin" /> : null}{busy ? 'Saving…' : submitLabel}</button>
        <button type="button" className="btn-secondary" onClick={onCancel} disabled={busy}>Cancel</button>
      </div>
    </form>
  );
};
