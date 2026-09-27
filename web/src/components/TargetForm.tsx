import React, { useState } from 'react';
import { Loader2 } from 'lucide-react';
import { ApiError, type Target, type TargetInput } from '../monitor';

interface TargetFormProps {
  initial?: Target;
  submitLabel: string;
  onSubmit: (input: TargetInput) => Promise<void>;
  onCancel: () => void;
}

// TargetForm is the add and edit form for a watched app. Validation lives on the server; the
// form only keeps the numbers in range and shows the server's message.
export const TargetForm: React.FC<TargetFormProps> = ({ initial, submitLabel, onSubmit, onCancel }) => {
  const [name, setName] = useState(initial?.name ?? '');
  const [url, setUrl] = useState(initial?.url ?? '');
  const [interval, setInterval_] = useState(initial?.interval_sec ?? 30);
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await onSubmit({ name: name.trim(), url: url.trim(), interval_sec: interval, enabled, container: initial?.container });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not save the app');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="panel" aria-label={submitLabel}>
      <div className="form-grid">
        <label>Name<input value={name} onChange={(e) => setName(e.target.value)} required maxLength={64} /></label>
        <label>Health URL<input type="url" value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://app.lan/healthz" /></label>
        <label>Interval (seconds)<input type="number" min={10} max={3600} value={interval} onChange={(e) => setInterval_(Number(e.target.value))} required /></label>
        <label style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
          <input type="checkbox" style={{ width: 'auto' }} checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />Enabled
        </label>
      </div>
      {error && <p className="form-error" role="alert">{error}</p>}
      <div className="dr-actions">
        <button type="submit" disabled={busy}>{busy ? <Loader2 size={14} className="animate-spin" /> : null}{busy ? 'Saving…' : submitLabel}</button>
        <button type="button" className="btn-secondary" onClick={onCancel} disabled={busy}>Cancel</button>
      </div>
    </form>
  );
};
