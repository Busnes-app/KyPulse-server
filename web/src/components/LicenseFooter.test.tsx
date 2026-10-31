import { readFileSync } from 'node:fs';
import { afterEach, beforeAll, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { LicenseFooter } from './LicenseFooter';

beforeAll(() => {
  // jsdom has no modal dialogs; open/close by attribute is enough here.
  HTMLDialogElement.prototype.showModal ??= function (this: HTMLDialogElement) { this.open = true; };
  HTMLDialogElement.prototype.close ??= function (this: HTMLDialogElement) { this.open = false; };
});

afterEach(cleanup);

describe('LicenseFooter', () => {
  it('opens the About dialog with version, copyright and the MIT text', () => {
    render(<LicenseFooter appName="KyPulse" version="0.5.0" />);
    const year = new Date().getFullYear();
    fireEvent.click(screen.getByRole('button', { name: `© ${year} · MIT License` }));
    const dialog = screen.getByRole('dialog', { name: 'About KyPulse' }) as HTMLDialogElement;
    expect(dialog.open).toBe(true);
    expect(dialog.textContent).toContain('v0.5.0');
    expect(dialog.textContent).toContain('Developed by Busnes.app');
    expect((screen.getByLabelText('MIT License') as HTMLTextAreaElement).value).toContain('Permission is hereby granted');
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(dialog.open).toBe(false);
  });

  it('shows no badge without a version', () => {
    render(<LicenseFooter appName="KyPulse" />);
    expect(document.querySelector('.ky-about-version')).toBeNull();
  });

  it('ships the repository licence unchanged', () => {
    // vitest runs from web/.
    expect(readFileSync('src/mit.txt', 'utf8')).toBe(readFileSync('../LICENSE.txt', 'utf8'));
  });
});
