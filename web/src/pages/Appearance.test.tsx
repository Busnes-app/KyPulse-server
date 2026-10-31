import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { Appearance } from './Appearance';
import { ThemeSwitcher } from '../components/ThemeSwitcher';

const root = document.documentElement;
const saved = (id: string | null) => {
  if (id === null) localStorage.removeItem('ky_theme');
  else localStorage.setItem('ky_theme', id);
  window.dispatchEvent(new StorageEvent('storage', { key: 'ky_theme' }));
};
const mode = (pair: string, label: string) =>
  within(within(screen.getByRole('group', { name: 'Theme' })).getByRole('group', { name: pair })).getByRole('radio', { name: label }) as HTMLInputElement;

describe('Appearance', () => {
  afterEach(() => {
    cleanup();
    saved(null);
  });

  it('offers six pairs with Match system, Light and Dark in one radio group, Busnes Match system by default', () => {
    render(<Appearance />);
    expect(screen.getByRole('heading', { level: 1, name: 'Appearance' })).toBeTruthy();
    expect(screen.getByRole('group', { name: 'Theme' }).querySelectorAll('[role="group"]')).toHaveLength(6);
    const radios = screen.getAllByRole('radio') as HTMLInputElement[];
    expect(radios).toHaveLength(18);
    expect(new Set(radios.map((r) => r.name)).size).toBe(1);
    expect(mode('Busnes', 'Match system').checked).toBe(true);
    expect(screen.getByText(/Match system follows your device’s light or dark setting, Light or Dark fixes the mode/)).toBeTruthy();
  });

  it('marks only the selected mode with a check', () => {
    saved('busnes-dark');
    render(<Appearance />);
    expect(mode('Busnes', 'Dark').closest('label')!.querySelector('svg')).not.toBeNull();
    expect(mode('Busnes', 'Light').closest('label')!.querySelector('svg')).toBeNull();
    expect(root.dataset.scheme).toBe('dark');
  });

  it('keeps earlier saved IDs: paper is Nord Light, base patina is Ky Dark, cyber resets to Busnes', () => {
    saved('paper');
    const { unmount } = render(<Appearance />);
    expect(mode('Nord', 'Light').checked).toBe(true);
    unmount();
    saved('patina');
    render(<Appearance />);
    expect(mode('Ky', 'Dark').checked).toBe(true);
    cleanup();
    saved('cyber');
    render(<Appearance />);
    expect(mode('Busnes', 'Match system').checked).toBe(true);
  });

  it('saves a fixed half and a pair following the OS, then clears colours for Busnes; the switcher agrees', () => {
    render(<><ThemeSwitcher /><Appearance /></>);
    fireEvent.click(mode('Ocean', 'Dark'));
    expect(screen.getByLabelText('Color theme')).toHaveProperty('value', 'ocean');
    expect(localStorage.getItem('ky_theme')).toBe('ocean');
    expect(root.dataset.theme).toBe('ocean');
    expect(root.style.getPropertyValue('--bg')).toBe('#0f1b24');
    expect(root.style.colorScheme).toBe('dark');
    fireEvent.click(mode('Forest', 'Match system'));
    expect(localStorage.getItem('ky_theme')).toBe('forest-pair');
    fireEvent.change(screen.getByLabelText('Color theme'), { target: { value: 'busnes-light' } });
    expect(mode('Busnes', 'Light').checked).toBe(true);
    expect(localStorage.getItem('ky_theme')).toBe('busnes-light');
    expect(root.dataset.theme).toBe('busnes-light');
    expect(root.style.getPropertyValue('--bg')).toBe('');
    expect(root.style.colorScheme).toBe('light');
  });
});
