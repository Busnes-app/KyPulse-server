import { test, expect } from '@playwright/test';

// Phones scroll the Primary navigation sideways. Loading a deep link (here a reload of
// #/appearance) must bring the current row into view, without moving the page or focus.
test('phone navigation reveals the current row on a deep link', async ({ page }, testInfo) => {
  test.skip(testInfo.project.use.viewport.width > 390, 'phone layout only');
  await page.goto('/');
  await page.getByPlaceholder('admin', { exact: true }).fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();
  const nav = page.getByRole('navigation', { name: 'Primary' });
  await expect(nav).toBeVisible();
  await page.goto('/#/appearance');
  await page.reload();
  await expect(nav.locator('[aria-current="page"]')).toHaveText('Appearance');
  await expect.poll(() => nav.evaluate(el => {
    const box = el.getBoundingClientRect();
    const item = el.querySelector('[aria-current="page"]').getBoundingClientRect();
    return item.left >= box.left - 1 && item.right <= box.right + 1;
  })).toBe(true);
  const after = await page.evaluate(() => ({ y: scrollY, navFocused: !!document.activeElement?.closest('nav') }));
  expect(after.y).toBe(0);
  expect(after.navFocused).toBe(false);
  // An in-page link while scrolled down: the row comes into view, the page stays put. The
  // test-only height (CSSOM, allowed by the CSP) stops the page from clamping its scroll.
  await page.goto('/#/status');
  await expect(nav.locator('[aria-current="page"]')).toHaveText('Status');
  await page.locator('main').evaluate(el => el.style.setProperty('min-height', '400vh'));
  await page.evaluate(() => scrollTo(0, 600));
  await page.evaluate(() => { location.hash = '#/appearance'; });
  await expect(nav.locator('[aria-current="page"]')).toHaveText('Appearance');
  await expect.poll(() => nav.evaluate(el => {
    const box = el.getBoundingClientRect();
    const item = el.querySelector('[aria-current="page"]').getBoundingClientRect();
    return item.left >= box.left - 1 && item.right <= box.right + 1;
  })).toBe(true);
  expect(await page.evaluate(() => scrollY)).toBe(600);
});
