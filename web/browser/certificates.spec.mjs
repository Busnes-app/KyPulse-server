import { test, expect } from '@playwright/test';
import { createServer } from 'node:tls';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// A TLS-only target on loopback, which needs the administrator's explicit permission. The
// certificate is self-signed and expires in five days, so it reads as untrusted (failed).
let server;
let port;
let dir;

test.beforeAll(async () => {
  try { execFileSync('openssl', ['version'], { stdio: 'ignore' }); } catch { test.skip(true, 'openssl is not installed'); }
  dir = mkdtempSync(join(tmpdir(), 'ky-cert-'));
  execFileSync('openssl', ['req', '-x509', '-newkey', 'ec', '-pkeyopt', 'ec_paramgen_curve:P-256', '-nodes', '-days', '5',
    '-subj', '/CN=Fixture CA', '-addext', 'subjectAltName=DNS:edge.test',
    '-keyout', join(dir, 'key.pem'), '-out', join(dir, 'cert.pem')], { stdio: 'ignore' });
  server = createServer({ key: readFileSync(join(dir, 'key.pem')), cert: readFileSync(join(dir, 'cert.pem')) }, (s) => s.end());
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  port = server.address().port;
});
test.afterAll(async () => {
  if (server) await new Promise((r) => server.close(r));
  if (dir) rmSync(dir, { recursive: true, force: true });
});

async function signIn(page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('admin');
  await page.locator('input[type=password]').fill('BrowserUpdated456!');
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Primary' })).toBeVisible();
}

// Remove the target even when an assertion failed, so later specs see a clean Status.
async function removeTarget(page, name) {
  const csrf = (await page.context().cookies()).find((c) => c.name === 'ky_csrf')?.value;
  const list = await (await page.request.get('/api/targets')).json();
  for (const t of list.targets ?? []) {
    if (t.name === name) await page.request.delete(`/api/targets/${t.id}`, { headers: { 'X-CSRF-Token': csrf } });
  }
}

test('a loopback TLS target needs the local permission and shows its certificate', async ({ page }, info) => {
  test.setTimeout(90000);
  const name = `Edge ${info.project.name}`;
  await signIn(page);
  try { await checkTarget(page, name); } finally { await removeTarget(page, name); }
});

async function checkTarget(page, name) {
  await page.getByRole('button', { name: 'Add app' }).click();
  const dialog = page.getByRole('dialog', { name: 'Add app' });
  await dialog.getByLabel('Name').fill(name);
  await dialog.getByLabel('TLS certificate only').check();
  await dialog.getByLabel('Host and port').fill(`127.0.0.1:${port}`);
  await dialog.getByLabel('Server name (optional)').fill('edge.test');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('loopback');

  await dialog.getByLabel('Allow a loopback or link-local address').check();
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toBeHidden();

  await page.getByRole('link', { name: 'Certificates' }).click();
  const row = page.getByRole('row').filter({ hasText: name });
  // The page refreshes every 15 s; the first poll lands within the server's 5 s tick.
  await expect(row).toContainText('Failed', { timeout: 30000 });
  await expect(row).toContainText('certificate not trusted');
  await expect(row).toContainText(/[45] days left/);
  await expect(row).toContainText('Fixture CA');

  await row.getByRole('link', { name }).click();
  const cert = page.locator('.dr-section').filter({ has: page.getByRole('heading', { name: 'Certificate' }) });
  await expect(cert).toContainText(/[45] days left/);
  await expect(cert).toContainText('certificate not trusted');
  await expect(page.getByText('TLS endpoint')).toBeVisible();

  page.once('dialog', (d) => d.accept());
  await page.getByRole('button', { name: 'Delete' }).click();
  await expect(page).toHaveURL(/#\/status$/);
}
