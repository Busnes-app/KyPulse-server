import React from 'react';
import { LogOut, Settings as SettingsIcon, Activity, Bell, Archive } from 'lucide-react';
import { ThemeSwitcher } from './ThemeSwitcher';
import { hrefFor } from '../router';

interface AppHeaderProps {
  appName: string;
  activePath: string;
  user: { role: string; display_name?: string; username?: string } | null;
  onLogout: () => void;
}

export const navItems = [
  { path: '/status', label: 'Status', icon: Activity, admin: false },
  { path: '/alerts', label: 'Alerts', icon: Bell, admin: false },
  { path: '/backup', label: 'Backup', icon: Archive, admin: true },
  { path: '/settings', label: 'Settings & DB', icon: SettingsIcon, admin: false },
];

export const AppHeader: React.FC<AppHeaderProps> = ({ appName, activePath, user, onLogout }) => {
  const items = navItems.filter((item) => !item.admin || user?.role === 'admin');
  return (
    <header className="app-header">
      <div className="app-brand">
        <img src="/app-icon.png" width={28} height={28} alt="" />
        <span>{appName || 'kyPulse'}</span>
      </div>

      <nav className="app-nav" aria-label="Primary">
        {items.map((item) => {
          const Icon = item.icon;
          // An app detail page belongs to Status.
          const active = activePath === item.path || (item.path === '/status' && activePath.startsWith('/apps/'));
          return (
            <a
              key={item.path}
              href={hrefFor(item.path)}
              className={active ? 'ky-nav-item active' : 'ky-nav-item'}
              aria-current={active ? 'page' : undefined}
            >
              <Icon size={16} />
              <span>{item.label}</span>
            </a>
          );
        })}
      </nav>
      <div className="app-header-actions">
        <ThemeSwitcher />

        {user && (
          <div className="app-user">
            <div className="app-user-copy">
              <div style={{ fontWeight: 600, color: 'var(--ink-strong)' }}>{user.display_name || user.username}</div>
              <div style={{ fontSize: '11px', color: 'var(--ink)' }}>{user.role}</div>
            </div>
            <button
              className="btn-secondary app-logout"
              onClick={onLogout}
              title="Sign out"
              aria-label="Sign out"
            >
              <LogOut size={16} />
            </button>
          </div>
        )}
      </div>
    </header>
  );
};
