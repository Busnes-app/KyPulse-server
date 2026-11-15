import React, { useEffect, useState } from 'react';
import { Mail, Send, Trash2 } from 'lucide-react';
import { ApiError, deleteKyPostMail, getKyPostMail, saveKyPostMail, testKyPostMail, timeLabel, type KyPostMailInfo } from '../monitor';

const reasons: Record<string, string> = {
  pending: 'KyPulse is still asking KyIdentity for KyPost.',
  no_peer: 'KyIdentity has no KyPost paired with KyPulse.',
  unauthorized: "KyIdentity refused KyPulse's client credentials.",
  unreachable: 'KyIdentity did not answer; KyPulse keeps asking.',
  invalid_peer: "KyIdentity's KyPost address is not a plain https origin.",
};

interface Props { onChanged: () => void }

// KyPostMailCard is the second alert channel: one plain-text mail per alert from a member's
// KyPost mailbox. KyPost owns the rules (the member's switch, confirmed recipients, rate
// limit); this card picks the member and the recipients and shows KyPost's last answer.
export const KyPostMailCard: React.FC<Props> = ({ onChanged }) => {
  const [info, setInfo] = useState<KyPostMailInfo | null>(null);
  const [owner, setOwner] = useState('');
  const [recipients, setRecipients] = useState('');
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const apply = (m: KyPostMailInfo) => {
    setInfo(m);
    setOwner(m.owner_id ?? '');
    setRecipients((m.recipients ?? []).join('\n'));
  };
  const load = () => getKyPostMail().then(apply);

  useEffect(() => {
    load().catch((err) => setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not load the KyPost channel' }));
  }, []);

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    setBusy(true);
    setMessage(null);
    try {
      await fn();
      setMessage({ kind: 'ok', text: ok });
      onChanged();
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Request failed' });
    } finally {
      setBusy(false);
      await load().catch(() => {});
    }
  };

  const heading = (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
      <Mail size={20} style={{ color: 'var(--ink)' }} />
      <h3 style={{ fontSize: 16 }}>Email through KyPost</h3>
    </div>
  );
  if (!info) return null;
  if (!info.available) {
    return (
      <section className="panel" aria-label="Email through KyPost">
        {heading}
        <p className="dr-hint">Available when KyPulse signs in through KyIdentity: the mail goes out from a KyIdentity member's KyPost mailbox.</p>
      </section>
    );
  }

  const list = recipients.split(/[\s,]+/).map((r) => r.trim()).filter(Boolean);
  const dirty = owner !== (info.owner_id ?? '') || list.join('\n') !== (info.recipients ?? []).join('\n');
  const save = (e: React.FormEvent) => {
    e.preventDefault();
    void run(() => saveKyPostMail({ owner_id: owner, recipients: list }), 'KyPost channel saved');
  };
  const test = () => run(async () => {
    try {
      await testKyPostMail();
    } catch (err) {
      throw new ApiError(err instanceof ApiError ? err.status : 0, `Test refused: ${err instanceof ApiError ? err.message : 'network'}`);
    }
  }, 'KyPost accepted the test alert');
  const remove = () => {
    if (!window.confirm('Remove the KyPost channel? Alerts keep going to the webhook.')) return;
    void run(deleteKyPostMail, 'KyPost channel removed');
  };

  return (
    <form className="panel" onSubmit={save} aria-label="Email through KyPost">
      {heading}
      <p className="dr-hint"><strong>Same host:</strong> if KyPost runs on the same host as the app that failed, it can fail with it and the mail never leaves. Keep the webhook to an off-host receiver as the main path.</p>
      {!info.paired && <p className="form-error" role="status">Not paired. {reasons[info.reason ?? ''] ?? ''}</p>}
      {info.last && (
        <p className={info.last.ok ? 'form-ok' : 'form-error'}>
          Last send {timeLabel(info.last.at)}: {info.last.ok ? 'accepted by KyPost' : `refused (${info.last.error || 'unknown'})`}
        </p>
      )}
      <div className="form-grid">
        <label>Mailbox owner
          <select value={owner} onChange={(e) => setOwner(e.target.value)} required>
            <option value="">Choose a member…</option>
            {(info.owners ?? []).map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </select>
        </label>
        <label>Recipients (one per line, up to 11)
          <textarea rows={3} value={recipients} onChange={(e) => setRecipients(e.target.value)} required autoComplete="off" />
        </label>
      </div>
      <p className="dr-hint">The owner turns on "Let KyPulse send alerts from my mailbox" in KyPost Settings and lists the recipients there; an address other than their own must confirm by link first. KyPost refuses anything else, and its reason shows here.</p>
      {(info.owners ?? []).length === 0 && <p className="dr-hint">No member has signed in to KyPulse through KyIdentity yet.</p>}
      {message && <p className={message.kind === 'ok' ? 'form-ok' : 'form-error'} role={message.kind === 'ok' ? 'status' : 'alert'}>{message.text}</p>}
      <div className="dr-actions">
        <button type="submit" disabled={busy}>Save channel</button>
        {info.configured && (
          <button type="button" className="btn-secondary" disabled={busy || dirty || !info.paired} title={dirty ? 'Save first' : undefined} onClick={() => void test()}>
            <Send size={14} />Send test
          </button>
        )}
        {info.configured && <button type="button" className="btn-danger" disabled={busy} onClick={remove}><Trash2 size={14} />Remove channel</button>}
      </div>
    </form>
  );
};
