import { test, expect } from '@playwright/test';

test('desktop to phone resizing keeps the current navigation item visible', async ({ page }, testInfo) => {
  test.skip(testInfo.project.use.viewport.width < 1280, 'starts in the desktop layout');
  await page.goto('/');
  await page.getByLabel('Username').fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  const nav = page.getByRole('navigation', { name: 'Primary' });
  await expect(nav).toBeVisible();
  await page.goto('/#/backup');
  await expect(nav.locator('[aria-current="page"]')).toHaveText('Backup');
  await page.evaluate(() => document.fonts.ready);
  // A control that exists at both widths (the sidebar Sign out hides on phones, where the header icon replaces it).
  const focused = page.getByRole('main').getByRole('button').first();
  await focused.focus();
  await page.locator('main').evaluate(el => el.style.setProperty('min-height', '400vh'));
  await page.evaluate(() => scrollTo(0, 600));
  await page.setViewportSize({ width: 390, height: 844 });
  await expect.poll(() => nav.evaluate(el => {
    const box = el.getBoundingClientRect();
    const item = el.querySelector('[aria-current="page"]').getBoundingClientRect();
    return item.left >= box.left - 1 && item.right <= box.right + 1;
  })).toBe(true);
  await expect(focused).toBeFocused();
  expect(await page.evaluate(() => scrollY)).toBe(600);
});

test('phone navigation keeps the current row visible after the font loads', async ({ page, browser }, testInfo) => {
  test.skip(testInfo.project.use.viewport.width > 390, 'phone layout only');
  await page.goto('/');
  await page.getByLabel('Username').fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
  const context = await browser.newContext({
    storageState: await page.context().storageState(),
    viewport: testInfo.project.use.viewport,
    colorScheme: testInfo.project.use.colorScheme,
  });
  let releaseFont;
  const fontGate = new Promise(resolve => { releaseFont = resolve; });
  try {
    const coldPage = await context.newPage();
    await coldPage.route('**/fonts/Space_Grotesk/*.ttf', async route => { await fontGate; await route.continue(); });
    await coldPage.goto('http://127.0.0.1:5391/#/logs', { waitUntil: 'domcontentloaded' });
    const nav = coldPage.getByRole('navigation', { name: 'Primary' });
    await expect(nav.locator('[aria-current="page"]')).toHaveText('Logs');
    await expect.poll(() => coldPage.evaluate(() => document.fonts.status)).toBe('loading');
    releaseFont();
    await coldPage.evaluate(() => document.fonts.ready);
    await expect.poll(() => nav.evaluate(el => {
      const box = el.getBoundingClientRect();
      const item = el.querySelector('[aria-current="page"]').getBoundingClientRect();
      return item.left >= box.left - 1 && item.right <= box.right + 1;
    })).toBe(true);
    expect(await coldPage.evaluate(() => scrollY)).toBe(0);
  } finally {
    releaseFont();
    await context.close();
  }
});

// Phones scroll the Primary navigation sideways. Loading a deep link (here a reload of
// #/settings/appearance, the last row) must bring the current row into view, without moving the page or focus.
test('phone navigation reveals the current row on a deep link', async ({ page }, testInfo) => {
  test.skip(testInfo.project.use.viewport.width > 390, 'phone layout only');
  await page.goto('/');
  await page.getByLabel('Username').fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  const nav = page.getByRole('navigation', { name: 'Primary' });
  await expect(nav).toBeVisible();
  await page.goto('/#/settings/appearance');
  await page.reload();
  await expect(nav.locator('[aria-current="page"]')).toHaveText('Settings');
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
  await page.evaluate(() => { location.hash = '#/settings/appearance'; });
  await expect(nav.locator('[aria-current="page"]')).toHaveText('Settings');
  await expect.poll(() => nav.evaluate(el => {
    const box = el.getBoundingClientRect();
    const item = el.querySelector('[aria-current="page"]').getBoundingClientRect();
    return item.left >= box.left - 1 && item.right <= box.right + 1;
  })).toBe(true);
  expect(await page.evaluate(() => scrollY)).toBe(600);
});
