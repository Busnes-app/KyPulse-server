// Staged UI captures against a disposable kyPulse with synthetic data only.
// From the repository root, after `cd web && npm ci && npm run build` and
// `go build -o .browser/server ./cmd/server`:
//   node docs/ui-fleet-2026-10-26/capture.mjs <stage nn> <before|after>
import { createHash } from 'node:crypto';
import { createServer } from 'node:http';
import { createRequire } from 'node:module';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { networkInterfaces, release, tmpdir, type } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawn } from 'node:child_process';

const [stage, phase] = process.argv.slice(2);
if (!/^\d\d$/.test(stage ?? '') || !['before', 'after'].includes(phase)) throw new Error('usage: capture.mjs <nn> <before|after>');
const root = fileURLToPath(new URL('../../', import.meta.url));
const here = fileURLToPath(new URL('.', import.meta.url));
const { chromium } = createRequire(join(root, 'web/package.json'))('@playwright/test');
const port = process.env.CAPTURE_PORT ?? '5491';
const base = `http://127.0.0.1:${port}`;
const password = 'CaptureUpdated456!';

// The egress guard refuses loopback; serve the fake apps on a private bridge address
// (docker0 when present) rather than a LAN-facing interface.
function privateAddress() {
  const all = Object.entries(networkInterfaces()).flatMap(([name, list]) => (list ?? []).map(i => ({ name, ...i })));
  const ok = all.filter(i => i.family === 'IPv4' && !i.internal && /^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)/.test(i.address));
  return (ok.find(i => i.name === 'docker0') ?? ok.find(i => !/^(wl|en|eth)/.test(i.name)) ?? ok[0])?.address;
}
const fakeHost = process.env.CAPTURE_FAKE_HOST ?? privateAddress();
if (!fakeHost) throw new Error('no private IPv4 address for the fake apps');
const health = { '/billing': ['ok', 200], '/relay': ['degraded', 200], '/backup': ['down', 503] };
const fake = createServer((req, res) => {
  const [status, code] = health[req.url] ?? ['down', 404];
  res.writeHead(code, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ schema: 'ky.health/1', service: 'fixture', status, checks: [{ name: 'database', status, reason: status === 'ok' ? undefined : 'connection refused' }] }));
});
await new Promise(r => fake.listen(0, fakeHost, r));
const fakeBase = `http://${fakeHost}:${fake.address().port}`;

const dir = await mkdtemp(join(tmpdir(), 'kypulse-capture-'));
const server = spawn(join(root, '.browser/server'), [], {
  cwd: dir, stdio: ['ignore', 'ignore', 'inherit'],
  env: {
    PATH: process.env.PATH, KYPULSE_APP_URL: base, KYPULSE_HOST: '127.0.0.1', KYPULSE_PORT: port,
    KYPULSE_DB_DRIVER: 'sqlite', KYPULSE_DATA_DIR: join(dir, 'data'), KYPULSE_BACKUP_DIR: join(dir, 'backups'),
    KYPULSE_ADMIN_PASSWORD: 'CaptureInitial123!', KYPULSE_CAPTCHA_PROVIDER: 'none', KY_LOG_LEVEL: 'error',
  },
});
const cleanup = async () => { server.kill('SIGTERM'); fake.close(); await rm(dir, { recursive: true, force: true }); };

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
  const evidence = ':(exclude)docs/ui-fleet-2026-10-26';
  const tracked = git('diff', 'HEAD', '--binary', '--', '.', evidence);
  const untracked = git('ls-files', '--others', '--exclude-standard', '-z', '--', '.', evidence).toString().split('\0').filter(Boolean);
  if (!tracked.length && !untracked.length) return null;
  const h = createHash('sha256').update(tracked);
  for (const file of untracked.sort()) h.update(file).update(execFileSync('cat', [join(root, file)]));
  return h.digest('hex');
}

const surfaces = {
  status: { hash: '#/status', ready: 'Status' },
  appearance: { hash: phase === 'before' ? '#/settings' : '#/appearance', ready: phase === 'before' ? 'Settings' : 'Appearance' },
};

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
  for (const [name, path] of [['Billing API', '/billing'], ['Mail relay', '/relay'], ['Backup agent', '/backup']]) {
    await api(setup.request, 'POST', '/api/targets', { name, url: fakeBase + path, interval_sec: 10, enabled: true });
  }
  for (let i = 0; i < 90; i++) {
    const s = await api(setup.request, 'GET', '/api/status');
    if (s.pending === 0 && s.down > 0) break;
    await new Promise(r => setTimeout(r, 1000));
  }
  const storage = await setup.storageState();
  await setup.close();

  const out = join(here, 'screenshots');
  await mkdir(out, { recursive: true });
  const shots = [];
  for (const theme of ['light', 'dark']) {
    for (const [device, viewport] of [['desktop', { width: 1280, height: 900 }], ['mobile', { width: 390, height: 844 }]]) {
      const ctx = await browser.newContext({ baseURL: base, colorScheme: theme, viewport, deviceScaleFactor: 1, storageState: storage, serviceWorkers: 'block' });
      const page = await ctx.newPage();
      for (const [surface, s] of Object.entries(surfaces)) {
        await page.goto('/' + s.hash);
        await page.getByRole('heading', { name: s.ready, exact: true }).first().waitFor();
        if (surface === 'status') await page.getByText('Backup agent', { exact: true }).first().waitFor();
        await page.evaluate(() => document.fonts.ready);
        await page.waitForTimeout(300);
        const file = `kypulse-${stage}-${phase}-${surface}-${device}-${theme}.png`;
        const bytes = await page.screenshot({ path: join(out, file), fullPage: true });
        shots.push({ file, sha256: sha(bytes), surface, url: s.hash, device, viewport, theme_source: `OS ${theme} (prefers-color-scheme), no saved choice` });
      }
      await ctx.close();
    }
  }
  const manifest = {
    product: 'kyPulse-server', stage, phase, captured_at: new Date().toISOString(),
    source_commit: git('rev-parse', 'HEAD').toString().trim(), uncommitted_source_sha256: worktreeHash(),
    os: `${type()} ${release()}`, driver: `Playwright ${createRequire(join(root, 'web/package.json'))('@playwright/test/package.json').version} Chromium ${browser.version()}, device scale 1, service workers blocked`,
    server: 'disposable .browser/server on loopback with temporary SQLite data, removed after capture',
    fixture: 'synthetic: admin account; three watched apps (Billing API ok, Mail relay degraded, Backup agent down) served by a capture-owned fake health endpoint on a private bridge address',
    shots,
  };
  await writeFile(join(here, `manifest-${stage}-${phase}.json`), JSON.stringify(manifest, null, 2) + '\n');
  await browser.close();
  console.log(`${shots.length} captures, manifest-${stage}-${phase}.json`);
} finally {
  await cleanup();
}
