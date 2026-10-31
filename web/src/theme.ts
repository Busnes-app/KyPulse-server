import { useSyncExternalStore } from 'react';
import { readChoice, saveChoice, watchChoice } from './ky-ui/theme';
import { SYSTEM_THEME, THEME_PAIRS, applyThemeVariables, findPair, findTheme } from './ky-ui/themes';
// Busnes.app defaults; explicit choices are local to this browser under `ky_theme`. The saved
// value is `system` (Busnes following the OS), a pair ID (that pair following the OS) or a half
// ID (a fixed mode). Earlier IDs and aliases resolve to their half; removed ones to `system`.
const KEY = 'ky_theme';
export const THEME_CHOICES = THEME_PAIRS.map((pair) => ({
  pair,
  choices: [
    { mode: 'system', label: 'Match system', value: pair.id === 'busnes-pair' ? SYSTEM_THEME.id : pair.id },
    { mode: 'light', label: 'Light', value: pair.light },
    { mode: 'dark', label: 'Dark', value: pair.dark },
  ],
}));
export function storedTheme(): string {
  const saved = readChoice(KEY);
  return findPair(saved)?.id ?? findTheme(saved)?.id ?? SYSTEM_THEME.id;
}
let current = storedTheme();
// Every theme control on the page reads one store, so they never disagree.
const listeners = new Set<() => void>();
function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}
export const useTheme = () => useSyncExternalStore(subscribe, () => current);
const media = window.matchMedia?.('(prefers-color-scheme: dark)');
export function applyTheme(theme: string, persist = false) {
  const option = findTheme(theme);
  if (!option) return;
  current = findPair(theme)?.id ?? option.id;
  const root = document.documentElement;
  const resolved = option.id === 'system' ? (media?.matches ? 'busnes-dark' : 'busnes-light') : option.id;
  // Named palettes set their colours inline; Busnes clears them so the token aliases apply.
  const scheme = applyThemeVariables(resolved) ?? (resolved === 'busnes-dark' ? 'dark' : 'light');
  root.dataset.theme = resolved;
  root.dataset.scheme = scheme;
  root.style.colorScheme = scheme;
  if (persist) saveChoice(KEY, current);
  listeners.forEach((listener) => listener());
}
applyTheme(current);
watchChoice(KEY, () => applyTheme(storedTheme()));
