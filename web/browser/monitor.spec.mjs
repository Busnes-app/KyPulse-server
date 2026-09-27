import { test, expect } from '@playwright/test';
import { createServer } from 'node:http';
import { networkInterfaces } from 'node:os';

// The egress guard refuses loopback, so the fake app listens on every interface and is
// reached through the host's LAN address, which the guard admits.
function lanAddress() {
  for (const list of Object.values(networkInterfaces())) {
    for (const i of list ?? []) if (i.family === 'IPv4' && !i.internal && /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(i.address)) return i.address;
  }
  return null;
}

const state = { health: 503, hook: 500, hooks: 0 };
let server;
let base;

test.beforeAll(async () => {
  const ip = lanAddress();
  test.skip(!ip, 'no private IPv4 interface for the fake app');
  server = createServer((req, res) => {
    if (req.url === '/healthz') {
      res.writeHead(state.health, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ schema: 'ky.health/1', service: 'fake', status: state.health === 200 ? 'ok' : 'down', checks: [{ name: 'database', status: state.health === 200 ? 'ok' : 'down', reason: 'refused' }] }));
      return;
    }
    if (req.url === '/hook') { state.hooks++; res.writeHead(state.hook); res.end(); return; }
    res.writeHead(404); res.end();
  });
  await new Promise((r) => server.listen(0, '0.0.0.0', r));
  base = `http://${ip}:${server.address().port}`;
});
test.afterAll(async () => { if (server) await new Promise((r) => server.close(r)); });

async function signIn(page) {
  await page.goto('/');
  await page.getByPlaceholder('admin', { exact: true }).fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
}

test('status, alerts, detail page and the alert bar in every state', async ({ page }, testInfo) => {
  test.setTimeout(240_000);
  await signIn(page);
  // Empty install.
  await expect(page.getByRole('status')).toContainText('No apps watched yet');
  // Add the fake app (interval at the 10 s floor so the state machine settles quickly).
  await page.getByRole('button', { name: 'Add app' }).click();
  await page.getByLabel('Name').fill('Fake app');
  await page.getByLabel('Health URL').fill(`${base}/healthz`);
  await page.getByLabel('Interval (seconds)').fill('10');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  const tile = page.getByRole('link', { name: /Fake app/ });
  await expect(tile).toBeVisible();
  // Three down polls → down: red bar with the problem line linking to the detail page.
  await expect(page.getByRole('alert')).toContainText('Fake app down', { timeout: 90_000 });
  await page.screenshot({ path: testInfo.outputPath('status-down.png'), fullPage: true });
  await page.getByRole('alert').getByRole('link', { name: /Fake app down/ }).click();
  await expect(page).toHaveURL(/#\/apps\/tgt_/);
  const banner = page.getByRole('region', { name: 'Current state' });
  await expect(banner).toContainText('down');
  await expect(banner).toContainText(`GET ${base}/healthz`);
  await expect(page.getByText('No checks reported yet.')).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Primary' }).getByRole('link', { name: 'Status', exact: true })).toHaveAttribute('aria-current', 'page');
  await page.screenshot({ path: testInfo.outputPath('detail-down.png'), fullPage: true });
  // Silence until fixed: still shown, marked silenced.
  await page.getByRole('button', { name: 'Silence until fixed' }).click();
  await expect(banner).toContainText('silenced until fixed');
  // Alerts tab lists the transition and the silence.
  await page.getByRole('link', { name: 'Alerts' }).first().click();
  await expect(page.getByRole('row').nth(1)).toContainText('pending → down');
  await expect(page.getByText('Silenced:')).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('alerts.png'), fullPage: true });
  // Webhook that fails → "Alerts not being delivered".
  await page.getByRole('link', { name: 'Settings & DB' }).click();
  await page.getByLabel('Preset').selectOption('generic');
  await page.getByLabel('URL', { exact: true }).fill(`${base}/hook`);
  await page.getByRole('button', { name: 'Save webhook' }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Webhook saved' })).toBeVisible();
  await page.getByRole('button', { name: 'Send test' }).click();
  await expect(page.getByRole('alert').filter({ hasText: /Test failed/ })).toBeVisible({ timeout: 60_000 });
  await expect(page.getByRole('alert').filter({ hasText: 'Alerts not being delivered' })).toBeVisible({ timeout: 30_000 });
  await page.screenshot({ path: testInfo.outputPath('settings-webhook-failing.png'), fullPage: true });
  expect(await page.evaluate(() => document.body.textContent)).not.toContain('token=');
  // Recovery: two ok polls → green bar, silence cleared.
  state.health = 200;
  await page.getByRole('link', { name: 'Status' }).first().click();
  await expect(page.getByRole('status').filter({ hasText: 'All 1 app healthy' })).toBeVisible({ timeout: 90_000 });
  await page.screenshot({ path: testInfo.outputPath('status-ok.png'), fullPage: true });
  await tile.click();
  await expect(banner).toContainText('ok');
  await expect(page.getByText('database')).toBeVisible();
  await expect(banner).not.toContainText('silenced');
  // Delete goes back to Status.
  page.once('dialog', (d) => d.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page).toHaveURL(/#\/status$/);
  await expect(page.getByRole('status')).toContainText('No apps watched yet');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
