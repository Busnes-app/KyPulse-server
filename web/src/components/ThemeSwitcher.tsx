import { THEME_CHOICES, applyTheme, useTheme } from '../theme';

/** The sign-in screen's compact picker; Appearance offers the same choices as cards. */
export function ThemeSwitcher() {
  const theme = useTheme();
  return <select className="theme-selector" aria-label="Color theme" value={theme} onChange={(event) => applyTheme(event.target.value, true)}>
    {THEME_CHOICES.map(({ pair, choices }) => (
      <optgroup key={pair.id} label={pair.name}>
        {choices.map(({ mode, label, value }) => <option key={mode} value={value}>{pair.name} · {label}</option>)}
      </optgroup>
    ))}
  </select>;
}
