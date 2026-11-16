// Before/after captures of Settings › Server for "Email through KyPost". Usage, from web/:
//   node ../docs/ui-kypost-mail-2026-11-15/capture.mjs <server-binary> <before|after|after-kyidentity>
// Owns a disposable data directory and port 5392 (hold /tmp/ky-integ-test.lock).
// after-kyidentity configures KySignOn against https://id.example.invalid, a name that never
// resolves, so the channel shows its unpaired state and nothing leaves the host.
import { createRequire } from 'node:module';
import { mkdtemp, rm, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawn, execFileSync } from 'node:child_process';
import { DatabaseSync } from 'node:sqlite';
import { fileURLToPath } from 'node:url';

const { chromium } = createRequire(fileURLToPath(new URL('../../web/package.json', import.meta.url)))('@playwright/test');
const [binary, phase] = process.argv.slice(2);
const out = fileURLToPath(new URL(`./screenshots/${phase}/`, import.meta.url));
await mkdir(out, { recursive: true });
const dir = await mkdtemp(join(tmpdir(), 'ky-capture-'));
const env = {
  PATH: process.env.PATH, KYPULSE_APP_URL: 'http://127.0.0.1:5392', KYPULSE_HOST: '127.0.0.1', KYPULSE_PORT: '5392',
  KYPULSE_DB_DRIVER: 'sqlite', KYPULSE_DATA_DIR: join(dir, 'data'), KYPULSE_BACKUP_DIR: join(dir, 'backups'),
  KYPULSE_CAPTCHA_PROVIDER: 'none', PUSH_RELAY_URL: '', APNS_RELAY_URL: '',
};
if (phase === 'after-kyidentity') Object.assign(env, {
  KYPULSE_KYSIGNON_ISSUER: 'https://id.example.invalid', KYPULSE_KYSIGNON_CLIENT_ID: 'kypulse',
  KYPULSE_KYSIGNON_SECRET: 'capture-client-secret-not-real-0000000', KYPULSE_KYSIGNON_HMAC_SECRET: 'capture-hmac-secret-not-real-000000000000',
});
const opts = { cwd: dir, env, stdio: 'ignore' };
execFileSync(binary, ['init-admin', '-username', 'admin', '-password', 'CaptureAdmin123!'], opts);
const db = new DatabaseSync(join(dir, 'data', 'kypulse.db'));
db.prepare('UPDATE users SET must_change_password=0').run();
const now = new Date().toISOString();
db.prepare(`INSERT INTO users (id,username,email,display_name,password_hash,role,status,sso_provider,sso_subject,created_at,updated_at)
  VALUES ('usr_ops','ops','ops@example.com','Ops Rota','','viewer','active','kysignon','sub-ops',?,?)`).run(now, now);
db.close();
const server = spawn(binary, [], opts);
try {
  for (let i = 0; i < 50; i++) {
    try { if ((await fetch('http://127.0.0.1:5392/healthz')).ok) break; } catch { /* starting */ }
    await new Promise((r) => setTimeout(r, 200));
  }
  const browser = await chromium.launch();
  for (const scheme of ['light', 'dark']) {
    for (const [w, label] of [[1280, 'desktop'], [390, 'phone']]) {
      const page = await (await browser.newContext({ colorScheme: scheme, viewport: { width: w, height: 900 } })).newPage();
      await page.goto('http://127.0.0.1:5392/');
      await page.getByLabel('Username').fill('admin');
      await page.locator('input[type=password]').fill('CaptureAdmin123!');
      await page.getByRole('button', { name: 'Sign in', exact: true }).click();
      await page.getByRole('navigation', { name: 'Primary' }).waitFor();
      await page.goto('http://127.0.0.1:5392/#/settings/server');
      await page.getByRole('form', { name: 'Alert webhook' }).waitFor();
      await page.waitForTimeout(800);
      await page.screenshot({ path: join(out, `settings-server-${scheme}-${label}.png`), fullPage: true });
      await page.context().close();
    }
  }
  await browser.close();
} finally {
  server.kill('SIGTERM');
  await new Promise((r) => server.on('exit', r));
  await rm(dir, { recursive: true, force: true });
}
