// Pulse: Backup page facts (adjacent .dr-fact-label consumer with Badges) at base vs head,
// and app-detail Delete placement/guard/confirmation at head (admin real; user via mocked role).
import { spawn } from 'node:child_process';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { connect } from 'node:net';
import { join } from 'node:path';
import { chromium } from '/home/yoshi/git/busnes.app/ky-ui/.claude/worktrees/fleet-appearance/node_modules/playwright/index.mjs';

const OUT = '/home/yoshi/git/busnes.app/kyPulse-server/.claude/worktrees/fleet-review-followups/docs/ui-review-followups-2026-10-26/reviewer-verify/pulse';
const PORT = '5391', base = `http://127.0.0.1:${PORT}`, password = 'ReviewUpdated456!';
const res = { checks: [], runs: {} };
const check = (name, pass, detail) => { res.checks.push({ name, pass, detail }); console.log(pass ? 'PASS' : 'FAIL', name, JSON.stringify(detail)); };
async function run(label, bin) {
  const busy = await new Promise(r => { const s = connect(+PORT, '127.0.0.1', () => { s.destroy(); r(true); }); s.on('error', () => r(false)); });
  if (busy) throw new Error('5391 busy; refusing');
  const dir = mkdtempSync(`/tmp/kypulse-r2-verify-6y7s/run-pulse-${label}-`);
  const proc = spawn(bin, [], { cwd: dir, stdio: ['ignore', 'ignore', 'inherit'], env: { PATH: process.env.PATH, KYPULSE_APP_URL: base, KYPULSE_HOST: '127.0.0.1', KYPULSE_PORT: PORT,
    KYPULSE_DB_DRIVER: 'sqlite', KYPULSE_DATA_DIR: join(dir, 'data'), KYPULSE_BACKUP_DIR: join(dir, 'backups'), KYPULSE_ADMIN_PASSWORD: 'ReviewInitial123!', KYPULSE_CAPTCHA_PROVIDER: 'none', KY_LOG_LEVEL: 'error' } });
  const exited = new Promise(r => proc.once('exit', r));
  const browser = await chromium.launch(); res.browser = `chromium ${browser.version()}`;
  try {
    for (let i = 0; ; i++) { try { if ((await fetch(base + '/healthz')).ok) break; } catch {} if (i > 100) throw new Error('not ready'); await new Promise(r => setTimeout(r, 100)); }
    const setup = await browser.newContext({ baseURL: base }); const rq = setup.request;
    const api = async (method, path, data) => { const csrf = (await setup.storageState()).cookies.find(c => c.name === 'ky_csrf')?.value;
      const r = await rq.fetch(base + path, { method, data, headers: csrf ? { 'X-CSRF-Token': csrf } : {} }); if (!r.ok()) throw new Error(`${method} ${path} ${r.status()} ${await r.text()}`); return r.json().catch(() => null); };
    await rq.get('/');
    await api('POST', '/api/auth/login', { username: 'admin', password: 'ReviewInitial123!' });
    await api('POST', '/api/auth/change-password', { current_password: 'ReviewInitial123!', new_password: password });
    await api('POST', '/api/auth/login', { username: 'admin', password });
    const id = (await api('POST', '/api/targets', { name: 'Paused fixture app', url: 'https://example.invalid/health', interval_sec: 60, enabled: false })).target.id;
    const storage = await setup.storageState(); await setup.close();
    for (const [device, viewport] of [['desktop', { width: 1280, height: 900 }], ['phone', { width: 390, height: 844 }]]) {
      const ctx = await browser.newContext({ baseURL: base, viewport, storageState: storage, colorScheme: 'light', serviceWorkers: 'block' });
      const page = await ctx.newPage();
      await page.goto('/#/backup'); await page.locator('.dr-facts').waitFor(); await page.waitForLoadState('networkidle');
      const facts = await page.evaluate(() => [...document.querySelectorAll('.dr-facts .dr-fact')].map(f => {
        const l = f.querySelector('.dr-fact-label'), s = l.querySelector('span'), b = l.querySelector('.badge'), v = f.querySelector('.dr-fact-value');
        const r = (e) => e && e.getBoundingClientRect();
        return { label: s?.textContent, badge: b?.textContent, labelBox: [Math.round(r(l).width), Math.round(r(l).height)],
          badgeSameLineAsLabel: b && s ? Math.abs(r(b).top - r(s).top) < 6 : null, badgeRightAligned: b ? Math.round(r(l).right - r(b).right) : null,
          valueLeftOfLabelRight: v ? Math.round(r(v).left - r(l).right) : null, valueBelowLabel: v ? r(v).top >= r(l).bottom - 1 : null };
      }));
      res.runs[`${label}-${device}`] = facts;
      console.log(label, device, JSON.stringify(facts));
      await page.locator('.dr-facts').screenshot({ path: `${OUT}-${label}-backup-facts-${device}.png` });
      if (label === 'head') {
        await page.goto(`/#/apps/${id}`); await page.locator('.dr-header h1').waitFor();
        const layout = await page.evaluate(() => ({ header: [...document.querySelectorAll('.dr-header button')].map(b => b.textContent.trim()),
          remove: [...document.querySelectorAll('.dr-remove button')].map(b => ({ text: b.textContent.trim(), cls: b.className })),
          backLinkColor: getComputedStyle(document.querySelector('.dr-hint a')).color, ink: getComputedStyle(document.documentElement).getPropertyValue('--ink-strong') }));
        check(`head ${device}: admin header has only Edit; Delete is the filled danger in Stop watching`, layout.header.join() === 'Edit' && layout.remove.length === 1 && /btn-danger/.test(layout.remove[0].cls), layout);
        const deletes = []; page.on('request', r => { if (r.method() === 'DELETE') deletes.push(r.url()); });
        let msg = null; page.once('dialog', d => { msg = d.message(); d.dismiss(); });
        await page.locator('.dr-remove .btn-danger').click(); await page.waitForTimeout(300);
        check(`head ${device}: Delete confirms; dismiss sends no DELETE`, !!msg && deletes.length === 0, { msg, deletes });
        await page.screenshot({ path: `${OUT}-head-appdetail-${device}.png`, fullPage: true });
      }
      await ctx.close();
    }
    if (label === 'head') {
      const ctx = await browser.newContext({ baseURL: base, viewport: { width: 1280, height: 900 }, storageState: storage });
      await ctx.route('**/api/auth/me', async (route) => { const r = await route.fetch(); const j = await r.json(); (j.user ?? j).role = 'user'; await route.fulfill({ response: r, json: j }); });
      const page = await ctx.newPage(); await page.goto(`/#/apps/${id}`); await page.locator('.dr-header h1').waitFor(); await page.waitForTimeout(300);
      const n = await page.evaluate(() => ({ edit: document.querySelectorAll('.dr-header button').length, remove: document.querySelectorAll('.dr-remove').length, danger: document.querySelectorAll('.btn-danger').length }));
      check('head: non-admin (mocked role) sees no Edit, no Stop watching, no danger button', n.edit === 0 && n.remove === 0 && n.danger === 0, n);
      await ctx.close();
    }
  } finally { await browser.close(); proc.kill('SIGTERM'); await exited; rmSync(dir, { recursive: true, force: true }); }
}
try {
  await run('base', '/tmp/kypulse-r2-verify-6y7s/bin/pulse-base');
  await run('head', '/tmp/kypulse-r2-verify-6y7s/bin/pulse-head');
} finally { writeFileSync(`${OUT}-results.json`, JSON.stringify(res, null, 1)); }
