import React, { useEffect, useRef } from 'react';
import { LogOut, Settings as SettingsIcon, Activity, Bell, Archive, Palette, History, HardDrive } from 'lucide-react';
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
  { path: '/activity', label: 'Activity', icon: History, admin: true },
  { path: '/logs', label: 'Logs', icon: Archive, admin: true },
  { path: '/backup', label: 'Backup', icon: HardDrive, admin: true },
  { path: '/settings', label: 'Settings & DB', icon: SettingsIcon, admin: false },
  { path: '/appearance', label: 'Appearance', icon: Palette, admin: false },
];

export const AppHeader: React.FC<AppHeaderProps> = ({ appName, activePath, user, onLogout }) => {
  const items = navItems.filter((item) => !item.admin || user?.role === 'admin');
  const navRef = useRef<HTMLElement>(null);
  // Phones scroll the navigation sideways: keep the current page's row in view, also on deep links.
  useEffect(() => {
    let cancelled = false;
    const revealCurrent = () => {
      if (!cancelled) navRef.current?.querySelector('[aria-current="page"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
    };
    revealCurrent();
    window.addEventListener('resize', revealCurrent);
    // Font loading can widen the row after the first scroll.
    void document.fonts?.ready.then(revealCurrent);
    return () => {
      cancelled = true;
      window.removeEventListener('resize', revealCurrent);
    };
  }, [activePath]);
  return (
    <header className="app-header">
      <div className="app-brand">
        <img src="/app-icon.png" width={28} height={28} alt="" />
        <span>{appName || 'kyPulse'}</span>
      </div>

      <nav ref={navRef} className="app-nav" aria-label="Primary">
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
