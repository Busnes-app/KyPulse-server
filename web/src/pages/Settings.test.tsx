import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/react';
import { Settings } from './Settings';
import { AppHeader } from '../components/AppHeader';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const admin = { id: 'u1', role: 'admin', username: 'admin', display_name: 'Administrator', sso_provider: 'local', totp_enabled: false };

describe('Settings', () => {
  it('shows one head, the section tabs in order, and marks the open tab', () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 200 })));
    render(<Settings section="appearance" settings={{ db_driver: 'sqlite' }} user={admin} onChanged={() => {}} />);
    expect(screen.getAllByRole('heading', { level: 1 }).map((h) => h.textContent)).toEqual(['Settings']);
    const tabs = within(screen.getByRole('navigation', { name: 'Settings sections' })).getAllByRole('link');
    expect(tabs.map((t) => [t.textContent, t.getAttribute('href')])).toEqual([
      ['Account', '#/settings/account'], ['Appearance', '#/settings/appearance'], ['Server', '#/settings/server'],
    ]);
    expect(tabs.filter((t) => t.getAttribute('aria-current') === 'page').map((t) => t.textContent)).toEqual(['Appearance']);
    expect(screen.getByRole('group', { name: 'Theme' })).toBeTruthy();
  });

  it('Account names the user once and offers two-factor sign-in to a local account', () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 200 })));
    render(<Settings section="account" settings={null} user={admin} onChanged={() => {}} />);
    const facts = screen.getByRole('region', { name: 'Signed in as' });
    expect(within(facts).queryByText('Name')).toBeNull();
    expect(within(facts).getByText('admin')).toBeTruthy();
    expect(within(facts).getByText('KyPulse password')).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Two-factor authentication' })).toBeTruthy();
  });

  it('Server keeps storage for everyone and the webhook and KyYard for administrators', () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 200 })));
    render(<Settings section="server" settings={{ db_driver: 'postgres' }} user={{ ...admin, role: 'viewer' }} onChanged={() => {}} />);
    expect(screen.getByText('postgres')).toBeTruthy();
    expect(screen.queryByRole('form', { name: 'Alert webhook' })).toBeNull();
    expect(screen.queryByRole('heading', { name: 'KyYard' })).toBeNull();
  });
});

describe('AppHeader', () => {
  it('keeps Settings as a row, drops Appearance, and signs out with an icon button', () => {
    render(<AppHeader appName="KyPulse" activePath="/settings/server" user={{ role: 'admin', username: 'admin' }} onLogout={() => {}} />);
    const nav = screen.getByRole('navigation', { name: 'Primary' });
    expect(within(nav).queryByRole('link', { name: 'Appearance' })).toBeNull();
    expect(within(nav).getByRole('link', { name: 'Settings' }).getAttribute('aria-current')).toBe('page');
    const signOut = screen.getAllByRole('button', { name: 'Sign out' });
    expect(signOut).toHaveLength(2);
    for (const b of signOut) {
      expect(b.className).toBe('ky-icon-button');
      expect(b.getAttribute('title')).toBe('Sign out');
      expect(b.textContent).toBe('');
    }
    expect(screen.queryByRole('link', { name: 'Settings', hidden: false })?.closest('.ky-user-actions')).toBeNull();
  });
});
