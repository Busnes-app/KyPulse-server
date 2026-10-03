import { test, expect } from '@playwright/test';
import { createServer } from 'node:http';
import { networkInterfaces } from 'node:os';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

let yard;
let yardURL;
test.beforeAll(async () => {
  const ip = Object.values(networkInterfaces()).flat().find(i => i?.family === 'IPv4' && !i.internal && /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(i.address))?.address;
  test.skip(!ip, 'no private IPv4 interface for fake KyYard');
  yard = createServer((req, res) => {
    res.setHeader('Content-Type', 'application/json');
    if (req.url === '/api/service-tokens/claim') { res.end(JSON.stringify({ token: 'browser-yard-token', organization: { id: 'browser-org', name: 'Browser KyYard' } })); return; }
    if (req.headers.authorization !== 'Bearer browser-yard-token') { res.writeHead(401); res.end(); return; }
    if (req.url.endsWith('/endpoints?limit=200')) { res.end(JSON.stringify([{ id: 'browser-ep', name: 'Browser endpoint', runtime: 'docker', state: 'active' }])); return; }
    if (req.url.endsWith('/inventory')) { res.end(JSON.stringify({ endpoint_id: 'browser-ep', state: 'active', observed_at: new Date().toISOString(), received_at: new Date().toISOString(), snapshot: { containers: [{ id: 'browser-ct', name: 'browser-app', state: 'running', status: 'Up', image: 'test' }, { id: 'browser-worker', name: 'browser-app-worker', state: 'running', status: 'Up', image: 'test' }] } })); return; }
    if (req.url.endsWith('/samples')) { res.end(JSON.stringify([{ container_id: 'browser-ct', observed_at: new Date().toISOString(), memory_bytes: 1048576, memory_limit: 0, restart_count: 5 }])); return; }
    if (req.url.includes('/logs?')) { res.setHeader('Content-Type', 'text/plain'); res.end(`${new Date().toISOString()} browser collection line\n`); return; }
    if (req.url.includes('/audit?')) { res.end('[]'); return; } // deployed old API remains visibly unsupported
    res.writeHead(404); res.end();
  });
  await new Promise(resolve => yard.listen(0, '0.0.0.0', resolve));
  yardURL = `http://${ip}:${yard.address().port}`;
});
test.afterAll(async () => { if (yard) await new Promise(resolve => yard.close(resolve)); });

async function signIn(page, username = 'admin') {
  await page.goto('/');
  await page.getByPlaceholder('admin', { exact: true }).fill(username);
  await page.locator('input[type=password]').fill(username === 'admin' ? 'BrowserUpdated456!' : 'BrowserViewer123!');
  await page.getByRole('button', { name: 'Sign In', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
}
async function csrf(page) {
  return { 'X-CSRF-Token': (await page.context().cookies()).find(c => c.name === 'ky_csrf').value };
}

test('admin logs, activity, pairing, filters, retry and viewer boundaries', async ({ page, context }, testInfo) => {
  test.setTimeout(150000);
  await signIn(page);
  const app = `logs-${testInfo.project.name}`;
  const sourceName = `sender-${testInfo.project.name}`;
  const targetResponse = await page.request.post('/api/targets', { headers: await csrf(page), data: { name: app, url: 'https://example.com/healthz', interval_sec: 30, enabled: false, container: 'browser-ep/browser-app' } });
  expect(targetResponse.status()).toBe(201);
  const { target } = await targetResponse.json();
  try {
    const pairing = await page.request.post('/api/kyyard/pair', { headers: await csrf(page), data: { url: yardURL, pairing_code: '123456' } });
    expect(pairing.status()).toBe(200);
    await expect.poll(async () => (await (await page.request.get('/api/kyyard/containers')).json()).length).toBe(2);
    // Leave Status so its suggestions reload after this API-side pairing.
    await page.goto('/#/settings');
    await expect(page.getByRole('form', { name: 'Alert webhook' })).toBeVisible();
    await page.goto('/#/status');
    await page.getByRole('button', { name: 'Add app' }).click();
    const container = page.getByLabel('KyYard container');
    await expect(page.locator('#kyyard-suggestions option')).toHaveCount(2);
    await container.pressSequentially('browser-ep/browser-app');
    await expect(page.getByLabel('Name', { exact: true })).toHaveValue('');
    await container.pressSequentially('-worker');
    await expect(page.getByLabel('Name', { exact: true })).toHaveValue('');
    await container.press('Tab');
    await expect(page.getByLabel('Name', { exact: true })).toHaveValue('browser-app-worker');
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.goto(`/#/apps/${target.id}`);
    await expect(page.getByText('0 since monitoring began (under 1 min)')).toBeVisible();
    await expect(page.getByText(/5 total · sampled/)).toBeVisible();
    await page.goto('/#/logs');
    await expect(page.getByRole('heading', { name: 'Logs', exact: true })).toBeVisible();
    await expect(page.getByRole('region', { name: 'KyYard collection' })).toContainText('Audit feed: no successful collection yet · stale (audit_cursor_unsupported)', { timeout: 80000 });
    await expect(page.getByRole('region', { name: 'KyYard collection' })).toContainText(/Container logs: last success.*fresh/);
    await expect(page.getByRole('region', { name: 'KyYard collection' })).toContainText(/Inventory: last success.*fresh/);
    await page.getByLabel('Bind log source to watched app').selectOption(target.id);
    await page.getByLabel('Log source name', { exact: true }).fill(sourceName);
    await expect(page.getByLabel('Sender HTTPS origin')).toHaveValue('');
    await page.getByLabel('Sender HTTPS origin').fill('https://pulse.example.com:8443/');
    await page.getByRole('button', { name: 'Add source', exact: true }).click();
    const command = page.locator('.log-command');
    await expect(command).toContainText(`--name ${sourceName}`);
    const commandText = await command.textContent();
    const commandOrigin = commandText.match(/--url '([^']+)' --code/)[1];
    expect(commandOrigin).toBe('https://pulse.example.com:8443');
    execFileSync('go', ['test', './internal/sender', '-run', '^TestPairScreenCommandOrigin$', '-count=1'], {
      cwd: fileURLToPath(new URL('../../', import.meta.url)),
      env: { ...process.env, KYPULSE_TEST_SCREEN_PAIR_ORIGIN: commandOrigin },
      timeout: 60000,
    });
    const code = commandText.match(/--code (\d{6})/)[1];
    const claim = await page.request.post('/api/log-sources/claim', { headers: await csrf(page), data: { pairing_code: code, name: sourceName } });
    expect(claim.ok()).toBe(true);
    const claimed = await claim.json();
    expect(await page.locator('body').textContent()).not.toContain(claimed.token);
    const now = Date.now();
    const xss = '<img src=x onerror=alert(1)>';
    const lines = Array.from({ length: 105 }, (_, i) => ({ line: JSON.stringify({ app, level: i === 104 ? 'error' : 'info', timestamp: new Date(now - 300000 + i * 1000).toISOString(), message: i === 104 ? `${xss}\u001b[31m literal %_\u0007` : `sample-${i}` }) }));
    for (let i = 0; i < 5; i++) lines.push({ line: JSON.stringify({ app, timestamp: new Date(now - 300000 + i * 75000).toISOString(), seq: i + 1, hash: 'fixture', fields: [], action: 'auth.login', user_id: 'alice', outcome: 'failure', ip_address: '192.0.2.1', resource: 'session' }) });
    const ingest = await page.request.post('/api/ingest/logs', { headers: { Authorization: `Bearer ${claimed.token}`, 'Content-Type': 'application/x-ndjson' }, data: lines.map(JSON.stringify).join('\n') + '\n' });
    expect(ingest.status()).toBe(204);
    await page.getByRole('button', { name: 'Hide code' }).click();
    await page.getByRole('button', { name: 'Refresh sources' }).click();
    await expect(page.getByRole('button', { name: `Revoke ${sourceName}`, exact: true })).toBeVisible();
    await page.getByLabel('Log app', { exact: true }).fill(app);
    await page.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(page.locator('.log-lines').first().locator(':scope > li')).toHaveCount(100);
    await page.getByRole('button', { name: 'Load more', exact: true }).click();
    await expect(page.locator('.log-lines').first().locator(':scope > li')).toHaveCount(110);
    await page.getByLabel('Log level', { exact: true }).fill('error');
    await page.getByLabel('Literal text').fill('%_');
    await page.getByLabel('From (UTC)').fill(new Date(now - 3600000).toISOString().slice(0, 16));
    await page.getByLabel('To (UTC)').fill(new Date(now + 3600000).toISOString().slice(0, 16));
    await page.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(page.locator('.log-lines').first().locator(':scope > li')).toHaveCount(1);
    await expect(page.getByText(`${xss} literal %_`, { exact: true })).toBeVisible();
    expect(await page.locator('.app-main img').count()).toBe(0);
    expect(await page.locator('.log-lines').first().textContent()).not.toMatch(/[\u001b\u0007]/);
    await page.getByText('Raw line', { exact: true }).click();
    await expect(page.locator('details[open] pre')).toContainText(xss);
    await context.setOffline(true);
    await page.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(page.getByRole('button', { name: 'Retry', exact: true })).toBeVisible();
    await expect(page.getByText('No logs found.', { exact: true })).toHaveCount(0);
    await context.setOffline(false);
    await page.getByRole('button', { name: 'Retry', exact: true }).click();
    await expect(page.getByText(`${xss} literal %_`, { exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath('logs.png'), fullPage: true });
    await page.getByRole('button', { name: 'Clear', exact: true }).click();
    await expect(page.getByLabel('Literal text')).toHaveValue('');

    await page.goto('/#/activity');
    await expect(page.getByRole('region', { name: 'KyYard collection' })).toContainText('audit_cursor_unsupported');
    await page.getByLabel('Activity app').fill(app);
    await page.getByLabel('Actor', { exact: true }).fill('alice');
    await page.getByLabel('Outcome', { exact: true }).fill('failure');
    await page.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(page.getByRole('region', { name: 'Failed sign-in bursts' })).toContainText('5 failed sign-ins');
    await expect(page.locator('.log-lines > li')).toHaveCount(5);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath('activity.png'), fullPage: true });
    await page.getByLabel('Actor', { exact: true }).fill('nobody');
    await page.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(page.getByText(/No audit events match/)).toBeVisible();
    await expect(page.getByText(/no audit events: not on shared logging/)).toHaveCount(0);
    await page.getByLabel('Activity app').fill('never-logged');
    await page.getByRole('button', { name: 'Apply', exact: true }).click();
    await expect(page.getByText(/no audit events: not on shared logging/)).toBeVisible();
    await expect(page.getByText(/Within the retained 7-day window/)).toBeVisible();
    await page.goto('/#/settings');
    await expect(page.getByRole('region', { name: 'KyYard collection' })).toContainText('audit_cursor_unsupported');
    await page.goto(`/#/apps/${target.id}`);
    await expect(page.getByRole('region', { name: 'Recent logs' }).locator('li')).toHaveCount(20);
    await expect(page.getByRole('region', { name: 'Recent logs' }).getByText(`${xss} literal %_`, { exact: true })).toBeVisible();
    expect(await page.getByRole('region', { name: 'Recent logs' }).locator('img').count()).toBe(0);

    await page.goto('/#/logs');
    page.once('dialog', dialog => dialog.dismiss());
    await page.getByRole('button', { name: `Revoke ${sourceName}`, exact: true }).click();
    await expect(page.getByRole('button', { name: `Revoke ${sourceName}`, exact: true })).toBeVisible();
    page.once('dialog', dialog => dialog.accept());
    await page.getByRole('button', { name: `Revoke ${sourceName}`, exact: true }).click();
    await expect(page.getByRole('button', { name: `Revoke ${sourceName}`, exact: true })).toHaveCount(0);
    expect((await page.request.post('/api/ingest/logs', { headers: { Authorization: `Bearer ${claimed.token}`, 'Content-Type': 'application/x-ndjson' }, data: '{"line":"refused"}\n' })).status()).toBe(401);
    await page.getByRole('button', { name: 'Sign out', exact: true }).click();
    await signIn(page, 'viewer');
    for (const name of ['Logs', 'Activity']) {
      await page.goto(`/#/${name.toLowerCase()}`);
      await expect(page.getByRole('heading', { name, exact: true })).toHaveCount(0);
      expect((await page.request.get(`/api/${name.toLowerCase()}`)).status()).toBe(403);
    }
    const logRequests = [];
    page.on('request', req => { if (new URL(req.url()).pathname === '/api/logs') logRequests.push(req.url()); });
    await page.goto(`/#/apps/${target.id}`);
    await expect(page.getByRole('heading', { name: app, exact: true })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Recent logs' })).toHaveCount(0);
    expect(logRequests).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  } finally {
    await context.setOffline(false);
    // Restore an admin session for cleanup even if a viewer assertion failed.
    const login = await page.request.post('/api/auth/login', { data: { username: 'admin', password: 'BrowserUpdated456!' } });
    expect(login.ok()).toBe(true);
    await page.request.delete('/api/kyyard', { headers: await csrf(page) });
    await page.request.delete(`/api/targets/${target.id}`, { headers: await csrf(page) });
  }
});
