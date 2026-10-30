import { readFileSync } from 'node:fs';
import { test, expect } from '@playwright/test';

const version = readFileSync(new URL('../../internal/appversion/VERSION', import.meta.url), 'utf8').trim();

test('the copyright line opens the About dialog with the server version and licence', async ({ page }, testInfo) => {
  await page.goto('/');
  await page.getByPlaceholder('admin', { exact: true }).fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
  const link = page.getByRole('button', { name: /© \d{4} · MIT License/ });
  // Phones drop the line, as KyPost does.
  if (testInfo.project.use.viewport.width <= 720) {
    await expect(link).toBeHidden();
    return;
  }
  await link.click();
  const dialog = page.getByRole('dialog', { name: 'About kyPulse' });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('.license-version-badge')).toHaveText(`v${version}`);
  await expect(dialog.getByLabel('MIT License')).toHaveValue(/^MIT License\n\nCopyright \(c\) 2026 Busnes\.app/);
  await dialog.getByRole('button', { name: 'Close' }).click();
  await expect(dialog).toBeHidden();
});
