// Visual refinement captures against a disposable kyPulse with synthetic data only.
// From the repository root, after `cd web && npm ci && npm run build` and
// `go build -o .browser/server ./cmd/server`:
//   node docs/ui-visual-refinement-2026-10-26/capture.mjs <stage nn> <before|after>
import { createHash } from 'node:crypto';
import { createServer } from 'node:http';
import { createRequire } from 'node:module';
import { connect } from 'node:net';
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises';
import { networkInterfaces, release, tmpdir, type } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawn } from 'node:child_process';

const [stage, phase] = process.argv.slice(2);
if (!/^\d\d$/.test(stage ?? '') || !['before', 'after'].includes(phase)) throw new Error('usage: capture.mjs <nn> <before|after>');
const evidenceDir = 'docs/ui-visual-refinement-2026-10-26';
const root = fileURLToPath(new URL('../../', import.meta.url));
const here = fileURLToPath(new URL('.', import.meta.url));
const { chromium } = createRequire(join(root, 'web/package.json'))('@playwright/test');
const port = process.env.CAPTURE_PORT ?? '5391';
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

// Refuse to start when another process holds a port: the run would capture someone else's server.
for (const p of [port]) {
  const busy = await new Promise(r => { const s = connect(+p, '127.0.0.1', () => { s.destroy(); r(true); }); s.on('error', () => r(false)); });
  if (busy) throw new Error(`127.0.0.1:${p} is in use by another process; set CAPTURE_PORT`);
}
const dir = await mkdtemp(join(tmpdir(), 'kypulse-capture-'));
const server = spawn(join(root, '.browser/server'), [], {
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

// One app's page (the down Backup agent) under Busnes, and under the saved light (Sky) and
// dark (Forest) suite palettes of the fleet review F7 captures. Nothing is clicked.
const surfaces = {};
const variants = { 'app-detail': null, 'app-delete-sky': 'sky', 'app-delete-forest': 'forest' };
const targetIds = {};

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
    targetIds[name] = (await api(setup.request, 'POST', '/api/targets', { name, url: fakeBase + path, interval_sec: 10, enabled: true })).target.id;
  }
  for (let i = 0; i < 90; i++) {
    const s = await api(setup.request, 'GET', '/api/status');
    if (s.pending === 0 && s.down > 0) break;
    await new Promise(r => setTimeout(r, 1000));
  }
  for (const [surface, saved] of Object.entries(variants)) surfaces[surface] = { hash: `#/apps/${targetIds['Backup agent']}`, ready: 'Backup agent', saved };
  const storage = await setup.storageState();
  await setup.close();

  const out = join(here, 'screenshots');
  await mkdir(out, { recursive: true });
  const shots = [];
  let fonts = '';
  for (const theme of ['light', 'dark']) {
    for (const [device, viewport] of [['desktop', { width: 1280, height: 900 }], ['mobile', { width: 390, height: 844 }]]) {
      const ctx = await browser.newContext({ baseURL: base, colorScheme: theme, viewport, deviceScaleFactor: 1, storageState: storage, serviceWorkers: 'block' });
      const page = await ctx.newPage();
      for (const [surface, s] of Object.entries(surfaces)) {
        await page.goto('/' + s.hash);
        await page.evaluate(v => v ? localStorage.setItem('ky_theme', v) : localStorage.removeItem('ky_theme'), s.saved ?? null);
        await page.reload();
        await page.getByRole('heading', { name: s.ready, exact: true }).first().waitFor();
        await page.getByRole('heading', { name: 'Alert history' }).waitFor();
        await page.evaluate(() => document.fonts.ready);
        fonts ||= await page.evaluate(() => `Space Grotesk loaded: ${document.fonts.check('15px "Space Grotesk"')}`);
        await page.waitForTimeout(300);
        const file = `kypulse-${stage}-${phase}-${surface}-${device}-${theme}.png`;
        const bytes = await page.screenshot({ path: join(out, file), fullPage: true });
        shots.push({ file, sha256: sha(bytes), surface, url: s.hash, device, viewport, theme_source: s.saved ? `saved choice ${s.saved}; OS ${theme} (prefers-color-scheme)` : `OS ${theme} (prefers-color-scheme), no saved choice` });
      }
      await ctx.close();
    }
  }
  // Behaviour checks in the real app, both phases: each records pass/fail, none aborts.
  const checks = [];
  const check = async (name, fn) => {
    try { checks.push({ name, pass: true, detail: (await fn()) ?? '' }); } catch (e) { checks.push({ name, pass: false, detail: String(e.message ?? e).split('\n')[0] }); }
  };
  const errors = [];
  const ctx = await browser.newContext({ baseURL: base, colorScheme: 'light', viewport: { width: 1280, height: 900 }, storageState: storage });
  const page = await ctx.newPage();
  page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
  page.on('pageerror', e => errors.push(e.message));
  for (const saved of ['sky', 'forest', null]) {
    await page.goto('/' + surfaces['app-detail'].hash);
    await page.evaluate(v => v ? localStorage.setItem('ky_theme', v) : localStorage.removeItem('ky_theme'), saved);
    await page.reload();
    await page.getByRole('heading', { name: 'Alert history' }).waitFor();
    await check(`${saved ?? 'busnes'}: the Status link is not a browser-default link colour`, async () => {
      const c = await page.getByRole('link', { name: 'Status', exact: true }).last().evaluate(a => getComputedStyle(a).color);
      if (['rgb(0, 0, 238)', 'rgb(85, 26, 139)'].includes(c)) throw new Error(`link colour ${c}`);
      return `link colour ${c}`;
    });
  }
  await check('the page header holds no filled destructive control', async () => {
    const inHeader = await page.locator('.dr-header .btn-danger').count();
    const del = page.getByRole('button', { name: 'Delete', exact: true });
    const cls = await del.getAttribute('class');
    if (inHeader) throw new Error(`${inHeader} .btn-danger in the page header`);
    return `Delete keeps class "${cls}" outside the header`;
  });
  await check('setting labels are sentence case, not uppercase', async () => {
    const t = await page.locator('.dr-fact-label').first().evaluate(el => getComputedStyle(el).textTransform);
    if (t === 'uppercase') throw new Error('text-transform uppercase');
    return `text-transform ${t}`;
  });
  await check('no console errors or CSP violations', async () => { if (errors.length) throw new Error(errors.join(' | ')); return 'none'; });
  await ctx.close();

  const manifest = {
    product: 'kyPulse-server', stage, phase, captured_at: new Date().toISOString(),
    source_commit: git('rev-parse', 'HEAD').toString().trim(), uncommitted_source_sha256: worktreeHash(),
    os: `${type()} ${release()}`, driver: `Playwright ${createRequire(join(root, 'web/package.json'))('@playwright/test/package.json').version} Chromium ${browser.version()}, device scale 1, service workers blocked`,
    fonts: `self-hosted Space Grotesk bundled in web/dist; ${fonts}`,
    server: 'disposable .browser/server (built from this tree, embedding the committed or rebuilt web/dist) on loopback with temporary SQLite data, removed after capture',
    fixture: 'synthetic: admin account; three watched apps (Billing API ok, Mail relay degraded, Backup agent down) served by a capture-owned fake health endpoint on a private bridge address. Durations and timestamps are live and differ between runs',
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
