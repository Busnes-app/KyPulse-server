import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn, execFileSync } from 'node:child_process';
import { DatabaseSync } from 'node:sqlite';

// Always own disposable data. Never reuse a running development/production server.
const dir = await mkdtemp(join(tmpdir(), 'ky-browser-'));
const binary = fileURLToPath(new URL('../../.browser/server', import.meta.url));
const options = {
  cwd: dir,
  env: {
    PATH: process.env.PATH,
    KYPULSE_APP_URL: 'http://127.0.0.1:5391',
    KYPULSE_HOST: '127.0.0.1', KYPULSE_PORT: '5391', KYPULSE_DB_DRIVER: 'sqlite',
    KYPULSE_DATA_DIR: join(dir, 'data'), KYPULSE_BACKUP_DIR: join(dir, 'backups'),
    KYPULSE_ADMIN_PASSWORD: 'BrowserInitial123!', KYPULSE_CAPTCHA_PROVIDER: 'none',
    KYPULSE_ALERT_ALLOW_HTTP: 'true', KYPULSE_KYYARD_ALLOW_HTTP: 'true',
  },
  stdio: 'inherit',
};
// Bootstrap a real disposable viewer with the normal password hash, then lower its role
// before the HTTP process starts. This database belongs only to this harness.
try {
  execFileSync(binary, ['init-admin', '-username', 'admin', '-password', 'BrowserInitial123!'], options);
  execFileSync(binary, ['init-admin', '-username', 'viewer', '-password', 'BrowserViewer123!'], options);
  const fixtureDB = new DatabaseSync(join(dir, 'data', 'kypulse.db'));
  fixtureDB.prepare("UPDATE users SET role='viewer', must_change_password=0 WHERE username='viewer'").run();
  fixtureDB.close();
} catch (error) {
  await rm(dir, { recursive: true, force: true });
  throw error;
}
const server = spawn(binary, [], options);
for (const signal of ['SIGTERM', 'SIGINT']) process.on(signal, () => server.kill('SIGTERM'));
server.on('error', async error => { console.error(error.message); await rm(dir, { recursive: true, force: true }); process.exit(1); });
server.on('exit', async code => { await rm(dir, { recursive: true, force: true }); process.exit(code ?? 1); });
