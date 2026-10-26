import React from 'react';
import { Database } from 'lucide-react';
import { WebhookForm } from '../components/WebhookForm';
import { KyYardCard } from '../components/KyYardCard';
import { TwoFactorCard } from '../components/TwoFactorCard';

interface SettingsProps {
  settings: any;
  user: { id?: string; role: string; sso_provider?: string; totp_enabled?: boolean };
  onChanged: () => void;
  onTOTPChange?: (enabled: boolean) => void;
}

export const Settings: React.FC<SettingsProps> = ({ settings, user, onChanged, onTOTPChange }) => {
  return (
    <div className="dr-page">
      <div className="dr-header"><h1>Settings</h1></div>
      <p className="status-intro">Two-factor sign-in, alert webhook, KyYard pairing and storage. Themes are under <a href="#/appearance">Appearance</a>.</p>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 480px), 1fr))', gap: '20px' }}>
        {/* Local accounts only: an identity provider owns MFA for SSO accounts. */}
        {user.sso_provider === 'local' && <TwoFactorCard key={user.id} enabled={!!user.totp_enabled} onEnabledChange={onTOTPChange} />}
        {user.role === 'admin' && <WebhookForm onChanged={onChanged} />}
        {user.role === 'admin' && <KyYardCard onChanged={onChanged} />}
        {/* Storage Card */}
        <div className="panel">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
            <Database size={20} style={{ color: 'var(--accent)' }} />
            <h3 style={{ fontSize: '16px' }}>Storage</h3>
          </div>
          <p style={{ color: 'var(--ink)', fontSize: '13px', marginBottom: '16px' }}>
            The database kyPulse keeps its apps, alerts and audit trail in. Set by <code className="font-mono">KYPULSE_DB_DRIVER</code>.
          </p>

          <div style={{ background: 'var(--bg)', border: '1px solid var(--line)', borderRadius: '6px', padding: '16px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '13px' }}>
              <span style={{ color: 'var(--ink)' }}>Database:</span>
              <span className="badge badge-accent font-mono">{settings?.db_driver || 'sqlite'}</span>
            </div>
          </div>
        </div>

      </div>
    </div>
  );
};
