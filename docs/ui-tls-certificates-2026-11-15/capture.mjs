// Before/after captures for the TLS certificate screens. Usage, from web/:
//   node ../docs/ui-tls-certificates-2026-11-15/capture.mjs <server-binary> <before|after>
// Owns a disposable data directory and port 5392 (hold /tmp/ky-integ-test.lock). Targets are
// seeded straight into the owned SQLite database with a recent poll and an hour's interval, so
// nothing is dialled and the screens show fixed certificate states.
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
const opts = { cwd: dir, env, stdio: 'ignore' };
execFileSync(binary, ['init-admin', '-username', 'admin', '-password', 'CaptureAdmin123!'], opts);
const db = new DatabaseSync(join(dir, 'data', 'kypulse.db'));
db.prepare("UPDATE users SET must_change_password=0").run();
const now = new Date();
const iso = (d) => d.toISOString();
const days = (n) => new Date(now.getTime() + n * 86400e3);
const after = phase === 'after';
const targets = [
  { id: 'tgt_post', name: 'KyPost', url: 'https://post.lan/healthz', cert: { not_after: iso(days(5.5)), issuer: 'Busnes Internal CA', name_match: true }, level: 'critical', cause: 'cert_expiring' },
  { id: 'tgt_edge', name: 'Edge proxy', url: after ? 'tls://edge.lan:443' : 'https://edge.lan/', cert: { not_after: iso(days(18.5)), issuer: "Let's Encrypt R11", name_match: true }, level: 'warning', cause: 'cert_expiring' },
  { id: 'tgt_cal', name: 'KyCalendar', url: 'https://calendar.lan/healthz', cert: { not_after: iso(days(80.5)), issuer: 'Busnes Internal CA', name_match: true }, level: 'ok', cause: '' },
];
for (const t of targets) {
  const result = JSON.stringify(after ? { state: 'ok', cert: t.cert } : { state: 'ok' });
  const track = JSON.stringify({ state: 'ok', since: iso(days(-3)), ok_streak: 5 });
  db.prepare(`INSERT INTO targets (id,name,url,interval_sec,enabled,container,state,state_since,cause,until_fixed,track_json,last_result,last_polled_at,last_latency_ms,created_at,updated_at)
    VALUES (?,?,?,3600,1,'','ok',?, '',0,?,?,?,14,?,?)`).run(t.id, t.name, t.url, iso(days(-3)), track, result, iso(now), iso(days(-4)), iso(now));
  if (after) {
    db.prepare('UPDATE targets SET cert_track=? WHERE id=?').run(JSON.stringify({ state: t.level, since: iso(days(-1)), cause: t.cause }), t.id);
    if (t.level !== 'ok') db.prepare(`INSERT INTO target_events (target_id,at,from_state,to_state,cause,reminder,notified,notify_error) VALUES (?,?,?,?,?,0,1,'')`)
      .run(t.id, iso(days(-1)), 'cert_ok', 'cert_' + t.level, t.cause);
  }
}
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
      const shot = async (name) => { await page.waitForTimeout(400); await page.screenshot({ path: join(out, `${name}-${scheme}-${label}.png`), fullPage: true }); };
      await page.goto('http://127.0.0.1:5392/#/apps/tgt_post'); await page.getByRole('heading', { name: 'KyPost' }).waitFor(); await shot('app-detail');
      await page.goto('http://127.0.0.1:5392/#/alerts'); await page.getByRole('heading', { name: 'Alerts' }).waitFor(); await shot('alerts');
      await page.goto('http://127.0.0.1:5392/#/status'); await page.getByRole('button', { name: 'Add app' }).click();
      if (after) await page.getByLabel('TLS certificate only').check();
      await shot('add-dialog');
      if (after) { await page.goto('http://127.0.0.1:5392/#/certificates'); await page.getByRole('heading', { name: 'Certificates' }).waitFor(); await shot('certificates'); }
      await page.context().close();
    }
  }
  await browser.close();
} finally {
  server.kill('SIGTERM');
  await new Promise((r) => server.on('exit', r));
  await rm(dir, { recursive: true, force: true });
}
