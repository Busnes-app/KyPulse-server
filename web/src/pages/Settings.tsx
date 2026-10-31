import React from 'react';
import { Database, UserRound } from 'lucide-react';
import { WebhookForm } from '../components/WebhookForm';
import { KyYardCard } from '../components/KyYardCard';
import { TwoFactorCard } from '../components/TwoFactorCard';
import { PageHead } from '../components/PageHead';
import { Appearance } from './Appearance';
import { hrefFor, settingsSections, type SettingsSection } from '../router';

interface SettingsUser {
  id?: string;
  role: string;
  username?: string;
  display_name?: string;
  sso_provider?: string;
  totp_enabled?: boolean;
}

interface SettingsProps {
  section: SettingsSection;
  settings: any;
  user: SettingsUser;
  onChanged: () => void;
  onTOTPChange?: (enabled: boolean) => void;
}

/** Settings: the page head, one tab per section (each its own hash), then the open section. */
export const Settings: React.FC<SettingsProps> = ({ section, settings, user, onChanged, onTOTPChange }) => {
  const open = settingsSections.find((s) => s.id === section) ?? settingsSections[0];
  const isAdmin = user.role === 'admin';
  return (
    <div className="dr-page">
      <PageHead title="Settings" lede={isAdmin
        ? 'Your account, how KyPulse looks, and this server’s alert webhook, KyYard pairing and storage.'
        : 'Your account, how KyPulse looks, and where this server keeps its data.'} />
      <nav className="ky-settings-tabs" aria-label="Settings sections">
        {settingsSections.map((s) => (
          <a key={s.id} className="ky-nav-item" href={hrefFor(`/settings/${s.id}`)} aria-current={s === open ? 'page' : undefined}>{s.label}</a>
        ))}
      </nav>
      <section aria-label={open.label}>
        {open.id === 'account' && <Account user={user} onTOTPChange={onTOTPChange} />}
        {open.id === 'appearance' && <Appearance />}
        {open.id === 'server' && <Server settings={settings} isAdmin={isAdmin} onChanged={onChanged} />}
      </section>
    </div>
  );
};

const signInLabel = (provider?: string) => (!provider || provider === 'local' ? 'KyPulse password' : provider === 'kysignon' ? 'Single sign-on (KySignOn)' : 'Single sign-on');

function Account({ user, onTOTPChange }: { user: SettingsUser; onTOTPChange?: (enabled: boolean) => void }) {
  const role = user.role === 'admin' ? 'Administrator' : 'Everyday account';
  return (
    <div className="settings-stack">
      <section className="panel" aria-labelledby="account-title">
        <div style={{ display: 'flex', alignItems: 'center', gap: '8px', marginBottom: '12px' }}>
          <UserRound size={20} style={{ color: 'var(--ink)' }} aria-hidden="true" />
          <h3 id="account-title" style={{ fontSize: '16px' }}>Signed in as</h3>
        </div>
        <div className="dr-facts">
          {/* Never the same word twice: a name that repeats the username or role is left out. */}
          {user.display_name && ![user.username, role].includes(user.display_name) && (
            <div className="dr-fact"><span className="dr-fact-label">Name</span><span className="dr-fact-value">{user.display_name}</span></div>
          )}
          <div className="dr-fact"><span className="dr-fact-label">Username</span><span className="dr-fact-value dr-mono">{user.username}</span></div>
          <div className="dr-fact"><span className="dr-fact-label">Role</span><span className="dr-fact-value">{role}</span></div>
          <div className="dr-fact"><span className="dr-fact-label">Signs in with</span><span className="dr-fact-value">{signInLabel(user.sso_provider)}</span></div>
        </div>
      </section>
      {/* Local accounts only: an identity provider owns MFA for SSO accounts. */}
      {user.sso_provider === 'local' && <TwoFactorCard key={user.id} enabled={!!user.totp_enabled} onEnabledChange={onTOTPChange} />}
    </div>
  );
}

function Server({ settings, isAdmin, onChanged }: { settings: any; isAdmin: boolean; onChanged: () => void }) {
  return (
    <div className="settings-stack">
      {isAdmin && <WebhookForm onChanged={onChanged} />}
      {isAdmin && <KyYardCard onChanged={onChanged} />}
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
  );
}
