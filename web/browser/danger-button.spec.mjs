import { test, expect } from '@playwright/test';
import { THEME_CATALOG } from '../src/ky-ui/themes.js';

// The filled danger button's label must read on its fill for every suite palette, as the
// screen shows it: pixels from a screenshot, so hover opacity and focus are included. The
// control is the real Delete on a disabled synthetic app; it is never clicked.
const SUITE = THEME_CATALOG.filter(t => !t.id.startsWith('busnes-')).map(t => t.id);

// Fill = the most common pixel; label = the pixel that contrasts most with it. The clip stays
// inside the rounded corners, so the page behind the button is never sampled.
const shot = async (page, button) => {
  await button.scrollIntoViewIfNeeded();
  const box = await button.boundingBox();
  return page.screenshot({ animations: 'disabled', clip: { x: box.x + 8, y: box.y + 4, width: box.width - 16, height: box.height - 8 } });
};
const contrast = (decoder, png) => decoder.evaluate(async b64 => {
  const img = new Image();
  img.src = `data:image/png;base64,${b64}`;
  await img.decode();
  const canvas = Object.assign(document.createElement('canvas'), { width: img.width, height: img.height });
  const ctx = canvas.getContext('2d');
  ctx.drawImage(img, 0, 0);
  const data = ctx.getImageData(0, 0, img.width, img.height).data;
  const lum = (r, g, b) => [r, g, b].map(v => v / 255).map(v => v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4).reduce((s, v, i) => s + v * [0.2126, 0.7152, 0.0722][i], 0);
  const counts = new Map();
  for (let i = 0; i < data.length; i += 4) { const key = data[i] << 16 | data[i + 1] << 8 | data[i + 2]; counts.set(key, (counts.get(key) ?? 0) + 1); }
  const fill = [...counts].sort((a, b) => b[1] - a[1])[0][0];
  const fl = lum(fill >> 16, fill >> 8 & 255, fill & 255);
  let best = 1;
  for (const key of counts.keys()) {
    const l = lum(key >> 16, key >> 8 & 255, key & 255);
    best = Math.max(best, (Math.max(l, fl) + 0.05) / (Math.min(l, fl) + 0.05));
  }
  return +best.toFixed(2);
}, png.toString('base64'));

test('the filled danger button label reads on its fill in every suite palette', async ({ page, context }, testInfo) => {
  test.skip(testInfo.project.use.viewport.width < 1280, 'one width per OS scheme is enough for colour');
  test.setTimeout(120000);
  await page.goto('/');
  await page.getByLabel('Username').fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
  const headers = { 'X-CSRF-Token': (await context.cookies()).find(c => c.name === 'ky_csrf').value };
  const created = await page.request.post('/api/targets', { headers, data: { name: `danger-${testInfo.project.name}`, url: 'https://example.com/healthz', interval_sec: 30, enabled: false } });
  expect(created.status()).toBe(201);
  const { target } = await created.json();
  const helper = await context.newPage(); // another tab saves the choice; the app applies it live
  const decoder = await context.newPage();
  try {
    await page.goto(`/#/apps/${target.id}`);
    const button = page.getByRole('button', { name: 'Delete' });
    await expect(button).toHaveClass(/btn-danger/);
    await helper.goto('/app-icon.png');
    const choose = async id => {
      await helper.evaluate(id => localStorage.setItem('ky_theme', id), id);
      await expect.poll(() => page.evaluate(() => document.documentElement.dataset.theme)).toBe(id);
    };
    const results = [];
    for (const id of SUITE) {
      await choose(id);
      await page.mouse.move(0, 0);
      const rest = await contrast(decoder, await shot(page, button));
      await button.hover();
      const hover = await contrast(decoder, await shot(page, button));
      await page.mouse.move(0, 0);
      await button.focus();
      const focus = await contrast(decoder, await shot(page, button));
      await button.blur();
      results.push({ id, rest, hover, focus });
    }
    console.log(testInfo.project.name, JSON.stringify(results));
    expect(results.filter(r => Math.min(r.rest, r.hover, r.focus) < 4.5)).toEqual([]);
    // Busnes keeps its own label colour.
    for (const id of ['busnes-light', 'busnes-dark']) {
      await choose(id);
      expect(await button.evaluate(el => {
        const probe = document.body.appendChild(document.createElement('i'));
        probe.style.color = getComputedStyle(document.documentElement).getPropertyValue('--button-text').trim() || '#ffffff';
        const want = getComputedStyle(probe).color;
        probe.remove();
        return getComputedStyle(el).color === want;
      }), id).toBe(true);
    }
  } finally {
    await page.request.delete(`/api/targets/${target.id}`, { headers });
    await helper.close();
    await decoder.close();
  }
});
