// Review follow-up (R2) captures against a disposable kyPulse with synthetic data only.
// From the repository root, after `cd web && npm ci && npm run build` and
// `go build -o .browser/server ./cmd/server`:
//   node docs/ui-review-followups-2026-10-26/capture.mjs <stage nn> <before|after>
import { createHash } from 'node:crypto';
import { createServer } from 'node:http';
import { createRequire } from 'node:module';
import { connect } from 'node:net';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { networkInterfaces, release, tmpdir, type } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawn } from 'node:child_process';

const [stage, phase] = process.argv.slice(2);
if (!/^\d\d$/.test(stage ?? '') || !['before', 'after'].includes(phase)) throw new Error('usage: capture.mjs <nn> <before|after>');
const evidenceDir = 'docs/ui-review-followups-2026-10-26';
const root = fileURLToPath(new URL('../../', import.meta.url));
const here = fileURLToPath(new URL('.', import.meta.url));
const pw = createRequire(join(root, 'web/package.json'));
const { chromium } = pw('@playwright/test');
const port = process.env.CAPTURE_PORT ?? '5391';
const base = `http://127.0.0.1:${port}`;
const password = 'CaptureUpdated456!';

// The egress guard refuses loopback; serve the fake apps on a private bridge address.
function privateAddress() {
  const all = Object.entries(networkInterfaces()).flatMap(([name, list]) => (list ?? []).map(i => ({ name, ...i })));
  const ok = all.filter(i => i.family === 'IPv4' && !i.internal && /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(i.address));
  return (ok.find(i => i.name === 'docker0') ?? ok.find(i => !/^(wl|en|eth)/.test(i.name)) ?? ok[0])?.address;
}
const fakeHost = process.env.CAPTURE_FAKE_HOST ?? privateAddress();
if (!fakeHost) throw new Error('no private IPv4 address for the fake apps');
const fake = createServer((req, res) => {
  res.writeHead(503, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ schema: 'ky.health/1', service: 'fixture', status: 'down', checks: [{ name: 'database', status: 'down', reason: 'connection refused' }] }));
});
await new Promise(r => fake.listen(0, fakeHost, r));
const fakeBase = `http://${fakeHost}:${fake.address().port}`;

// Refuse to start when another process holds the port: the run would capture someone else's server.
const busy = await new Promise(r => { const s = connect(+port, '127.0.0.1', () => { s.destroy(); r(true); }); s.on('error', () => r(false)); });
if (busy) { fake.close(); throw new Error(`127.0.0.1:${port} is in use by another process; set CAPTURE_PORT`); }

const binary = join(root, '.browser/server');
const dir = await mkdtemp(join(tmpdir(), 'kypulse-capture-'));
const server = spawn(binary, [], {
  cwd: dir, stdio: ['ignore', 'ignore', 'inherit'],
  env: {
    PATH: process.env.PATH, KYPULSE_APP_URL: base, KYPULSE_HOST: '127.0.0.1', KYPULSE_PORT: port,
    KYPULSE_DB_DRIVER: 'sqlite', KYPULSE_DATA_DIR: join(dir, 'data'), KYPULSE_BACKUP_DIR: join(dir, 'backups'),
    KYPULSE_ADMIN_PASSWORD: 'CaptureInitial123!', KYPULSE_CAPTCHA_PROVIDER: 'none', KY_LOG_LEVEL: 'error',
  },
});
const exited = new Promise(r => server.once('exit', r));
const cleanup = async () => { server.kill('SIGTERM'); await exited; fake.close(); await rm(dir, { recursive: true, force: true }); };

async function api(ctx, method, path, data) {
  const csrf = (await ctx.storageState()).cookies.find(c => c.name === 'ky_csrf')?.value;
  const res = await ctx.fetch(base + path, { method, data, headers: csrf ? { 'X-CSRF-Token': csrf } : {} });
  if (!res.ok()) throw new Error(`${method} ${path}: ${res.status()} ${await res.text()}`);
  return res.json().catch(() => null);
}
function sha(bytes) { return createHash('sha256').update(bytes).digest('hex'); }
function git(...args) { return execFileSync('git', ['-C', root, ...args]); }
// Hash of uncommitted source, excluding this evidence directory.
function worktreeHash() {
  const evidence = `:(exclude)${evidenceDir}`;
  const tracked = git('diff', 'HEAD', '--binary', '--', '.', evidence);
  const untracked = git('ls-files', '--others', '--exclude-standard', '-z', '--', '.', evidence).toString().split('\0').filter(Boolean);
  if (!tracked.length && !untracked.length) return null;
  const h = createHash('sha256').update(tracked);
  for (const file of untracked.sort()) h.update(file).update(execFileSync('cat', [join(root, file)]));
  return h.digest('hex');
}

// Backup carries a status badge inside each fact label; the app page is a plain-label consumer.
const surfaces = { backup: { hash: '#/backup', ready: 'Backup & recovery' } };
const facts = page => page.evaluate(() => [...document.querySelectorAll('.dr-facts .dr-fact')].map(f => {
  const l = f.querySelector('.dr-fact-label'), s = l.querySelector('span') ?? l, b = l.querySelector('.badge'), v = f.querySelector('.dr-fact-value');
  const r = e => e.getBoundingClientRect();
  return { label: s.textContent, badge: b?.textContent ?? null, badgeOnLabelLine: b ? Math.abs(r(b).top - r(s).top) < 6 : null,
    labelWidth: Math.round(r(l).width), valueLeft: Math.round(r(v).left) };
}));

try {
  const browser = await chromium.launch();
  const setup = await browser.newContext({ baseURL: base });
  for (let i = 0; ; i++) {
    try { await setup.request.get('/healthz'); break; } catch (e) { if (i > 100) throw e; await new Promise(r => setTimeout(r, 100)); }
  }
  await setup.request.get('/');
  await api(setup.request, 'POST', '/api/auth/login', { username: 'admin', password: 'CaptureInitial123!' });
  await api(setup.request, 'POST', '/api/auth/change-password', { current_password: 'CaptureInitial123!', new_password: password });
  await api(setup.request, 'POST', '/api/auth/login', { username: 'admin', password });
  const id = (await api(setup.request, 'POST', '/api/targets', { name: 'Backup agent', url: fakeBase + '/backup', interval_sec: 10, enabled: true })).target.id;
  for (let i = 0; i < 90; i++) {
    if ((await api(setup.request, 'GET', '/api/status')).down > 0) break;
    await new Promise(r => setTimeout(r, 1000));
  }
  surfaces['app-detail'] = { hash: `#/apps/${id}`, ready: 'Backup agent' };
  const storage = await setup.storageState();
  await setup.close();
  const open = async (page, s) => {
    await page.goto('/' + s.hash);
    await page.getByRole('heading', { name: s.ready, exact: true }).first().waitFor();
    await page.locator('.dr-facts').first().waitFor();
    await page.waitForLoadState('networkidle');
    await page.evaluate(() => document.fonts.ready);
  };

  const out = join(here, 'screenshots');
  await mkdir(out, { recursive: true });
  const shots = [];
  const checks = [];
  const check = async (name, fn) => {
    try { checks.push({ name, pass: true, detail: (await fn()) ?? '' }); } catch (e) { checks.push({ name, pass: false, detail: String(e.message ?? e).split('\n')[0] }); }
  };
  let fonts = '';
  const errors = [];
  for (const theme of ['light', 'dark']) {
    for (const [device, viewport] of [['desktop', { width: 1280, height: 900 }], ['mobile', { width: 390, height: 844 }]]) {
      const ctx = await browser.newContext({ baseURL: base, colorScheme: theme, viewport, deviceScaleFactor: 1, storageState: storage, serviceWorkers: 'block' });
      const page = await ctx.newPage();
      page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
      page.on('pageerror', e => errors.push(e.message));
      for (const [surface, s] of Object.entries(surfaces)) {
        await open(page, s);
        fonts ||= await page.evaluate(() => `Space Grotesk loaded: ${document.fonts.check('15px "Space Grotesk"')}`);
        const f = await facts(page);
        if (theme === 'light') {
          if (surface === 'backup') await check(`${device} backup: every status badge sits on its label's line`, async () => {
            const off = f.filter(x => x.badgeOnLabelLine === false).map(x => `${x.label} ${x.badge}`);
            if (off.length) throw new Error(`wrapped: ${off.join(', ')}`);
            return f.map(x => `${x.label} ${x.badge}`).join('; ');
          });
          await check(`${device} ${surface}: values share one left edge; label column width`, async () => {
            const lefts = [...new Set(f.map(x => x.valueLeft))];
            if (lefts.length !== 1) throw new Error(`value left edges ${JSON.stringify(lefts)}`);
            return `value left ${lefts[0]}px; label widths ${JSON.stringify([...new Set(f.map(x => x.labelWidth))])}px`;
          });
        }
        await page.waitForTimeout(300);
        const file = `kypulse-${stage}-${phase}-${surface}-${device}-${theme}.png`;
        const bytes = await page.screenshot({ path: join(out, file), fullPage: true });
        shots.push({ file, sha256: sha(bytes), surface, url: s.hash, device, viewport, theme_source: `OS ${theme} (prefers-color-scheme), no saved choice` });
      }
      await ctx.close();
    }
  }
  await check('no console errors or CSP violations', async () => { if (errors.length) throw new Error(errors.join(' | ')); return 'none'; });

  const manifest = {
    product: 'kyPulse-server', stage, phase, captured_at: new Date().toISOString(),
    source_commit: git('rev-parse', 'HEAD').toString().trim(), uncommitted_source_sha256: worktreeHash(),
    server_binary_sha256: sha(await readFile(binary)),
    os: `${type()} ${release()}`, driver: `Playwright ${pw('@playwright/test/package.json').version} Chromium ${browser.version()}, device scale 1, service workers blocked`,
    fonts: `self-hosted Space Grotesk bundled in web/dist; ${fonts}`,
    server: `.browser/server built from this tree (embedding the committed web/dist) on 127.0.0.1:${port}; temporary SQLite and KYPULSE_BACKUP_DIR, removed after capture`,
    fixture: 'synthetic: admin account; one watched app "Backup agent" whose capture-owned fake health endpoint (private bridge address) answers 503; Backup has no pinned key, no KyRecovery pairing, an empty local backup directory and the default daily schedule. The local directory path, durations and next-run time are live and differ between runs',
    shots,
    checks,
  };
  await writeFile(join(here, `manifest-${stage}-${phase}.json`), JSON.stringify(manifest, null, 2) + '\n');
  await browser.close();
  console.log(`${shots.length} captures, ${checks.filter(c => c.pass).length}/${checks.length} checks passed, manifest-${stage}-${phase}.json`);
  for (const c of checks.filter(c => !c.pass)) console.log(`FAIL ${c.name}: ${c.detail}`);
} finally {
  await cleanup();
}
