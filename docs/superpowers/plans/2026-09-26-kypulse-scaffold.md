# kyPulse scaffold (step 2a) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the ky_server_base copy in this repo into kyPulse: renamed end to end, without device pairing and SCIM, with `admin`/`viewer` roles enforced, its own `/healthz`, and structured stderr logging.

**Architecture:** Six tasks on one branch, each leaving the build green and CI-shaped checks passing. Task 1 is the mechanical rename (module, env prefix, binary, image, database names, docs, CI). Tasks 2 and 3 delete the two subsystems kyPulse does not need, backend then frontend. Task 4 narrows the role enum and hides admin-only tabs from viewers. Task 5 adds `/healthz` from `ky-primitives/health` and routes every log line through `ky-primitives/logging`. Task 6 writes the kyPulse DOX docs and runs the closeout greps.

**Tech Stack:** Go 1.26 (`go.mod` pins 1.26.6), `modernc.org/sqlite`, `pgx`, React 19 + TypeScript + Vite, Vitest, Playwright; `ky-primitives` v0.9.0 (`health`, `logging`, `recoveryclient`, `password`, `keyfile`).

**Spec:** `docs/superpowers/specs/2026-09-26-kypulse-design.md`, section 2 "kyPulse server" → "Scaffold changes". Steps 2b (monitoring backend) and 2c (Status/Alerts UI) are separate plans that build on this one.

**Repository:** `/home/yoshi/git/busnes.app/kyPulse-server`. Work on branch `feat/kypulse-scaffold` off `docs/kypulse-design` (which carries the spec and this plan; it is the branch to merge to `master` afterwards). All paths are relative to the repo root. The remote is `https://github.com/Busnes-app/KyPulse-server.git`, so the published image name is `ghcr.io/busnes-app/kypulse-server` and the Go module is `github.com/Busnes-app/kypulse-server`.

**Deferred to step 2b:** the spec's "own audit trail uses `auditchain`" and the `internal/egress` guard; the scaffold's `audit_records` table keeps serving until then.

**Baseline (verified 2026-09-26):** `go test ./...` passes in every package; `cd web && npm test` runs 7 tests, all passing, after `check-vendor.mjs` verifies ky-ui 0.2.0.

## Global Constraints

- Module path `github.com/Busnes-app/kypulse-server`; binary `kypulse`; compose `container_name: kypulse`; image default `ghcr.io/busnes-app/kypulse-server:latest`; web package name `kypulse-web`; HTML title, manifest name and `DefaultAppName` are `kyPulse`.
- Every environment variable this program reads is `KYPULSE_*` (spec: "Env prefix `KYPULSE_`"), except `KY_LOG_LEVEL`, which `ky-primitives/logging` reads itself and is suite-wide, and `PORT`, which stays as the fallback for `KYPULSE_PORT`.
- `internal/devices`, `internal/scim`, `/api/devices/*`, `/scim/v2` and their web pages are gone (spec: "Remove device QR pairing ... and SCIM").
- Kept as-is: local login with MFA, SSO including KyIdentity/KySignOn, sessions, CAPTCHA, settings, theme, kyrecovery backup (spec: "Keep ...").
- Users are `admin` or `viewer`; new users default to `viewer`; the store refuses any other value; every route enforces the role server-side (spec: "Roles").
- `GET /healthz` is public, served by `health.Handler("kypulse", lg, ...)` with one check named `database`; response shape `ky.health/1` (spec section 1).
- All process output goes to stderr as JSON lines through `ky-primitives/logging` with `App: "kypulse"` (spec: "Own logs").
- `web/dist` is committed after every frontend change; CI fails on a stale build (`git diff --exit-code -- web/dist`).
- `web/src/ky-ui/` is vendored and hash-pinned: never hand-edit it.
- The sealed KyRecovery token label becomes `kypulse:setting:kyrecovery_token`. kyPulse has no deployments, so no sealed row exists under the old label.
- Migration 1 is edited in place (tables dropped, role default changed) for the same reason: no database exists yet that ran it.
- Historical ky_server_base plans under `docs/superpowers/plans/2026-09-0*` are deleted; they describe another product's history and still live in that repo.

## Review Focus

1. **A `KY_`-prefixed variable an operator still sets** (old compose file, old shell profile) is silently ignored. Expect startup to keep working on defaults, and the rename to be complete so docs and compose agree. Pinned by Task 1's closeout grep (no `\bKY_` outside `KY_LOG_LEVEL` and `docs/superpowers/`).
2. **A KySignOn or OIDC identity provider asserting `role: "manager"` or `role: "user"`.** Expect the user to land as `viewer`, never as an unknown role that no route recognises. Pinned by `TestRoleForMapsUnknownRolesToViewer` (Task 4).
3. **A viewer session calling an admin route directly.** Expect 403 regardless of what the UI hides. Pinned by `TestPrivilegedEndpointsRequireAdmin` with `viewer` (Task 4).
4. **`/healthz` while the database is unreachable.** Expect 503 with `checks[0].status == "down"` and no DSN or hostname in the body. Pinned by `TestHealthzReportsDatabaseDown` (Task 5).
5. **A log line carrying a newline from user-controlled input.** Expect one JSON line, the control character replaced, so an attacker cannot forge a log record. Pinned by `TestLogBridgeEmitsOneJSONLinePerCall` (Task 5).

---

### Task 1: Rename the scaffold to kyPulse

**Files:**
- Modify: `go.mod`, every `*.go` file, `Makefile`, `Dockerfile`, `.dockerignore`, `docker-compose.yml`, `docker-compose.build.yml`, `docker-compose.lan-dns.yml`, `docker-compose.static-ip.yml`, `.github/workflows/ci.yml`, `.github/workflows/ky-primitives-compat.yml`, `scripts/smoke-test.sh`, `docs/RESTORE.md`, `internal/config/config.go`, `internal/config/AGENTS.md`, `internal/backup/settings.go`, `internal/backup/AGENTS.md`, `cmd/server/main.go`, `web/package.json`, `web/package-lock.json`, `web/index.html`, `web/public/manifest.json`
- Delete: `scripts/ky-init.sh`, `docs/superpowers/plans/2026-09-0*.md` (22 files)
- Create: `README.md`
- Test: existing suites; `scripts/smoke-test.sh`

**Interfaces:**
- Consumes: nothing.
- Produces: the import path `github.com/Busnes-app/kypulse-server/...` every later task uses; env names `KYPULSE_*`; `config.DefaultAppName = "kyPulse"`.

- [ ] **Step 1: Create the branch**

```bash
git switch docs/kypulse-design && git switch -c feat/kypulse-scaffold
```

- [ ] **Step 2: Bump ky-primitives and rename the module**

```bash
go get github.com/Busnes-app/ky-primitives@v0.9.0
git ls-files '*.go' go.mod '*.md' | grep -v '^docs/superpowers/' | xargs sed -i 's|github.com/Busnes-app/ky_server_base|github.com/Busnes-app/kypulse-server|g'
sed -i 's|^module github.com/Busnes-app/kypulse-server$|module github.com/Busnes-app/kypulse-server|' go.mod   # no-op guard: confirms the line exists
grep -n '^module' go.mod
```

Expected: `module github.com/Busnes-app/kypulse-server`.

- [ ] **Step 3: Rename the env prefix everywhere the program, its scripts, compose files and docs read it**

```bash
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | grep -v '^web/src/ky-ui/' | xargs grep -l '\bKY_' | xargs sed -i 's/\bKY_/KYPULSE_/g'
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -n '\bKY_' ; echo "exit=$?"
```

Expected: no matches (`exit=1`). `KY_LOG_LEVEL` is not in this repo yet; Task 5 adds it by name and documents it as the one `KY_` exception. Also check `grep -rn 'KYPULSE_LOG_LEVEL' .` prints nothing.

- [ ] **Step 4: Rename binary, image, container, database and product strings**

Apply, in this order (the first pattern is a prefix of the second):

```bash
# binary and image names
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -l 'ky_server_base' | xargs sed -i 's/ky_server_base/kypulse/g'
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -l 'ky-server-base' | xargs sed -i 's/ky-server-base/kypulse-server/g'
# default database names (sqlite file, postgres db) and compose postgres container
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -l 'ky_server\b' | xargs sed -i 's/\bky_server\b/kypulse/g'
sed -i 's/container_name: ky_postgres/container_name: kypulse_postgres/' docker-compose.yml
sed -i 's/ky_ci/kypulse_ci/g' .github/workflows/ci.yml
```

Then by hand:

- `cmd/server/main.go`: `const appVersion = "0.1.0"`; the `version` case prints `fmt.Println("kypulse v0.1.0 (Busnes.app kyPulse)")`; every `[KY-BASE]` prefix becomes `[KYPULSE]`.
- `internal/config/config.go`: `const DefaultAppName = "kyPulse"`; the `Config` doc comment says "for kyPulse".
- `docker-compose.yml`: `KYPULSE_APP_NAME=kyPulse`; the `image:` line must read `image: ${KYPULSE_IMAGE:-ghcr.io/busnes-app/kypulse-server:latest}` (the sed produced it; confirm). In the verification comment block, `--repo Busnes-app/kypulse-server` and `--cert-identity https://github.com/Busnes-app/kypulse-server/.github/workflows/ci.yml@refs/heads/master` (sed produced these; confirm with `grep -n 'repo\|cert-identity' docker-compose.yml`). Delete the line beginning `#   Images built before 2026-09-16 cannot be verified by name any more`: kyPulse has no such images.
- `docs/RESTORE.md`: same confirmations for lines 85-93 and 103; replace the default app-name mentions `Busnes.app` at lines 51, 70 and 155 with `kyPulse`, and the example file `Busnes_2eapp.cap-XXXXXXXX.kycap` at line 67 with `kyPulse.cap-XXXXXXXX.kycap`. Delete the paragraph at line 273 that starts with `KYPULSE_SCIM_TOKEN` (SCIM is removed in Task 2).
- `web/package.json` and `web/package-lock.json`: `"name": "kypulse-web"` (the lock has it twice).
- `web/index.html`: `<title>kyPulse</title>`.
- `web/public/manifest.json`: `"short_name": "kyPulse"`, `"name": "kyPulse"`.
- `.gitignore`: remove the `/ky_server_base` line if the sed left it as `/kypulse` twice; keep exactly one `/kypulse`.
- `Dockerfile` first line: `# Multi-stage build for kypulse`.
- `.github/workflows/ky-primitives-compat.yml` line 60: the message names `kypulse-server` (sed did this; confirm).

- [ ] **Step 5: Delete the scaffold script and the base-era plans**

```bash
git rm -q scripts/ky-init.sh docs/superpowers/plans/2026-09-0*.md
ls docs/superpowers/plans
```

Expected: only `2026-09-26-ky-primitives-health.md` and `2026-09-26-kypulse-scaffold.md`.

- [ ] **Step 6: Replace the README gate in CI and write the README**

In `.github/workflows/ci.yml`, inside the step `image and attestation coordinates match this repository`, delete these lines (they enforce ky-server-base's owner-move warning, which kyPulse never had):

- the `grep -qxF '## Upgrading after the Busnes-app owner move' README.md ...` line
- the `grep -qxF 'The GitHub organisation was renamed on 2026-09-16 ...' README.md ...` line
- the four `bad=$(grep -i... 'busness-app' ...` lines and the `bad=$(grep -inE 'returns[[:space:]]+...` line

Keep everything else in the step: the `img`/`ident` derivation, the compose-and-docs file list, the `image:` default check, the "every image coordinate equals $img" check (change its `grep -vxF -e "$img" -e "ghcr.io/busness-app"` to `grep -vxF "$img"`), the `--repo`/`--cert-identity` checks, and the "README may only point at docs that exist" line.

Create `README.md`:

````markdown
# kyPulse

Unified health and logging for the Busnes.app Ky suite. kyPulse polls every Ky app's
`/healthz`, pulls container state and logs from KyYard, accepts logs from paired senders, and
sends an outbound webhook when an app goes down or degrades. It is read-only: it watches and
reports, and never changes the apps it watches.

Design: `docs/superpowers/specs/2026-09-26-kypulse-design.md`.

## Run

```sh
docker compose up -d
```

The published image is `ghcr.io/busnes-app/kypulse-server`; `docker-compose.yml` explains how
to pin a commit by digest and verify its attestation. Every setting is a `KYPULSE_*` variable
(see `internal/config/AGENTS.md`); `KY_LOG_LEVEL` sets the log level and is shared across the
suite. The first start creates an `admin` user and prints its password to stderr unless
`KYPULSE_ADMIN_PASSWORD` is set.

Watch kyPulse's own `GET /healthz` from outside; kyPulse does not monitor itself.

## Backup and restore

kyPulse backs up to KyRecovery like every suite product. `docs/RESTORE.md` is the restore
runbook.

## Develop

```sh
make ci      # tidy, gofmt, vet, race tests, frontend tests, smoke test
```
````

- [ ] **Step 7: Verify the rename builds, tests and smokes**

```bash
go mod tidy && git diff --exit-code go.mod go.sum || true   # inspect: ky-primitives must be v0.9.0
gofmt -l $(git ls-files '*.go')                              # must print nothing
go vet ./... && go build -o kypulse ./cmd/server && go test -count=1 ./...
shellcheck scripts/*.sh
scripts/smoke-test.sh
cd web && npm ci && npm test && npm run build && cd .. && git status --short web/dist
```

Expected: all `ok`; smoke test reports every check passed; `git status --short web/dist` shows the two hashed asset files and `index.html` modified (the title changed). If `go mod tidy` changed `go.sum`, that is expected from the bump.

Closeout greps, all of which must print nothing:

```bash
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -n 'ky_server_base\|ky-server-base\|KY-BASE\|Busnes.app Base\|ky-server-base-web'
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -n '\bKY_'
```

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "rename: ky_server_base scaffold becomes kypulse

Module, binary, image, container, env prefix (KYPULSE_), database names,
sealer label, docs and CI. Drops the scaffold-cloning script and the
base-era plans. ky-primitives v0.9.0."
```

---

### Task 2: Remove device pairing and SCIM (Go)

**Files:**
- Delete: `internal/devices/` (whole directory, including `AGENTS.md`), `internal/scim/` (whole directory)
- Modify: `internal/api/server.go`, `internal/api/device_handlers.go` (delete the file), `internal/api/settings_handlers.go`, `internal/api/api_test.go`, `internal/api/authz_test.go`, `internal/api/AGENTS.md`, `internal/config/config.go`, `internal/config/AGENTS.md`, `internal/store/store.go`, `internal/store/models.go`, `internal/store/sqlstore.go`, `internal/store/store_test.go`, `internal/store/password_test.go`, `internal/store/migrations/migrations.go`, `internal/store/AGENTS.md`, `docker-compose.yml`, `scripts/smoke-test.sh`, `go.mod`, `go.sum`
- Test: `go test ./...`

**Interfaces:**
- Consumes: Task 1's module path.
- Produces: `store.Store` without `Devices()`/`Groups()`; `config.Config` without `SCIM`; `api.NewServer(cfg *config.Config, st store.Store) *Server` unchanged in signature (Task 5 changes it).

- [ ] **Step 1: Delete the packages and the handler file**

```bash
git rm -rq internal/devices internal/scim internal/api/device_handlers.go
```

- [ ] **Step 2: Unwire the API server**

In `internal/api/server.go`:

- Remove the imports `.../internal/devices` and `.../internal/scim`.
- In `type Server struct`, delete the fields `pairing *devices.PairingService` and `scim *scim.Server`.
- In `NewServer`, delete `pairing := devices.NewPairingService(...)` and `scimSrv := scim.NewServer(...)`, and the `pairing:` and `scim:` entries of the struct literal.
- In `routes()`, delete the block:

```go
	// Devices & Ephemeral QR Pairing
	s.mux.HandleFunc("/api/devices/pair/init", s.requireAuthenticated(s.handlePairInit))
	s.mux.HandleFunc("/api/devices/pair/verify", s.handlePairVerify)
	s.mux.HandleFunc("/api/devices/pair/poll", s.handlePairPoll)
```

  and the block:

```go
	// SCIM 2.0 routes
	s.scim.RegisterRoutes(s.mux)
```

- In `ServeHTTP`, delete:

```go
	// SCIM middleware
	if strings.HasPrefix(r.URL.Path, "/scim/v2") {
		s.scim.AuthMiddleware(s.mux).ServeHTTP(w, r)
		return
	}
```

  `strings` is still used by `csrfExempt`, so the import stays. `requireAuthenticated` now has no caller; delete it (Task 5 does not need it either).

In `internal/api/settings_handlers.go`, delete the line `out["scim_enabled"] = s.config.SCIM.Enabled` and change the comment above the admin block to:

```go
	// extra_settings is admin-only. The KyRecovery pairing token is sealed at rest and never
	// leaves the process in either form: an admin can see that a recovery URL is set, never
	// the token that authenticates to it.
```

- [ ] **Step 3: Drop SCIM configuration**

In `internal/config/config.go`: delete the `SCIM SCIMConfig` field, the `SCIMConfig` type and its comment, and the `SCIM: SCIMConfig{...}` literal in `LoadFromEnv`. In `docker-compose.yml` delete `- KYPULSE_SCIM_ENABLED=true`. In `scripts/smoke-test.sh` delete the `KYPULSE_SCIM_ENABLED=true \` line inside `start_server` and the line `check "scim rejects missing bearer" "$(status "$BASE/scim/v2/Users")" "401"`.

- [ ] **Step 4: Drop the store surface**

In `internal/store/store.go`: delete `ErrPairingExpired`, the `Devices() DeviceStore` and `Groups() GroupStore` lines of `Store`, and the whole `DeviceStore` and `GroupStore` interfaces with their comments.

In `internal/store/models.go`: delete the `DevicePairing` and `Group` types with their comments; change the `SSOProvider` comment to `// "local", "kysignon", "oidc", "saml"` and the `Action` example comment to `// e.g. "auth.login", "admin.backup_run"`.

In `internal/store/sqlstore.go`:
- Delete the `devices *deviceStore` and `groups *groupStore` fields, the two `s.devices = ...`/`s.groups = ...` assignments, and the `Devices()`/`Groups()` accessor methods.
- Delete everything from the line `// Device Pairing Store` (with its `// ----` rule above it) through the closing brace of the last `groupStore` method, immediately before the `// ----` rule above `// Audit Store` (lines 522-793 in the current file).
- In `revokePasswordGrants`, change the table list to `[]string{"sessions", "mfa_challenges"}`.

In `internal/store/migrations/migrations.go`, migration 1: delete the `device_pairings` table and its two indexes, and the `groups` and `group_members` tables, from both the `SQLite` and `Postgres` strings. Leave `users`, `sessions`, `audit_records`, `server_settings` untouched (Task 4 edits the role default).

- [ ] **Step 5: Fix the tests that referenced them**

- `internal/store/store_test.go`: delete `TestDevicePairingLifecycle` and `TestGroupStoreAndMembers` entirely (lines 153-226). Remove any import that becomes unused (`uuid` is still used by `TestUserStoreLifecycle`; check with `go vet`).
- `internal/store/password_test.go`: delete the `st.Devices().CreatePairing(...)` block (lines 82-84) and the `st.Devices().GetPairingBySecret(...)` assertion (lines 99-101).
- `internal/api/api_test.go`: delete step `// 4. /api/devices/pair/init` (lines 128-137) and renumber the drill step to `// 4.`; delete `TestPairPollProjectsTheRecord` (lines 481-526, including its comment). Remove now-unused imports (`strings`, `time`) only if `go vet` reports them.
- `internal/api/authz_test.go`, `TestSettingsExposureByRole`: the seeded secret is a SCIM token. Rename it to a generic secret so the test still proves the tiering: replace both `"scim_token"` with `"webhook_secret"`, `"super-secret-bearer"` with `"super-secret-value"` (three occurrences), the comment `(SCIM bearer token, recovery token)` with `(the webhook secret, the recovery token)`, and the message `"anonymous settings leaked the SCIM bearer token"` with `"anonymous settings leaked a stored secret"`.

- [ ] **Step 6: Tidy, build, test**

```bash
go mod tidy
grep -c 'elimity-com/scim\|scim2/filter-parser\|q-uint' go.mod   # expect 0
gofmt -l $(git ls-files '*.go') ; go vet ./... && go test -count=1 ./...
```

Expected: `0`, nothing from gofmt, all packages `ok`. `github.com/google/uuid` stays (testdb uses it).

- [ ] **Step 7: DOX pass**

- `internal/api/AGENTS.md`: Purpose no longer mentions SCIM; delete the "SCIM bearer token" wording from the `/api/settings` bullet (it becomes: "`db_driver` for any session, and `extra_settings` for admins only; ...").
- `internal/store/AGENTS.md`: Ownership lists `UserStore`, `SessionStore`, `AuditStore`, `SettingsStore`; in Local Contracts remove "device pairings" from the `CompletePasswordChange` bullet and delete the "MFA challenges and device pairings are consumed..." bullet's device half (keep: "MFA challenges are consumed with database state transitions that permit exactly one successful use.").
- `internal/config/AGENTS.md`: no SCIM mention exists; leave unchanged.
- `docs/RESTORE.md`: confirm no `SCIM` remains (`grep -n SCIM docs/RESTORE.md`).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "remove device pairing and SCIM

kyPulse has no phone clients and no inbound provisioning. Drops the
packages, routes, SCIM auth branch, store surface, tables and config."
```

---

### Task 3: Remove device pairing and SCIM (web)

**Files:**
- Delete: `web/src/components/QRPairingModal.tsx`, `web/src/pages/SCIMAdmin.tsx`
- Modify: `web/src/App.tsx`, `web/src/components/AppHeader.tsx`, `web/src/pages/Dashboard.tsx`, `web/browser/ui.spec.mjs`, `web/package.json`, `web/package-lock.json`, `web/AGENTS.md`, `web/dist/**`
- Test: `cd web && npm test && npm run build`; Playwright spec edited (CI runs it)

**Interfaces:**
- Consumes: Task 2 (the routes are gone, so the UI must not call them).
- Produces: nav tabs `dashboard`, `backup`, `settings` and the `AppHeader` shape Task 4 edits.

- [ ] **Step 1: Delete the components and the dependency**

```bash
git rm -q web/src/components/QRPairingModal.tsx web/src/pages/SCIMAdmin.tsx
cd web && npm uninstall qrcode @types/qrcode && cd ..
```

- [ ] **Step 2: Edit `web/src/App.tsx`**

Delete `import { SCIMAdmin } from './pages/SCIMAdmin';` and the line `{activeTab === 'scim' && <SCIMAdmin />}`.

- [ ] **Step 3: Replace `web/src/components/AppHeader.tsx`**

```tsx
import React from 'react';
import { LogOut, Settings as SettingsIcon, LayoutDashboard, Archive } from 'lucide-react';
import { ThemeSwitcher } from './ThemeSwitcher';

interface AppHeaderProps {
  appName: string;
  activeTab: string;
  onTabChange: (tab: string) => void;
  user: any;
  onLogout: () => void;
}

export const AppHeader: React.FC<AppHeaderProps> = ({ appName, activeTab, onTabChange, user, onLogout }) => {
  const navItems = [
    { id: 'dashboard', label: 'Overview', icon: LayoutDashboard },
    { id: 'backup', label: 'Backup', icon: Archive },
    { id: 'settings', label: 'Settings & DB', icon: SettingsIcon },
  ];

  return (
    <header className="app-header">
      <div className="app-brand">
        <img src="/app-icon.png" width={28} height={28} alt="" />
        <span>{appName || 'kyPulse'}</span>
      </div>

      <nav className="app-nav" aria-label="Primary">
        {navItems.map((item) => {
          const Icon = item.icon;
          const active = activeTab === item.id;
          return (
            <button
              key={item.id}
              onClick={() => onTabChange(item.id)}
              className={active ? 'ky-nav-item active' : 'ky-nav-item'}
              aria-current={active ? 'page' : undefined}
            >
              <Icon size={16} />
              <span>{item.label}</span>
            </button>
          );
        })}
      </nav>
      <div className="app-header-actions">
        <ThemeSwitcher />

        {user && (
          <div className="app-user">
            <div className="app-user-copy">
              <div style={{ fontWeight: 600, color: 'var(--ink-strong)' }}>{user.display_name || user.username}</div>
              <div style={{ fontSize: '11px', color: 'var(--ink)' }}>{user.role}</div>
            </div>
            <button
              className="btn-secondary app-logout"
              onClick={onLogout}
              title="Sign out"
              aria-label="Sign out"
            >
              <LogOut size={16} />
            </button>
          </div>
        )}
      </div>
    </header>
  );
};
```

- [ ] **Step 4: Edit `web/src/pages/Dashboard.tsx`**

Delete the whole `SCIM 2.0 Inbound Provisioning` card object (the second element of `cards`) and remove `Users` from the `lucide-react` import. Change the subtitle paragraph text to:

```tsx
          {settings?.app_name || 'kyPulse'} watches the Ky suite: health, logs and alerts land here.
```

Also change the `Welcome` line's fallbacks from `'Busnes.app'` to `'kyPulse'` in `App.tsx` (three occurrences: loading screen, `Login appName`, `AppHeader appName`).

- [ ] **Step 5: Edit `web/browser/ui.spec.mjs`**

Delete the pairing-dialog block, from the line `const pair = page.getByRole('button', { name: 'Pair Device' });` through `await expect(pair).toBeFocused();` inclusive (lines 60-76). In the cache regex, change `(api|scim|saml|browser-regression-uncached)` to `(api|saml|browser-regression-uncached)`. Rename the test to `'production CSP, worker, themes, keyboard and responsive shell'`.

- [ ] **Step 6: Build, test, commit dist**

```bash
cd web && npm test && npm run build && cd ..
grep -rl 'qrcode\|SCIM\|Pair Device' web/src web/dist/assets ; echo "exit=$?"
git status --short web/dist
```

Expected: 7 vitest tests pass, build succeeds, grep `exit=1`, dist shows changed assets. Playwright runs in CI; run it locally if Chromium is installed: `go build -o .browser/server ./cmd/server && cd web && npx playwright install chromium && npm run test:browser`.

- [ ] **Step 7: DOX pass**

`web/AGENTS.md`: Purpose drops "90-second ephemeral QR device pairing modals" and "administrative management panels"; delete the sentence "Pairing uses a native modal dialog for focus containment, Escape and focus restoration." from the service-worker bullet.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "web: drop the pairing modal and SCIM page

Nav is Overview, Backup, Settings. qrcode dependency removed; dist rebuilt."
```

---

### Task 4: Admin and viewer roles

**Files:**
- Modify: `internal/store/store.go`, `internal/store/sqlstore.go`, `internal/store/models.go`, `internal/store/migrations/migrations.go`, `internal/store/store_test.go`, `internal/sso/sso.go`, `internal/sso/kysignon.go`, `internal/sso/sso_test.go`, `internal/api/sso_handlers.go`, `internal/api/authz_test.go`, `internal/api/api_test.go`, `web/src/components/AppHeader.tsx`, `web/dist/**`, `internal/store/AGENTS.md`, `internal/sso/AGENTS.md`
- Test: `internal/store/store_test.go`, `internal/sso/sso_test.go`, `internal/api/authz_test.go`

**Interfaces:**
- Consumes: Task 2's store surface, Task 3's `AppHeader`.
- Produces: `store.RoleAdmin = "admin"`, `store.RoleViewer = "viewer"`, `store.ErrInvalidRole`, `sso.RoleFor(claimed string) string`.

- [ ] **Step 1: Write the failing store test.** Append to `internal/store/store_test.go`:

```go
func TestUserRoleIsAdminOrViewer(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	for _, role := range []string{"user", "manager", "", "Admin"} {
		err := st.Users().CreateUser(ctx, &store.User{ID: "usr_" + role, Username: "u" + role, Role: role, Status: "active"})
		if !errors.Is(err, store.ErrInvalidRole) {
			t.Errorf("CreateUser with role %q: got %v, want ErrInvalidRole", role, err)
		}
	}

	viewer := &store.User{ID: "usr_v", Username: "v", Role: store.RoleViewer, Status: "active"}
	if err := st.Users().CreateUser(ctx, viewer); err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	viewer.Role = "manager"
	if err := st.Users().UpdateUser(ctx, viewer); !errors.Is(err, store.ErrInvalidRole) {
		t.Errorf("UpdateUser to manager: got %v, want ErrInvalidRole", err)
	}
	viewer.Role = store.RoleAdmin
	if err := st.Users().UpdateUser(ctx, viewer); err != nil {
		t.Errorf("UpdateUser to admin: %v", err)
	}
}
```

Add `"errors"` to the test file's imports if it is not there.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/store/ -run TestUserRoleIsAdminOrViewer`
Expected: FAIL, build error `undefined: store.ErrInvalidRole`.

- [ ] **Step 3: Implement the role guard**

`internal/store/store.go`, after the `var (...)` error block:

```go
// Roles. A viewer sees status and alerts; an admin sees everything and changes settings.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// ErrInvalidRole is returned for any role other than RoleAdmin or RoleViewer.
var ErrInvalidRole = errors.New("role must be admin or viewer")

func validRole(role string) bool { return role == RoleAdmin || role == RoleViewer }
```

`internal/store/sqlstore.go`: as the first statement of both `CreateUser` and `UpdateUser`:

```go
	if !validRole(user.Role) {
		return ErrInvalidRole
	}
```

`internal/store/models.go`: the `Role` comment becomes `// RoleAdmin or RoleViewer`.

`internal/store/migrations/migrations.go`, migration 1, both dialects: `role ... NOT NULL DEFAULT 'viewer'`.

Replace every `Role: "user"` in test files with `Role: store.RoleViewer` (or `"viewer"` where the package is `store` itself):

```bash
grep -rln 'Role:\s*"user"\|"bob", "user"' --include='*_test.go' . 
```

Expected files: `internal/store/store_test.go`, `internal/api/api_test.go` (`TestLoginRejectsUnparseableStoredHash`), `internal/api/authz_test.go` (`loginAs(t, srv, st, "bob", "user")` twice → `"viewer"`), `internal/sso/sso_test.go` (`Role: "user"` in the webhook payload → `"viewer"`). Edit each by hand.

- [ ] **Step 4: Run the store tests**

Run: `go test ./internal/store/`
Expected: PASS.

- [ ] **Step 5: Write the failing SSO test.** Append to `internal/sso/sso_test.go`:

```go
func TestRoleForMapsUnknownRolesToViewer(t *testing.T) {
	cases := map[string]string{
		"admin":   "admin",
		"viewer":  "viewer",
		"user":    "viewer",
		"manager": "viewer",
		"":        "viewer",
		"Admin":   "viewer",
	}
	for claimed, want := range cases {
		if got := sso.RoleFor(claimed); got != want {
			t.Errorf("RoleFor(%q) = %q, want %q", claimed, got, want)
		}
	}
}
```

(The test file is package `sso_test`; check its imports and add `"github.com/Busnes-app/kypulse-server/internal/sso"` if missing.)

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./internal/sso/ -run TestRoleForMapsUnknownRolesToViewer`
Expected: FAIL, `undefined: sso.RoleFor`.

- [ ] **Step 7: Implement `RoleFor` and use it at every provisioning site**

`internal/sso/sso.go`, after the `IdentityClaims` type:

```go
// RoleFor maps a role asserted by an identity provider onto kyPulse's two roles. Only an
// exact "admin" grants admin; every other value, including a missing one, is a viewer, so a
// provider that speaks another product's role vocabulary cannot grant privilege by accident.
func RoleFor(claimed string) string {
	if claimed == store.RoleAdmin {
		return store.RoleAdmin
	}
	return store.RoleViewer
}
```

Add the `store` import to `sso.go` if it is not already imported there.

`internal/sso/kysignon.go`: replace

```go
		role := payload.Role
		if role == "" {
			role = "user"
		}
```

with

```go
		role := RoleFor(payload.Role)
```

`internal/api/sso_handlers.go` line 80: `Role: sso.RoleFor(claims.Role),`.

- [ ] **Step 8: Run the SSO and API tests**

Run: `go test ./internal/sso/ ./internal/api/`
Expected: PASS, including `TestPrivilegedEndpointsRequireAdmin` now proving a `viewer` gets 403 on every admin route.

- [ ] **Step 9: Hide admin-only tabs from viewers**

In `web/src/components/AppHeader.tsx`, replace the `navItems` constant with:

```tsx
  const navItems = [
    { id: 'dashboard', label: 'Overview', icon: LayoutDashboard, admin: false },
    { id: 'backup', label: 'Backup', icon: Archive, admin: true },
    { id: 'settings', label: 'Settings & DB', icon: SettingsIcon, admin: false },
  ].filter((item) => !item.admin || user?.role === 'admin');
```

The backup routes are admin-only server-side (`TestPrivilegedEndpointsRequireAdmin`); this only stops a viewer landing on a screen of 403s. In `web/src/App.tsx`, guard the render the same way: `{activeTab === 'backup' && user.role === 'admin' && <Backup />}`.

Rebuild: `cd web && npm test && npm run build && cd ..`.

- [ ] **Step 10: DOX pass**

- `internal/store/AGENTS.md` Local Contracts: add "- Users carry `RoleAdmin` or `RoleViewer`; `CreateUser` and `UpdateUser` return `ErrInvalidRole` for anything else. New users default to viewer."
- `internal/sso/AGENTS.md` Local Contracts: add "- Provider-asserted roles pass through `RoleFor`: exactly `admin` grants admin, everything else is a viewer."

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "roles: users are admin or viewer

Store refuses any other role; identity providers map through RoleFor;
viewers do not see the Backup tab. Dist rebuilt."
```

---

### Task 5: Structured logging and `/healthz`

**Files:**
- Create: `cmd/server/logging.go`, `cmd/server/logging_test.go`
- Modify: `cmd/server/main.go`, `internal/api/server.go`, `internal/api/api_test.go` (`setupTestServer`), `internal/api/authz_test.go` (add a healthz test), `internal/api/AGENTS.md`, `internal/config/AGENTS.md`, `scripts/smoke-test.sh`, `.github/workflows/ci.yml`
- Test: `cmd/server/logging_test.go`, `internal/api/authz_test.go`

**Interfaces:**
- Consumes: `logging.New`, `logging.Config`, `logging.FromEnv`, `(*logging.Logger).Handler()`; `health.Handler(service string, lg *logging.Logger, checks ...health.Check) http.Handler`, `health.Check{Name, Timeout, Run}`; `store.Store.Ping(ctx) error`.
- Produces: `api.NewServer(cfg *config.Config, st store.Store, lg *logging.Logger) *Server`; `newLogger(out io.Writer) (*logging.Logger, error)` in `cmd/server`; route `GET /healthz`.

- [ ] **Step 1: Write the failing log-bridge test** at `cmd/server/logging_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// Every line the process writes must be one JSON object: the stdlib log package and raw slog
// calls both route through the ky-primitives handler, and a newline inside a value cannot
// split a record, so a caller-controlled string cannot forge a log line.
func TestLogBridgeEmitsOneJSONLinePerCall(t *testing.T) {
	var buf bytes.Buffer
	if _, err := newLogger(&buf); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.SetOutput(nil); slog.SetDefault(slog.Default()) })

	log.Printf("[KYPULSE] listening on %s", "127.0.0.1:8080\nlevel=FATAL forged=1")
	slog.Warn("raw slog", "user_id", "u1", "password", "hunter2")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d:\n%s", len(lines), buf.String())
	}
	for i, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", i, err, l)
		}
		if m["app"] != "kypulse" {
			t.Errorf("line %d app = %v", i, m["app"])
		}
	}
	if strings.Contains(lines[0], "\n") || !strings.Contains(lines[0], "listening on 127.0.0.1:8080") {
		t.Errorf("stdlib line was not sanitised into one record: %s", lines[0])
	}
	if strings.Contains(lines[1], "hunter2") {
		t.Errorf("undeclared slog attribute reached the line: %s", lines[1])
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./cmd/server/ -run TestLogBridgeEmitsOneJSONLinePerCall`
Expected: FAIL, `undefined: newLogger`.

- [ ] **Step 3: Implement `cmd/server/logging.go`**

```go
package main

import (
	"io"
	"log"
	"log/slog"

	"github.com/Busnes-app/ky-primitives/logging"
)

// newLogger builds the process logger and routes the stdlib log package and the default slog
// logger through it, so every line on stderr is one sanitised JSON record. The scaffold's
// log.Printf call sites keep working; monitoring code written from here on uses declared
// events on the returned logger.
func newLogger(out io.Writer) (*logging.Logger, error) {
	cfg, err := logging.FromEnv() // KY_LOG_LEVEL, shared across the suite
	if err != nil {
		return nil, err
	}
	cfg.App = "kypulse"
	cfg.Out = out
	lg, err := logging.New(cfg)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(slog.New(lg.Handler()))
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(lg.Handler(), slog.LevelInfo).Writer())
	return lg, nil
}
```

- [ ] **Step 4: Run the test**

Run: `go test ./cmd/server/ -run TestLogBridgeEmitsOneJSONLinePerCall -v`
Expected: PASS.

- [ ] **Step 5: Write the failing healthz tests.** Append to `internal/api/authz_test.go`:

```go
func TestHealthzIsPublicAndReportsTheDatabase(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	w := do(t, srv, "GET", "/healthz", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Schema  string `json:"schema"`
		Service string `json:"service"`
		Status  string `json:"status"`
		Checks  []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Schema != "ky.health/1" || resp.Service != "kypulse" || resp.Status != "ok" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
	if len(resp.Checks) != 1 || resp.Checks[0].Name != "database" || resp.Checks[0].Status != "ok" {
		t.Errorf("checks = %+v", resp.Checks)
	}
}

func TestHealthzReportsDatabaseDown(t *testing.T) {
	srv, st, cfg := setupTestServer(t)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	w := do(t, srv, "GET", "/healthz", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte(`"name":"database","status":"down"`)) {
		t.Errorf("database check not down: %s", body)
	}
	for _, leak := range []string{cfg.Database.DSN, "sql: database is closed", cfg.Database.DataDir} {
		if leak != "" && bytes.Contains([]byte(body), []byte(leak)) {
			t.Errorf("healthz leaks %q: %s", leak, body)
		}
	}
}
```

`setupTestServer` already registers `st.Close()` in `t.Cleanup`; a second close of a `database/sql` handle returns nil, so the test's explicit close is safe.

- [ ] **Step 6: Run them to verify they fail**

Run: `go test ./internal/api/ -run 'TestHealthz'`
Expected: FAIL. `/healthz` falls through to the SPA handler and returns HTML, so `decode` fails / status is not 503.

- [ ] **Step 7: Wire the logger and the route**

`internal/api/server.go`:
- Add imports `"github.com/Busnes-app/ky-primitives/health"` and `"github.com/Busnes-app/ky-primitives/logging"`.
- Change the constructor signature to `func NewServer(cfg *config.Config, st store.Store, lg *logging.Logger) *Server` and add a field `lg *logging.Logger` to `Server`, set in the literal.
- In `routes()`, before the `// Auth` block:

```go
	// Liveness for monitors and readiness probes; public and cached by the lib. The only check
	// is the database, so the body never says more than "database down".
	s.mux.Handle("GET /healthz", health.Handler("kypulse", s.lg,
		health.Check{Name: "database", Run: s.store.Ping},
	))
```

`internal/api/api_test.go`, `setupTestServer`: build a logger that writes to `io.Discard` and pass it:

```go
	lg, err := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	srv := api.NewServer(cfg, st, lg)
```

with imports `"io"` and `"github.com/Busnes-app/ky-primitives/logging"`. The only other call is `internal/api/backup_test.go:689`, `api.NewServer(cfg, &blockingSessionStore{...})`: build the same `io.Discard` logger there (or reuse one from `setupTestServer` by returning it) and pass it as the third argument. Confirm with `grep -rn 'api.NewServer(' internal cmd`.

`cmd/server/main.go`:
- At the top of `main()`, before the subcommand switch:

```go
	if _, err := newLogger(os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
```

- In `runServer`, keep a handle: the call above installs the bridge but `NewServer` needs the logger value. Change `main()` to `lg, err := newLogger(os.Stderr)` and pass `lg` into `runServer(lg)`, whose signature becomes `func runServer(lg *logging.Logger)`; inside, `srv := api.NewServer(cfg, st, lg)`. Add the `logging` import to `main.go`.

- [ ] **Step 8: Run the whole suite**

```bash
gofmt -l $(git ls-files '*.go') ; go vet ./... && go test -race -count=1 ./...
```

Expected: nothing from gofmt, all `ok`.

- [ ] **Step 9: Smoke test and CI cover `/healthz`**

`scripts/smoke-test.sh`: after the first `start_server` and its readiness curl, add:

```bash
check "healthz is 200 when the database is up" "$(status "$BASE/healthz")" "200"
contains "healthz serves ky.health/1" "$(curl -s "$BASE/healthz")" '"schema":"ky.health/1"'
```

(`check`, `contains` and `status` are the script's existing helpers; place the two lines directly after `check "GET / serves the PWA" ...`.)

`.github/workflows/ci.yml`, job `docker`, step `Container serves the app`: after the `/api/settings` curl add

```yaml
          curl -sf http://127.0.0.1:18080/healthz | grep -q '"status":"ok"'
```

Run `shellcheck scripts/*.sh && go build -o kypulse ./cmd/server && scripts/smoke-test.sh`. Expected: every check passes, and the server log in the smoke output is JSON lines.

- [ ] **Step 10: DOX pass**

- `internal/api/AGENTS.md`: add to the route table area a line "- `GET /healthz` is public: `health.Handler("kypulse", lg, database ping)` from ky-primitives, `ky.health/1`, 200 for ok/degraded and 503 for down, cached 5 s by the lib. It is the route kyPulse's own monitor should watch." Update `NewServer`'s description if the doc mentions its signature.
- `internal/config/AGENTS.md`: add "- `KY_LOG_LEVEL` (not `KYPULSE_`) is read by `ky-primitives/logging` and sets the level for every line; it is the one suite-wide variable. Output is JSON lines on stderr; there is no log file."
- `README.md` already states both.

- [ ] **Step 11: Commit**

```bash
git add -A
git commit -m "healthz and structured stderr logging

GET /healthz via ky-primitives/health with a database check. All stdlib
log and slog output routes through ky-primitives/logging as JSON lines."
```

---

### Task 6: kyPulse DOX root and closeout

**Files:**
- Modify: `AGENTS.md` (repo root; currently an untracked copy of the suite root doc), `UI-VERIFICATION.md` (untracked; commit unchanged)
- Test: closeout greps; full local CI

**Interfaces:**
- Consumes: everything above.
- Produces: the DOX rail for steps 2b and 2c.

- [ ] **Step 1: Replace the root `AGENTS.md`**

The suite root (`/home/yoshi/git/busnes.app/AGENTS.md`) is this file's parent and already carries the engineering principles, the DOX framework text and the KyRecovery integration contract. This file must not duplicate them. Write:

````markdown
# kyPulse server

## Purpose

Unified health and logging for the Ky suite: polls every app's `/healthz`, pulls container
state and logs from KyYard, accepts logs from paired senders, shows status, alerts, activity
and logs, and sends outbound webhook alerts. Read-only towards the apps it watches.

Design authority: `docs/superpowers/specs/2026-09-26-kypulse-design.md`. Plans live in
`docs/superpowers/plans/`.

## Ownership

- `cmd/server/`: process entry, CLI subcommands (`init-admin`, `backup-drill`,
  `export-capsule`, `deposit`, `restore`, `version`), the log bridge, the backup scheduler.
- `internal/`: one package per concern; each has its own AGENTS.md.
- `web/`: the React PWA, embedded into the binary from `web/dist`.
- `scripts/`: smoke test.
- `docs/RESTORE.md`: restore runbook.

## Local Contracts

- Module `github.com/Busnes-app/kypulse-server`; binary `kypulse`; image
  `ghcr.io/busnes-app/kypulse-server`; every setting is `KYPULSE_*` except the suite-wide
  `KY_LOG_LEVEL`.
- Two roles, `admin` and `viewer`; the store refuses anything else and identity providers map
  through `sso.RoleFor`. Viewers see Status and Alerts; admins see everything.
- Every process line is a JSON record on stderr through `ky-primitives/logging`; there is no
  log file and no log socket.
- `GET /healthz` is public and is what an external monitor watches; kyPulse does not monitor
  itself.
- `web/dist` is committed after every frontend change; CI diffs it. `web/src/ky-ui/` is
  vendored and hash-pinned; never hand-edit it.
- No device pairing, no SCIM. Local login with MFA, SSO (KySignOn, OIDC, SAML metadata) and
  KyRecovery backup are kept from the scaffold.

## Work Guidance

- Follow the suite root's engineering principles and the KyRecovery integration contract.
- Business rules (health normalisation, alert transitions, retention) are pure functions the
  handlers and scheduler call.
- Outbound HTTP goes through `internal/egress` (planned in step 2b): private and LAN targets
  allowed, loopback and link-local refused, no redirects.

## Verification

- `make ci`: tidy check, gofmt, vet, race tests, frontend tests, smoke test.
- `.github/workflows/ci.yml` additionally runs Postgres tests, Playwright browser regressions,
  govulncheck, the Docker image smoke and the image-coordinate check.

## Child DOX Index

- `internal/api/AGENTS.md`: HTTP routes, auth wrappers, backup handlers, `/healthz`, SPA hosting.
- `internal/auth/AGENTS.md`: password policy, TOTP, recovery codes, sessions, PoW CAPTCHA.
- `internal/backup/AGENTS.md`: KyRecovery adapter over `ky-primitives/recoveryclient`.
- `internal/config/AGENTS.md`: `KYPULSE_*` loading, defaults, validation.
- `internal/crypto/AGENTS.md`: AES-GCM, HMAC, randomness.
- `internal/sso/AGENTS.md`: KySignOn, OIDC, SAML SP, `RoleFor`.
- `internal/store/AGENTS.md`: SQLite/Postgres DAL, migrations, roles.
- `internal/testdb/AGENTS.md`: isolated test databases.
- `web/AGENTS.md`: PWA, themes, ky-ui vendoring, browser regressions.
````

- [ ] **Step 2: Commit `UI-VERIFICATION.md` unchanged**

It records the ky-ui 0.2.0 vendor evidence `web/AGENTS.md` points at. Step 2c replaces it with kyPulse's own screens.

- [ ] **Step 3: Closeout greps and full local CI**

```bash
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -n 'ky_server_base\|ky-server-base\|KY-BASE\|scim\|SCIM\|devices/pair\|QRPairing\|Busnes.app Base'
git ls-files | grep -v '^web/dist/' | grep -v '^docs/superpowers/' | xargs grep -n '\bKY_' | grep -v 'KY_LOG_LEVEL'
ls internal
make ci
```

Expected: both greps print nothing; `ls internal` shows no `devices` or `scim`; `make ci` ends with `==> Local CI checks passed`.

- [ ] **Step 4: Commit**

```bash
git add AGENTS.md UI-VERIFICATION.md
git commit -m "docs: kyPulse DOX root"
```

- [ ] **Step 5: Push and open the PR. Ask Yoshi first**, because this is outward-facing.

```bash
git push -u origin feat/kypulse-scaffold
gh pr create --base master --title "kyPulse scaffold: rename, drop pairing and SCIM, roles, healthz, structured logs" --body-file - <<'EOF'
Step 2a of the kyPulse design (`docs/superpowers/specs/2026-09-26-kypulse-design.md`).

- Rename end to end: module `kypulse-server`, binary `kypulse`, image `ghcr.io/busnes-app/kypulse-server`, env prefix `KYPULSE_`.
- Device QR pairing and SCIM removed (packages, routes, tables, config, UI, deps).
- Users are `admin` or `viewer`; store-enforced; IdP roles map through `sso.RoleFor`.
- `GET /healthz` from ky-primitives/health v0.9.0 with a database check.
- All stderr output is JSON lines through ky-primitives/logging.
- Also carries the design spec and the two plans from `docs/kypulse-design`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
```

Register the PR with `link_pull_request` and drive CI to green with the `pull-request` skill.
