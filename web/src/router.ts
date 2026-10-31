import { useEffect, useState } from 'react';

export interface Route {
  path: string;
  parts: string[];
}

function parse(hash: string): Route {
  let path = hash.replace(/^#/, '');
  if (!path.startsWith('/')) path = '/' + path;
  if (path === '/') path = '/status';
  return { path, parts: path.split('/').filter(Boolean) };
}

export function currentRoute(): Route {
  return parse(window.location.hash);
}

export function navigate(path: string): void {
  window.location.hash = path.startsWith('/') ? path : '/' + path;
}

export function hrefFor(path: string): string {
  return '#' + path;
}

// redirect swaps the current history entry, so Back skips a moved or unknown route.
export function redirect(path: string): void {
  window.location.replace(hrefFor(path.startsWith('/') ? path : '/' + path));
}

export type SettingsSection = 'account' | 'appearance' | 'server';

// Settings tabs in order: the personal ones first, then the server's. KyPulse has no app
// passwords or devices, so there is no Phones & apps tab.
export const settingsSections: { id: SettingsSection; label: string }[] = [
  { id: 'account', label: 'Account' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'server', label: 'Server' },
];

// movedRoute maps a route that used to be its own page to its Settings tab (null: not moved).
// The Settings row itself opens the first tab.
export function movedRoute(path: string): string | null {
  if (path === '/appearance') return '/settings/appearance';
  if (path === '/settings') return '/settings/account';
  return null;
}

// useHashRoute re-renders on every hashchange; the hash is the only navigation state.
export function useHashRoute(): Route {
  const [route, setRoute] = useState<Route>(currentRoute);
  useEffect(() => {
    const onChange = () => setRoute(currentRoute());
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
  }, []);
  return route;
}
