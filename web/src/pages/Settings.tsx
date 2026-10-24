import React from 'react';
import { Settings as SettingsIcon, Database, Palette } from 'lucide-react';
import { ThemeSwitcher } from '../components/ThemeSwitcher';
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
    <div style={{ maxWidth: '1080px', margin: '0 auto', padding: '32px 20px' }}>
      <div style={{ marginBottom: '24px' }}>
        <h1 style={{ fontSize: '24px', fontWeight: 'bold', display: 'flex', alignItems: 'center', gap: '10px' }}>
          <SettingsIcon size={24} style={{ color: 'var(--accent)' }} />
          <span>Settings</span>
        </h1>
        <p style={{ color: 'var(--ink)', fontSize: '14px', marginTop: '4px' }}>
          Two-factor sign-in, alert webhook, KyYard pairing, storage and appearance.
        </p>
      </div>

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

        {/* Appearance Card */}
        <div className="panel">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
            <Palette size={20} style={{ color: 'var(--accent)' }} />
            <h3 style={{ fontSize: '16px' }}>Appearance</h3>
          </div>
          <p style={{ color: 'var(--ink)', fontSize: '13px', marginBottom: '16px' }}>
            The theme follows your system until you pick one. The choice is saved in this browser only.
          </p>

          <div style={{ display: 'flex', alignItems: 'center', gap: '12px' }}>
            <span style={{ fontSize: '13px', color: 'var(--ink)' }}>Theme:</span>
            <ThemeSwitcher />
          </div>
        </div>
      </div>
    </div>
  );
};
