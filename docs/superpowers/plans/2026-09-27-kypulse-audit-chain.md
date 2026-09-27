# kyPulse Audit Chain (step 2d) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make kyPulse's own audit trail tamper-evident with `ky-primitives/auditchain`: every `audit_records` row is a keyed hash-chain link, an anchor outside the log catches truncation, a CLI verifies the whole chain, and `/healthz` reports when the chain cannot be appended to.

**Architecture:** The chain lives inside the store's `auditStore`, because every audit row already goes through it (`LogAudit` and the in-transaction `auth.password_changed` row). Each append runs `auditchain.Append` whose persist writes the row and the anchor in the same transaction; the in-memory chain is dropped after any failed write and re-resumed from the stored tail on the next append. Records are keyed by a 32-byte per-install key (`KYPULSE_AUDIT_KEY` or `<DataDir>/audit.key`), which the backup capsule carries beside the encryption key. A legacy log (rows without digests, no anchor) is keyed once on first start; a log that cannot be placed (records but no anchor, anchor but too few records, a tail that does not match the anchor) refuses to start, which is the suite's answer in kybookmarks and kypassword.

**Tech Stack:** Go 1.26, `ky-primitives` v0.9.0 (`auditchain`, `keyfile`, `health`), SQLite (modernc) and PostgreSQL (pgx) through the existing `rebind` layer, no new dependencies.

**Spec:** `docs/superpowers/specs/2026-09-26-kypulse-design.md` §2 Scaffold changes → "Own logs: … Its own audit trail uses `auditchain`", §1 (health check example `{"name":"audit",…}`), Testing. Suite precedent: `kybookmarks-server/internal/audit/audit.go` and `ky_server_base/docs/superpowers/plans/2026-09-02-audit-chain-convergence.md` (design decisions 1–5). KyRecovery contract (suite `AGENTS.md`): "Include every secret a restore needs."

## Global Constraints

- Chain key is exactly 32 bytes; `auditchain.New` refuses less (`ErrWeakKey`). Source: `keyfile.FromEnv("KYPULSE_AUDIT_KEY", 32)`, else `keyfile.LoadOrCreate(filepath.Join(dataDir, "audit.key"), 32)`. It is never the encryption key and never a constant.
- Hashed fields, in this order and nothing else: `created_at` as RFC3339Nano UTC, `user_id`, `action`, `resource`, `details`, `ip_address`. `created_at` is truncated to microseconds before hashing and storing, because PostgreSQL `TIMESTAMPTZ` keeps microseconds and a re-read value must reproduce the digest.
- Anchor lives in `server_settings` under key `audit_anchor` as JSON `{"count":N,"hash":"…"}`, written in the same transaction as the record.
- Existing behaviour that must survive: `s.audit` in `internal/api` still logs `audit_write_failed` and never fails the request; `CompletePasswordChange`/`ResetAdminPassword` stay atomic with their `auth.password_changed` row; `ListAuditRecords` keeps its order and paging; both dialects pass `go test -race ./...` (CI runs Postgres too).
- SQLite runs with one connection (`SetMaxOpenConns(1)`): any query issued while a transaction is open must run on that transaction, or it deadlocks.
- No new HTTP routes. New CLI subcommand `audit-verify`. `/healthz` gains the `audit` check; reason codes are declared with `health.DeclareReason`.
- Every env var is `KYPULSE_*`. Docs in this plan's last task; commits end with the trailer `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## Review Focus

1. **A PostgreSQL round trip of `created_at`** must reproduce the hashed string exactly; pinned by `TestAuditChainResumesAcrossOpen` (Task 2), which CI runs on both dialects.
2. **The one SQLite connection**: `append` runs `resume` reads on the caller's transaction; `keyLegacy` closes its read cursor before beginning its write transaction. Pinned by the Task 2 tests running on SQLite (a deadlock hangs them).
3. **A rolled-back transaction after a successful persist** (commit fails) leaves the in-memory chain ahead of disk; `forget()` after a failed commit makes the next append re-resume from the stored tail. Pinned by `TestAuditForgetResumesFromDisk` (Task 2).
4. **A fresh database with an anchor of count 0** (the store opened, nothing logged, restarted) must open as a new chain, not as "truncated". Pinned by `TestAuditChainResumesAcrossOpen` opening twice before the first row.
5. **A restore from an old capsule** brings `kypulse.db` and `audit.key` together, so the chain verifies; a capsule missing `audit.key` cannot verify after restore. Pinned by the payload test (Task 1) and `docs/RESTORE.md`.

---

### Task 1: Audit key in config and in the backup capsule

**Files:**
- Modify: `internal/config/config.go` (`DatabaseConfig`, `LoadFromEnv`)
- Modify: `internal/testdb/testdb.go` (`Config`, `postgresConfig`)
- Modify: `internal/backup/payload.go` (`Collect`, `Members`)
- Test: `internal/config/config_test.go`, `internal/backup/payload_test.go` (or the existing file that asserts `data/encryption.key`; find it with `grep -rln "encryption.key" internal/backup/*_test.go`)

**Interfaces:**
- Produces: `config.DatabaseConfig.AuditKey []byte` (json `-`), always 32 bytes after `LoadFromEnv` and after `testdb.Config`. `backup` seals `data/audit.key` (hex + newline, mode 0600) and lists it in `Members`.

- [ ] **Step 1: Write the failing config test**

Add to `internal/config/config_test.go`:

```go
func TestAuditKeyFromEnvOrFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KYPULSE_DATA_DIR", dir)
	t.Setenv("KYPULSE_AUDIT_KEY", "")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Database.AuditKey) != 32 {
		t.Fatalf("minted audit key is %d bytes", len(cfg.Database.AuditKey))
	}
	if _, err := os.Stat(filepath.Join(dir, "audit.key")); err != nil {
		t.Fatalf("audit.key not written: %v", err)
	}
	again, err := LoadFromEnv()
	if err != nil || !bytes.Equal(again.Database.AuditKey, cfg.Database.AuditKey) {
		t.Fatalf("second load did not reuse the file: %v", err)
	}

	t.Setenv("KYPULSE_AUDIT_KEY", strings.Repeat("ab", 32))
	cfg, err = LoadFromEnv()
	if err != nil || hex.EncodeToString(cfg.Database.AuditKey) != strings.Repeat("ab", 32) {
		t.Fatalf("env key not used: %v", err)
	}

	t.Setenv("KYPULSE_AUDIT_KEY", "short")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("a malformed KYPULSE_AUDIT_KEY must refuse to start")
	}
}
```

Add the imports the file lacks (`bytes`, `encoding/hex`, `os`, `path/filepath`, `strings`). If the file's other tests set `KYPULSE_DATA_DIR` themselves, follow their pattern.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/config/ -run TestAuditKeyFromEnvOrFile`
Expected: FAIL, `cfg.Database.AuditKey undefined`.

- [ ] **Step 3: Implement the config change**

In `internal/config/config.go`, `DatabaseConfig` gains:

```go
	// AuditKey keys the audit hash chain; 32 bytes, never serialised, never the encryption key.
	AuditKey []byte `json:"-"`
```

In `LoadFromEnv`, directly after the encryption-key block:

```go
	auditKey, ok, err := keyfile.FromEnv("KYPULSE_AUDIT_KEY", 32)
	if err != nil {
		return nil, fmt.Errorf("KYPULSE_AUDIT_KEY: %w", err)
	}
	if !ok {
		auditKey, err = keyfile.LoadOrCreate(filepath.Join(dataDir, "audit.key"), 32)
		if err != nil {
			return nil, fmt.Errorf("audit key: %w", err)
		}
	}
```

and in the `Database: DatabaseConfig{…}` literal add `AuditKey: auditKey,`.

- [ ] **Step 4: Give tests a key**

In `internal/testdb/testdb.go` add:

```go
// auditKey mints a per-test chain key; the store refuses to open without one.
func auditKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("testdb: audit key: %v", err)
	}
	return key
}
```

(import `crypto/rand`) and set `AuditKey: auditKey(t)` in both the SQLite literal in `Config` and the `config.DatabaseConfig` literal `postgresConfig` returns.

- [ ] **Step 5: Run config and testdb tests**

Run: `go test ./internal/config/ ./internal/testdb/`
Expected: PASS.

- [ ] **Step 6: Write the failing payload test**

In the backup test file that already asserts `data/encryption.key` is sealed, add a sibling assertion (or a new test) that the payload contains `data/audit.key` whose data is the hex of `cfg.Database.AuditKey` plus `\n`, and that `Members(cfg)` lists `data/audit.key`. Use the file's existing fixture for a config (it already sets `Security.EncryptionKey`; set `Database.AuditKey` to 32 bytes the same way). Also assert `Collect` returns an error when `Database.AuditKey` is not 32 bytes, mirroring the encryption-key check.

- [ ] **Step 7: Run it to verify it fails**

Run: `go test ./internal/backup/ -run 'Collect|Members'`
Expected: FAIL (no `data/audit.key` in the payload).

- [ ] **Step 8: Seal the audit key**

In `internal/backup/payload.go`:

```go
// auditKeyPath is where the restore puts the chain key: the <DataDir>/audit.key
// config.LoadFromEnv reads. Without it the restored audit log cannot be verified.
const auditKeyPath = "data/audit.key"
```

In `Collect`, after the encryption-key file is appended:

```go
	if len(cfg.Database.AuditKey) != 32 {
		return recoveryclient.Payload{}, fmt.Errorf("backup: audit key is %d bytes, want 32; refusing to seal a capsule whose audit log could not be verified after restore", len(cfg.Database.AuditKey))
	}
	files = append(files, recoveryclient.File{
		Path: auditKeyPath,
		Data: []byte(hex.EncodeToString(cfg.Database.AuditKey) + "\n"),
		Mode: 0600,
	})
```

In `Members`, the base list becomes `[]string{"data/kypulse.db", "config/settings.json", encryptionKeyPath, auditKeyPath}`.

- [ ] **Step 9: Run the backup tests and the drill**

Run: `go test ./internal/backup/ ./cmd/server/`
Expected: PASS (the drill's required-file list is derived from `Collect`, so it now requires `data/audit.key` automatically; if a drill test fixture enumerates files by hand, add the new path there).

- [ ] **Step 10: Commit**

```bash
git add internal/config internal/testdb internal/backup
git commit -m "config, backup: per-install audit chain key, sealed with the capsule"
```

---

### Task 2: The chain in the store

**Files:**
- Modify: `internal/store/migrations/migrations.go` (migration 6)
- Modify: `internal/store/models.go` (`AuditRecord`, `ChainStatus`)
- Modify: `internal/store/store.go` (`AuditStore` interface)
- Create: `internal/store/audit.go` (the whole `auditStore`; delete the old `auditStore` block from `sqlstore.go`)
- Modify: `internal/store/sqlstore.go` (`newSQLStore`, `revokePasswordGrants`, the two callers' commits)
- Modify: `internal/store/factory.go` (`Open` passes the key)
- Test: `internal/store/audit_test.go` (package `store`, internal)

**Interfaces:**
- Consumes: `config.DatabaseConfig.AuditKey` (Task 1).
- Produces: `AuditStore` gains `VerifyChain(ctx) (ChainStatus, error)` and `Ready(ctx) error`; `AuditRecord` gains `Seq uint64` (json `seq`) and `Hash string` (json `hash`); `ChainStatus{Count uint64; Head string}`; `store.ErrAuditUnplaceable` wraps refusals at open. Task 3 relies on all of these.

- [ ] **Step 1: Write the failing tests**

```go
// internal/store/audit_test.go
package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/auditchain"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func openAudit(t *testing.T, cfg config.DatabaseConfig) *SQLStore {
	t.Helper()
	st, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st.(*SQLStore)
}

func logN(t *testing.T, st *SQLStore, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := st.Audit().LogAudit(context.Background(), &AuditRecord{UserID: "u1", Action: "test.event", Resource: "r", Details: "i=" + string(rune('a'+i)), IPAddress: "10.0.0.1"}); err != nil {
			t.Fatalf("log %d: %v", i, err)
		}
	}
}

func TestAuditChainAppendsAndVerifies(t *testing.T) {
	st := openAudit(t, testdb.Config(t))
	logN(t, st, 3)
	status, err := st.Audit().VerifyChain(context.Background())
	if err != nil || status.Count != 3 {
		t.Fatalf("verify: %+v %v", status, err)
	}
	rows, _, err := st.Audit().ListAuditRecords(context.Background(), 0, 10)
	if err != nil || len(rows) != 3 {
		t.Fatalf("list: %d %v", len(rows), err)
	}
	if rows[0].Seq != 3 || len(rows[0].Hash) != 64 || rows[2].Seq != 1 {
		t.Fatalf("rows carry no chain: %+v", rows[0])
	}
	if err := st.Audit().Ready(context.Background()); err != nil {
		t.Fatalf("ready: %v", err)
	}
}

func TestAuditChainResumesAcrossOpen(t *testing.T) {
	cfg := testdb.Config(t)
	first := openAudit(t, cfg)
	_ = first.Close() // opened, nothing logged: a restart must not read this as truncation
	second := openAudit(t, cfg)
	logN(t, second, 2)
	_ = second.Close()
	third := openAudit(t, cfg)
	logN(t, third, 1)
	status, err := third.Audit().VerifyChain(context.Background())
	if err != nil || status.Count != 3 {
		t.Fatalf("after reopen: %+v %v", status, err)
	}
}

func TestAuditChainDetectsTamper(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	logN(t, st, 3)
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, st.rebind(`UPDATE audit_records SET details = 'edited' WHERE seq = ?`), 2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Audit().VerifyChain(ctx); !errors.Is(err, auditchain.ErrBrokenChain) {
		t.Fatalf("edited record not detected: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, st.rebind(`UPDATE audit_records SET details = ? WHERE seq = ?`), "i=b", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Audit().VerifyChain(ctx); err != nil {
		t.Fatalf("restored record should verify: %v", err)
	}
}

func TestAuditChainDetectsTruncationAndRefusesToOpen(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	logN(t, st, 3)
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, st.rebind(`DELETE FROM audit_records WHERE seq = ?`), 3); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Audit().VerifyChain(ctx); !errors.Is(err, auditchain.ErrTruncated) {
		t.Fatalf("truncation not detected: %v", err)
	}
	_ = st.Close()
	if _, err := Open(ctx, cfg); !errors.Is(err, ErrAuditUnplaceable) {
		t.Fatalf("a log shorter than its anchor must refuse to open: %v", err)
	}
}

func TestAuditChainRefusesToOpenWithoutAnchor(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	logN(t, st, 2)
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, st.rebind(`DELETE FROM server_settings WHERE key = ?`), anchorKey); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	if _, err := Open(ctx, cfg); !errors.Is(err, ErrAuditUnplaceable) {
		t.Fatalf("keyed records without an anchor must refuse to open: %v", err)
	}
}

func TestAuditLegacyRowsAreKeyedOnFirstOpen(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, action := range []string{"auth.login", "admin.target_create"} {
		if _, err := st.db.ExecContext(ctx, st.rebind(`INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at) VALUES (?, ?, ?, ?, ?, ?)`), "u1", action, "", "", "", now); err != nil {
			t.Fatal(err)
		}
	}
	_ = st.Close()
	st = openAudit(t, cfg)
	status, err := st.Audit().VerifyChain(ctx)
	if err != nil || status.Count != 2 {
		t.Fatalf("legacy rows not keyed: %+v %v", status, err)
	}
	logN(t, st, 1)
	if status, err = st.Audit().VerifyChain(ctx); err != nil || status.Count != 3 {
		t.Fatalf("append after keying: %+v %v", status, err)
	}
}

func TestAuditForgetResumesFromDisk(t *testing.T) {
	st := openAudit(t, testdb.Config(t))
	logN(t, st, 2)
	st.audit.forget()
	logN(t, st, 1)
	if status, err := st.Audit().VerifyChain(context.Background()); err != nil || status.Count != 3 {
		t.Fatalf("after forget: %+v %v", status, err)
	}
}

func TestPasswordChangeRowIsChained(t *testing.T) {
	st := openAudit(t, testdb.Config(t))
	ctx := context.Background()
	u := &User{ID: "usr_1", Username: "a", DisplayName: "a", PasswordHash: "old", Role: RoleAdmin, MustChangePassword: true}
	if err := st.Users().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().CompletePasswordChange(ctx, "usr_1", "old", "new", "10.0.0.9"); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := st.Audit().ListAuditRecords(ctx, 0, 5)
	if len(rows) != 1 || rows[0].Action != "auth.password_changed" || rows[0].Seq != 1 {
		t.Fatalf("password change row not chained: %+v", rows)
	}
	if _, err := st.Audit().VerifyChain(ctx); err != nil {
		t.Fatal(err)
	}
}
```

Check `User`'s field names against `internal/store/models.go` (`MustChangePassword`, `Role`, `PasswordHash`) and `CompletePasswordChange`'s signature in `store.go` before relying on them; adjust the fixture to the real names, not the other way round.

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/store/ -run 'TestAudit|TestPasswordChangeRowIsChained'`
Expected: FAIL to compile (`VerifyChain`, `ErrAuditUnplaceable`, `anchorKey`, `Seq` undefined).

- [ ] **Step 3: Migration 6**

Append to `registry` in `internal/store/migrations/migrations.go`:

```go
	{
		Version: 6,
		Name:    "audit_chain",
		SQLite: `
ALTER TABLE audit_records ADD COLUMN seq INTEGER;
ALTER TABLE audit_records ADD COLUMN prev_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_records ADD COLUMN hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_seq ON audit_records(seq);
`,
		Postgres: `
ALTER TABLE audit_records ADD COLUMN seq BIGINT;
ALTER TABLE audit_records ADD COLUMN prev_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE audit_records ADD COLUMN hash VARCHAR(64) NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_seq ON audit_records(seq);
`,
	},
```

- [ ] **Step 4: Models and interface**

`internal/store/models.go`: `AuditRecord` gains, after `CreatedAt`:

```go
	Seq  uint64 `json:"seq"`  // chain position, 1-based; 0 only on a row written before the chain
	Hash string `json:"hash"` // this record's digest; prev_hash is not exposed
```

and a new type:

```go
// ChainStatus is what VerifyChain reports about an intact audit chain.
type ChainStatus struct {
	Count uint64 `json:"count"`
	Head  string `json:"head"`
}
```

`internal/store/store.go`: `AuditStore` becomes

```go
// AuditStore logs security events as links of a keyed hash chain.
type AuditStore interface {
	LogAudit(ctx context.Context, r *AuditRecord) error
	ListAuditRecords(ctx context.Context, offset, limit int) ([]*AuditRecord, int, error)
	// VerifyChain walks every record against the anchor. It never writes.
	VerifyChain(ctx context.Context) (ChainStatus, error)
	// Ready reports whether the next append can chain onto the stored tail.
	Ready(ctx context.Context) error
}
```

and next to `ErrInvalidRole`:

```go
// ErrAuditUnplaceable reports an audit log the store cannot place at open: keyed records with
// no anchor, an anchor counting more records than exist, or a tail that is not the anchor's.
var ErrAuditUnplaceable = errors.New("audit log cannot be placed against its anchor")
```

- [ ] **Step 5: Write `internal/store/audit.go`**

Remove the `auditStore` type, `LogAudit` and `ListAuditRecords` from `sqlstore.go` (the block under `// Audit Store`) and create:

```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Busnes-app/ky-primitives/auditchain"
)

// anchorKey is the server_settings row holding the chain's count and head. It lives outside
// the log because hashes inside a table can never show that rows were deleted from its end.
const anchorKey = "audit_anchor"

// genesisHash is the predecessor of the first record, the anchor of an empty log.
const genesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// auditWriteBudget bounds one append. The audit write is not the caller's to cancel: a
// dropped connection must not lose the row for the action it already performed.
const auditWriteBudget = 10 * time.Second

// dbtx is what append and resume run their statements on: the pool at open, the caller's
// transaction otherwise. SQLite has one connection, so a read on the pool while a
// transaction is open would wait for itself.
type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type auditStore struct {
	store *SQLStore
	key   []byte

	mu    sync.Mutex        // serialises appends and guards chain
	chain *auditchain.Chain // nil until resumed, and again after a write the store could not confirm
}

// fieldsOf is the record content the chain authenticates. The order is the chain format:
// changing it invalidates every stored digest.
func fieldsOf(r *AuditRecord) []string {
	return []string{r.CreatedAt.UTC().Format(time.RFC3339Nano), r.UserID, r.Action, r.Resource, r.Details, r.IPAddress}
}

// stamp fixes the record time to what both dialects store, so a re-read reproduces the digest.
func stamp(r *AuditRecord) {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	r.CreatedAt = r.CreatedAt.UTC().Truncate(time.Microsecond)
}

// open places the chain at start: a new log, a legacy log keyed once, or the stored tail.
func (a *auditStore) open(ctx context.Context) error {
	if len(a.key) < 32 {
		return fmt.Errorf("audit: chain key is %d bytes, want 32", len(a.key))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.resume(ctx, a.store.db, true)
	return err
}

// forget drops the in-memory chain after a write the store did not confirm; the next append
// re-reads the tail from disk instead of chaining onto a record that may not exist.
func (a *auditStore) forget() {
	a.mu.Lock()
	a.chain = nil
	a.mu.Unlock()
}

func (a *auditStore) loadAnchor(ctx context.Context, q dbtx) (auditchain.Anchor, bool, error) {
	var raw string
	err := q.QueryRowContext(ctx, a.store.rebind("SELECT value FROM server_settings WHERE key = ?"), anchorKey).Scan(&raw)
	if errorsIs(err, sql.ErrNoRows) {
		return auditchain.Anchor{}, false, nil
	}
	if err != nil {
		return auditchain.Anchor{}, false, err
	}
	var anchor auditchain.Anchor
	if err := json.Unmarshal([]byte(raw), &anchor); err != nil {
		return auditchain.Anchor{}, false, fmt.Errorf("%w: anchor row does not decode", ErrAuditUnplaceable)
	}
	return anchor, true, nil
}

func (a *auditStore) writeAnchor(ctx context.Context, q dbtx, anchor auditchain.Anchor) error {
	raw, _ := json.Marshal(anchor)
	_, err := q.ExecContext(ctx, a.store.rebind(`
INSERT INTO server_settings (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`), anchorKey, string(raw), time.Now().UTC())
	return err
}

// tail reads the highest-sequence record as a chain record.
func (a *auditStore) tail(ctx context.Context, q dbtx) (auditchain.Record, error) {
	var r AuditRecord
	var prev string
	err := q.QueryRowContext(ctx, `SELECT seq, prev_hash, hash, user_id, action, resource, details, ip_address, created_at
FROM audit_records WHERE seq IS NOT NULL ORDER BY seq DESC LIMIT 1`).Scan(&r.Seq, &prev, &r.Hash, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt)
	if err != nil {
		return auditchain.Record{}, err
	}
	return auditchain.Record{Seq: r.Seq, Prev: prev, Hash: r.Hash, Fields: fieldsOf(&r)}, nil
}

// resume returns the chain, placing it from the store when it is not in memory. mu is held.
// pool says q is the connection pool, which keying a legacy log needs (it opens its own
// transaction); from inside a caller's transaction that would deadlock on SQLite.
func (a *auditStore) resume(ctx context.Context, q dbtx, pool bool) (*auditchain.Chain, error) {
	if a.chain != nil {
		return a.chain, nil
	}
	anchor, hasAnchor, err := a.loadAnchor(ctx, q)
	if err != nil {
		return nil, err
	}
	var total, unkeyed uint64
	if err := q.QueryRowContext(ctx, "SELECT COUNT(1), COUNT(1) - COUNT(seq) FROM audit_records").Scan(&total, &unkeyed); err != nil {
		return nil, err
	}
	switch {
	case total == 0 && (!hasAnchor || anchor.Count == 0):
		c, err := auditchain.New(a.key)
		if err != nil {
			return nil, err
		}
		a.chain = c
		return c, nil
	case total == 0:
		return nil, fmt.Errorf("%w: no audit records, but the anchor counts %d; the log was emptied", ErrAuditUnplaceable, anchor.Count)
	case !hasAnchor && unkeyed == total:
		if !pool {
			return nil, fmt.Errorf("%w: %d unkeyed records and no anchor appeared while running", ErrAuditUnplaceable, total)
		}
		return a.keyLegacy(ctx, q)
	case !hasAnchor:
		return nil, fmt.Errorf("%w: %d keyed audit records but no anchor row (%s); a truncated log cannot be told from an intact one, so this server will not start. Restore the row from backup, or move the records aside to begin a new chain and keep the old ones for the auditor", ErrAuditUnplaceable, total, anchorKey)
	case unkeyed > 0:
		return nil, fmt.Errorf("%w: %d audit records carry no digest although the chain is anchored", ErrAuditUnplaceable, unkeyed)
	}
	last, err := a.tail(ctx, q)
	if err != nil {
		return nil, err
	}
	c, err := auditchain.Resume(a.key, last, anchor)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAuditUnplaceable, err)
	}
	a.chain = c
	return c, nil
}

// keyLegacy digests a log written before the chain, in id order, and anchors it. It runs
// once, on the first start after the migration; every later row is chained at write time.
// q must be the pool: the rows are read to completion before the write transaction begins,
// which is what SQLite's single connection requires.
func (a *auditStore) keyLegacy(ctx context.Context, q dbtx) (*auditchain.Chain, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, user_id, action, resource, details, ip_address, created_at FROM audit_records ORDER BY id")
	if err != nil {
		return nil, err
	}
	var ids []int64
	var tuples [][]string
	for rows.Next() {
		var id int64
		var r AuditRecord
		if err := rows.Scan(&id, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		stamp(&r)
		ids = append(ids, id)
		tuples = append(tuples, fieldsOf(&r))
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	records, anchor, err := auditchain.Replay(a.key, tuples)
	if err != nil {
		return nil, err
	}
	tx, err := a.store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for i, rec := range records {
		if _, err := tx.ExecContext(ctx, a.store.rebind(`UPDATE audit_records SET seq = ?, prev_hash = ?, hash = ?, created_at = ? WHERE id = ?`),
			rec.Seq, rec.Prev, rec.Hash, mustTime(rec.Fields[0]), ids[i]); err != nil {
			return nil, err
		}
	}
	if err := a.writeAnchor(ctx, tx, anchor); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	c, err := auditchain.Resume(a.key, records[len(records)-1], anchor)
	if err != nil {
		return nil, err
	}
	a.chain = c
	return c, nil
}

// mustTime parses the RFC3339Nano stamp fieldsOf produced; it cannot fail for our own output.
func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic("audit: " + err.Error())
	}
	return t
}

// append chains r and writes it and the anchor on tx. The caller commits; if that commit
// fails it must call forget, because the chain has advanced past what the store holds.
func (a *auditStore) append(ctx context.Context, tx *sql.Tx, r *AuditRecord) error {
	stamp(r)
	a.mu.Lock()
	defer a.mu.Unlock()
	c, err := a.resume(ctx, tx, false)
	if err != nil {
		return err
	}
	rec, err := c.Append(ctx, func(rec auditchain.Record, anchor auditchain.Anchor) error {
		if _, err := tx.ExecContext(ctx, a.store.rebind(`
INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at, seq, prev_hash, hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
`), r.UserID, r.Action, r.Resource, r.Details, r.IPAddress, r.CreatedAt, rec.Seq, rec.Prev, rec.Hash); err != nil {
			return err
		}
		return a.writeAnchor(ctx, tx, anchor)
	}, fieldsOf(r)...)
	if err != nil {
		a.chain = nil
		return err
	}
	r.Seq, r.Hash = rec.Seq, rec.Hash
	return nil
}

func (a *auditStore) LogAudit(ctx context.Context, r *AuditRecord) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), auditWriteBudget)
	defer cancel()
	tx, err := a.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := a.append(ctx, tx, r); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		a.forget()
		return err
	}
	return nil
}

func (a *auditStore) Ready(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.resume(ctx, a.store.db, true)
	return err
}

func (a *auditStore) VerifyChain(ctx context.Context) (ChainStatus, error) {
	anchor, ok, err := a.loadAnchor(ctx, a.store.db)
	if err != nil {
		return ChainStatus{}, err
	}
	if !ok {
		anchor = auditchain.Anchor{Count: 0, Hash: genesisHash}
	}
	rows, err := a.store.db.QueryContext(ctx, `SELECT seq, prev_hash, hash, user_id, action, resource, details, ip_address, created_at
FROM audit_records ORDER BY seq`)
	if err != nil {
		return ChainStatus{}, err
	}
	defer rows.Close()
	records := func(yield func(auditchain.Record, error) bool) {
		for rows.Next() {
			var r AuditRecord
			var seq sql.NullInt64
			var prev string
			if err := rows.Scan(&seq, &prev, &r.Hash, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt); err != nil {
				yield(auditchain.Record{}, err)
				return
			}
			if !seq.Valid {
				yield(auditchain.Record{}, errors.New("record written before the chain was keyed"))
				return
			}
			if !yield(auditchain.Record{Seq: uint64(seq.Int64), Prev: prev, Hash: r.Hash, Fields: fieldsOf(&r)}, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(auditchain.Record{}, err)
		}
	}
	if err := auditchain.VerifyStream(a.key, records, anchor); err != nil {
		return ChainStatus{}, err
	}
	return ChainStatus{Count: anchor.Count, Head: anchor.Hash}, nil
}

func (a *auditStore) ListAuditRecords(ctx context.Context, offset, limit int) ([]*AuditRecord, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var count int
	if err := a.store.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM audit_records").Scan(&count); err != nil {
		return nil, 0, err
	}
	q := a.store.rebind(`
SELECT id, user_id, action, resource, details, ip_address, created_at, seq, hash
FROM audit_records
ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?
`)
	rows, err := a.store.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var records []*AuditRecord
	for rows.Next() {
		var r AuditRecord
		var seq sql.NullInt64
		if err := rows.Scan(&r.ID, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt, &seq, &r.Hash); err != nil {
			return nil, 0, err
		}
		if seq.Valid {
			r.Seq = uint64(seq.Int64)
		}
		records = append(records, &r)
	}
	return records, count, rows.Err()
}
```

Notes for the implementer: `errorsIs` already exists in `sqlstore.go`. On SQLite, `COUNT(1) - COUNT(seq)` scans into `uint64` fine; if the driver returns `int64` for Postgres and the scan complains, scan into `int64` and convert. The `ON CONFLICT(key)` upsert is the same one `SetSetting` uses, so both dialects accept it. Keep the `// Audit Store` banner comment out of `sqlstore.go` once the block is moved.

- [ ] **Step 6: Wire the key and the open**

`internal/store/sqlstore.go`, `newSQLStore(ctx, db, driver)` becomes `newSQLStore(ctx context.Context, db *sql.DB, driver string, auditKey []byte)`; after the stores are built:

```go
	s.audit = &auditStore{store: s, key: auditKey}
	…
	if err := s.audit.open(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
```

`internal/store/factory.go`: `openSQLite(ctx, cfg.DSN, cfg.AuditKey)` and `openPostgres(ctx, cfg.DSN, cfg.MaxOpenConns, cfg.MaxIdleConns, cfg.ConnMaxLifetime, cfg.AuditKey)`; both pass the key through to `newSQLStore`.

`revokePasswordGrants` replaces its raw `INSERT INTO audit_records …` with:

```go
	return u.store.audit.append(ctx, tx, &AuditRecord{UserID: userID, Action: "auth.password_changed", Resource: "user", Details: details, IPAddress: ip, CreatedAt: now})
```

and in both callers (`CompletePasswordChange`, `ResetAdminPassword`) the final `return tx.Commit()` becomes:

```go
	if err := tx.Commit(); err != nil {
		u.store.audit.forget()
		return err
	}
	return nil
```

Read both functions first: `revokePasswordGrants` must stay the last statement before `Commit`, so no other error path runs after the chain has advanced.

- [ ] **Step 7: Run the store tests on SQLite**

Run: `go test -race ./internal/store/`
Expected: PASS, including the pre-existing password tests.

- [ ] **Step 8: Run them on PostgreSQL**

Start a throwaway server and run the package against it:

```bash
docker run -d --rm --name kypulse-pg -e POSTGRES_USER=ci -e POSTGRES_PASSWORD=ci -e POSTGRES_DB=ci_test -p 55432:5432 postgres:17-alpine
sleep 3
KYPULSE_TEST_POSTGRES_DSN='postgres://ci:ci@127.0.0.1:55432/ci_test?sslmode=disable' go test -race -count=1 ./internal/store/
docker stop kypulse-pg
```

Expected: PASS. If `TestAuditChainResumesAcrossOpen` fails only here, the timestamp round trip is the cause: check `stamp` is applied before hashing and that `created_at` is written from the truncated value.

- [ ] **Step 9: Whole tree**

Run: `go build ./... && go test -race ./...`
Expected: PASS (every package that opens a store gets its key from `testdb.Config`).

- [ ] **Step 10: Commit**

```bash
git add internal/store
git commit -m "store: audit records are links of a keyed hash chain with an anchor"
```

---

### Task 3: Verify command, health check, smoke test, docs

**Files:**
- Modify: `cmd/server/main.go` (`main` switch, new `runAuditVerify`)
- Modify: `internal/api/server.go` (`/healthz` checks)
- Modify: `scripts/smoke-test.sh`
- Modify: `AGENTS.md`, `README.md`, `docs/RESTORE.md`, `internal/store/AGENTS.md`, `internal/config/AGENTS.md`, `internal/api/AGENTS.md`, `internal/backup/AGENTS.md`
- Test: `internal/api/server_test.go` or wherever `/healthz` is tested (`grep -rn healthz internal/api/*_test.go`)

**Interfaces:**
- Consumes: `store.AuditStore.VerifyChain`, `Ready`, `store.ChainStatus`, `store.ErrAuditUnplaceable` (Task 2).

- [ ] **Step 1: Write the failing healthz test**

In the test file that exercises `/healthz`, add:

```go
func TestHealthzReportsTheAuditCheck(t *testing.T) {
	srv, _ := newTestServer(t) // use the file's existing constructor
	w := do(t, srv, "GET", "/healthz", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz = %d", w.Code)
	}
	var body struct {
		Checks []struct{ Name, Status string } `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, c := range body.Checks {
		names[c.Name] = c.Status
	}
	if names["database"] != "ok" || names["audit"] != "ok" {
		t.Fatalf("checks: %+v", body.Checks)
	}
}
```

Adapt the constructor and request helper names to what the file already uses.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/api/ -run TestHealthzReportsTheAuditCheck`
Expected: FAIL (`audit` missing).

- [ ] **Step 3: Add the health check**

In `internal/api/server.go`, near the other `var` declarations:

```go
// reasonChainBroken is what /healthz says when the audit chain cannot be appended to: the
// stored tail no longer matches its anchor. Every audit write fails until it is repaired.
var reasonChainBroken = health.DeclareReason("chain_broken")
```

and the handler registration becomes:

```go
	// Liveness for monitors and readiness probes; public and cached by the lib. Two checks:
	// the database answers, and the audit chain can take the next record.
	s.mux.Handle("GET /healthz", health.Handler("kypulse", s.lg,
		health.Check{Name: "database", Run: s.store.Ping},
		health.Check{Name: "audit", Run: func(ctx context.Context) error {
			if err := s.store.Audit().Ready(ctx); err != nil {
				return health.Fail(reasonChainBroken)
			}
			return nil
		}},
	))
```

Run: `go test ./internal/api/`
Expected: PASS.

- [ ] **Step 4: The `audit-verify` subcommand**

In `cmd/server/main.go`, add to the switch before `case "version":`

```go
		case "audit-verify":
			runAuditVerify()
			return
```

and the function, next to `runDeposit`:

```go
// runAuditVerify walks the whole audit chain against its anchor and exits non-zero when any
// record was altered, reordered or removed. Run it after a restore and whenever the log is
// in doubt; the server only places the tail at start.
func runAuditVerify() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fatal("Failed to load configuration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	st, err := store.Open(ctx, cfg.Database)
	if err != nil {
		fatal("Failed to open database (%s): %v", cfg.Database.Driver, err)
	}
	defer st.Close()
	status, err := st.Audit().VerifyChain(ctx)
	if err != nil {
		fatal("Audit chain FAILED verification: %v", err)
	}
	fmt.Printf("audit chain verified: %d records, head %s\n", status.Count, status.Head)
}
```

`fatal` already exists in `cmd/server/logging.go` and exits 1. Update the `version` line's neighbour comment in `main`, if any lists subcommands.

- [ ] **Step 5: Smoke test**

In `scripts/smoke-test.sh`, after the server's graceful shutdown at the end of the HTTP section (find the last `stop_server` call), add:

```bash
echo "==> Audit chain"
VERIFY_OUT="$(KYPULSE_DATA_DIR="$WORK/data" KYPULSE_PORT="$PORT" KYPULSE_DB_DRIVER=sqlite "$BIN" audit-verify)"
contains "audit-verify walks the chain the server wrote" "$VERIFY_OUT" "audit chain verified"
check "audit-verify exits 0 on an intact chain" "$(KYPULSE_DATA_DIR="$WORK/data" KYPULSE_PORT="$PORT" KYPULSE_DB_DRIVER=sqlite "$BIN" audit-verify >/dev/null 2>&1 && echo 0 || echo 1)" "0"
```

Also assert in the `/healthz` section (there is a check for it already; extend it) that the body contains `"name":"audit"`.

Run: `make ci`
Expected: passes (the smoke test runs the binary end to end).

- [ ] **Step 6: Docs (DOX pass)**

- `internal/store/AGENTS.md` Local Contracts, add: "Audit rows are links of a keyed hash chain (`ky-primitives/auditchain`): `seq`, `prev_hash`, `hash` on the row, the anchor (`count`, `head`) in `server_settings.audit_anchor`, written in the same transaction. Fields hashed, in order: `created_at` (RFC3339Nano UTC, microsecond-truncated), `user_id`, `action`, `resource`, `details`, `ip_address`. `LogAudit` runs on a context detached from the caller with a 10 s budget. In-transaction audit rows (`revokePasswordGrants`) go through `audit.append` on the caller's transaction, and a failed commit calls `forget` so the next append re-reads the tail. `Open` refuses (`ErrAuditUnplaceable`) a log with keyed records and no anchor, an anchor counting more records than exist, or a tail that is not the anchor's; a log written before migration 6 (no digests, no anchor) is keyed once at first open. `VerifyChain` streams the whole log; `Ready` is what `/healthz` asks." Update the Ownership line to name `audit.go`.
- `internal/config/AGENTS.md`: after the encryption-key sentence add "The audit chain key comes from `KYPULSE_AUDIT_KEY` (32 bytes, hex or base64) or `<DataDir>/audit.key`, minted on first start; it is never the encryption key."
- `internal/api/AGENTS.md`: the `/healthz` bullet gains "and `audit` (`store.Audit().Ready`; `down` with reason `chain_broken` when the stored tail no longer matches its anchor)".
- `internal/backup/AGENTS.md`: the capsule members list gains `data/audit.key`.
- `docs/RESTORE.md`: add a row `| data/audit.key | 32 bytes. Keys the audit hash chain; without it the restored log cannot be verified |` to the members table, and a line after the restore steps: "Run `kypulse audit-verify` against the restored data directory; it walks every audit record against the anchor and exits 1 if the log was altered or shortened."
- `README.md`: env table gains `KYPULSE_AUDIT_KEY`; the CLI list gains `audit-verify`; one sentence under the security section: "kyPulse's own audit trail is a keyed hash chain; `kypulse audit-verify` checks it and `/healthz` reports `audit` down when it cannot be appended to."
- Root `AGENTS.md`: the `cmd/server/` bullet lists `audit-verify`; Local Contracts gains "The audit trail is a keyed hash chain (`internal/store/audit.go`); a log the store cannot place refuses to start."

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "audit-verify command, audit health check, smoke test and docs"
```
