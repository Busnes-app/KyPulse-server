import React from 'react';
import { Database } from 'lucide-react';
import { WebhookForm } from '../components/WebhookForm';
import { KyYardCard } from '../components/KyYardCard';
import { TwoFactorCard } from '../components/TwoFactorCard';
import { PageHead } from '../components/PageHead';

interface SettingsProps {
  settings: any;
  user: { id?: string; role: string; sso_provider?: string; totp_enabled?: boolean };
  onChanged: () => void;
  onTOTPChange?: (enabled: boolean) => void;
}

export const Settings: React.FC<SettingsProps> = ({ settings, user, onChanged, onTOTPChange }) => {
  return (
    <div className="dr-page">
      <PageHead title="Settings" lede={<>Two-factor sign-in, alert webhook, KyYard pairing and storage. Themes are under <a href="#/appearance">Appearance</a>.</>} />

      {/* One column of sections at a readable form width; each keeps its own height. */}
      <div className="settings-stack">
        {/* Local accounts only: an identity provider owns MFA for SSO accounts. */}
        {user.sso_provider === 'local' && <TwoFactorCard key={user.id} enabled={!!user.totp_enabled} onEnabledChange={onTOTPChange} />}
        {user.role === 'admin' && <WebhookForm onChanged={onChanged} />}
        {user.role === 'admin' && <KyYardCard onChanged={onChanged} />}
        <section className="panel" aria-labelledby="storage-title">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
            <Database size={20} style={{ color: 'var(--ink)' }} aria-hidden="true" />
            <h3 id="storage-title" style={{ fontSize: '16px' }}>Storage</h3>
          </div>
          <p className="dr-hint">The database KyPulse keeps its apps, alerts and audit trail in. Set by <code className="font-mono">KYPULSE_DB_DRIVER</code>.</p>
          <div className="dr-facts">
            <div className="dr-fact"><span className="dr-fact-label">Database</span><span className="dr-fact-value dr-mono">{settings?.db_driver || 'sqlite'}</span></div>
          </div>
        </section>
      </div>
    </div>
  );
};
