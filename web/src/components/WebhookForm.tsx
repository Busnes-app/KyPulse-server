import React, { useEffect, useState } from 'react';
import { Send, Trash2 } from 'lucide-react';
import { ApiError, deleteWebhook, getWebhook, saveWebhook, testWebhook, timeLabel, type WebhookInfo } from '../monitor';

const presets = [
  { id: 'ntfy', label: 'ntfy', hint: 'Topic URL, e.g. https://ntfy.sh/kypulse. Token is optional (access token).' },
  { id: 'gotify', label: 'Gotify', hint: 'Server URL. The app token goes in the Token field, never in the URL.' },
  { id: 'discord', label: 'Discord', hint: 'Channel webhook URL. No token.' },
  { id: 'generic', label: 'Generic JSON', hint: 'Any HTTPS endpoint; token is sent as a Bearer header when set.' },
];

interface WebhookFormProps { onChanged: () => void }

// WebhookForm configures the one outbound alert webhook. The token is write-only: the server
// only reports whether one is saved, and an empty field keeps it.
export const WebhookForm: React.FC<WebhookFormProps> = ({ onChanged }) => {
  const [info, setInfo] = useState<WebhookInfo | null>(null);
  const [preset, setPreset] = useState('ntfy');
  const [url, setUrl] = useState('');
  const [token, setToken] = useState('');
  const [clearToken, setClearToken] = useState(false);
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const apply = (w: WebhookInfo) => {
    setInfo(w);
    if (w.configured) {
      setPreset(w.preset ?? 'ntfy');
      setUrl(w.url ?? '');
    } else {
      setPreset('ntfy');
      setUrl('');
    }
    setToken('');
    setClearToken(false);
  };

  useEffect(() => {
    getWebhook().then(apply).catch((err) => setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not load the webhook' }));
  }, []);

  const run = async (fn: () => Promise<WebhookInfo | void>, ok: string) => {
    setBusy(true);
    setMessage(null);
    try {
      const out = await fn();
      if (out) apply(out);
      setMessage({ kind: 'ok', text: ok });
      onChanged();
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Request failed' });
    } finally {
      setBusy(false);
    }
  };

  const save = (e: React.FormEvent) => {
    e.preventDefault();
    void run(() => saveWebhook({ preset, url: url.trim(), token, clear_token: clearToken }), 'Webhook saved');
  };
  const test = () => run(async () => {
    try {
      await testWebhook();
    } catch (err) {
      throw new ApiError(err instanceof ApiError ? err.status : 0, `Test failed: ${err instanceof ApiError ? err.message : 'network'}`);
    }
    return getWebhook();
  }, 'Test message delivered');
  const remove = () => {
    if (!window.confirm('Remove the webhook? Alerts stop being delivered.')) return;
    void run(async () => { await deleteWebhook(); return { configured: false }; }, 'Webhook removed');
  };

  const hint = presets.find((p) => p.id === preset)?.hint;
  return (
    <form className="panel" onSubmit={save} aria-label="Alert webhook">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <Send size={20} style={{ color: 'var(--accent)' }} />
        <h3 style={{ fontSize: 16 }}>Alert webhook</h3>
      </div>
      {info && !info.configured && <p className="dr-hint">No webhook configured. Alerts show here but are not delivered anywhere.</p>}
      {info?.configured && info.last && (
        <p className={info.last.ok ? 'form-ok' : 'form-error'}>
          Last delivery {timeLabel(info.last.at)}: {info.last.ok ? 'delivered' : `failed (${info.last.error || 'unknown'})`}
        </p>
      )}
      <div className="form-grid">
        <label>Preset
          <select value={preset} onChange={(e) => setPreset(e.target.value)}>
            {presets.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
          </select>
        </label>
        <label>URL<input type="url" value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://" /></label>
        <label>Token (write-only)
          <input type="password" autoComplete="off" value={token} onChange={(e) => setToken(e.target.value)} disabled={clearToken}
            placeholder={info?.has_token ? 'saved; leave empty to keep' : 'none'} />
        </label>
        {info?.has_token && (
          <label style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
            <input type="checkbox" style={{ width: 'auto' }} checked={clearToken} onChange={(e) => setClearToken(e.target.checked)} />Remove the saved token
          </label>
        )}
      </div>
      {hint && <p className="dr-hint">{hint}</p>}
      {message && <p className={message.kind === 'ok' ? 'form-ok' : 'form-error'} role={message.kind === 'ok' ? 'status' : 'alert'}>{message.text}</p>}
      <div className="dr-actions">
        <button type="submit" disabled={busy}>Save webhook</button>
        {info?.configured && <button type="button" className="btn-secondary" disabled={busy} onClick={() => void test()}><Send size={14} />Send test</button>}
        {info?.configured && <button type="button" className="btn-danger" disabled={busy} onClick={remove}><Trash2 size={14} />Remove webhook</button>}
      </div>
    </form>
  );
};
