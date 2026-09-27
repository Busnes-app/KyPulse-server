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
