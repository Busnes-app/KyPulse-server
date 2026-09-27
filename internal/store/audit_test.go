package store

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	if p := first.Audit().Placement(); p.Mode != "new" || p.Count != 0 || p.Head != genesisHash {
		t.Fatalf("fresh placement: %+v", p)
	}
	_ = first.Close() // opened, nothing logged: a restart must not read this as truncation
	second := openAudit(t, cfg)
	logN(t, second, 2)
	before, err := second.Audit().VerifyChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Close()
	third := openAudit(t, cfg)
	if p := third.Audit().Placement(); p.Mode != "resumed" || p.Count != 2 || p.Head != before.Head {
		t.Fatalf("reopen placement: %+v, want resumed at %+v", p, before)
	}
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

// insertUnkeyed writes audit rows the way the server did before the chain existed.
func insertUnkeyed(t *testing.T, st *SQLStore, actions ...string) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for _, action := range actions {
		if _, err := st.db.ExecContext(context.Background(), st.rebind(`INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at) VALUES (?, ?, ?, ?, ?, ?)`), "u1", action, "", "", "", now); err != nil {
			t.Fatal(err)
		}
	}
}

// unmigrate returns the schema to its state before migration 6, so the next open applies it.
func unmigrate(t *testing.T, st *SQLStore) {
	t.Helper()
	for _, stmt := range []string{
		"DROP INDEX idx_audit_seq",
		"ALTER TABLE audit_records DROP COLUMN seq",
		"ALTER TABLE audit_records DROP COLUMN prev_hash",
		"ALTER TABLE audit_records DROP COLUMN hash",
		"DELETE FROM schema_migrations WHERE version = 6",
	} {
		if _, err := st.db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func TestAuditLegacyRowsAreKeyedOnlyByMigration(t *testing.T) {
	ctx := context.Background()
	t.Run("upgrade", func(t *testing.T) {
		cfg := testdb.Config(t)
		st := openAudit(t, cfg)
		insertUnkeyed(t, st, "auth.login", "admin.target_create")
		unmigrate(t, st)
		_ = st.Close()
		st = openAudit(t, cfg)
		if mode := st.Audit().Placement().Mode; mode != "legacy_keyed" {
			t.Fatalf("placement mode %q, want legacy_keyed", mode)
		}
		status, err := st.Audit().VerifyChain(ctx)
		if err != nil || status.Count != 2 {
			t.Fatalf("legacy rows not keyed: %+v %v", status, err)
		}
		logN(t, st, 1)
		if status, err = st.Audit().VerifyChain(ctx); err != nil || status.Count != 3 {
			t.Fatalf("append after keying: %+v %v", status, err)
		}
	})
	t.Run("cleared chain is not rekeyed", func(t *testing.T) {
		cfg := testdb.Config(t)
		st := openAudit(t, cfg)
		logN(t, st, 3)
		if _, err := st.db.ExecContext(ctx, `UPDATE audit_records SET seq = NULL, prev_hash = '', hash = ''`); err != nil {
			t.Fatal(err)
		}
		if _, err := st.db.ExecContext(ctx, st.rebind(`DELETE FROM server_settings WHERE key = ?`), anchorKey); err != nil {
			t.Fatal(err)
		}
		_ = st.Close()
		if _, err := Open(ctx, cfg); !errors.Is(err, ErrAuditUnplaceable) {
			t.Fatalf("a keyed log stripped of its chain must refuse to open: %v", err)
		}
	})
}

// Two processes on one database: each must chain onto the other's records, not its own
// memory of the tail.
func TestAppendFollowsAnotherProcess(t *testing.T) {
	cfg := testdb.Config(t)
	a := openAudit(t, cfg)
	b := openAudit(t, cfg)
	logN(t, a, 1)
	logN(t, b, 1)
	logN(t, a, 1)
	for _, st := range []*SQLStore{a, b} {
		if status, err := st.Audit().VerifyChain(context.Background()); err != nil || status.Count != 3 {
			t.Fatalf("verify: %+v %v", status, err)
		}
	}
}

func TestParallelAppendsStayChained(t *testing.T) {
	st := openAudit(t, testdb.Config(t))
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 80)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				errs <- st.Audit().LogAudit(ctx, &AuditRecord{UserID: "u1", Action: "test.event", Resource: "r", IPAddress: "10.0.0.1"})
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if status, err := st.Audit().VerifyChain(ctx); err != nil || status.Count != 80 {
		t.Fatalf("verify: %+v %v", status, err)
	}
}

func TestAuditAppendAfterRolledBackAppend(t *testing.T) {
	ctx := context.Background()
	st := openAudit(t, testdb.Config(t))
	logN(t, st, 2)

	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	finish, err := st.audit.append(ctx, tx, &AuditRecord{UserID: "u1", Action: "test.event", Resource: "r", IPAddress: "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	finish(false)

	logN(t, st, 1)
	if status, err := st.Audit().VerifyChain(ctx); err != nil || status.Count != 3 {
		t.Fatalf("after a rolled-back append: %+v %v", status, err)
	}
}

func TestPasswordChangeRowIsChained(t *testing.T) {
	st := openAudit(t, testdb.Config(t))
	ctx := context.Background()
	u := &User{ID: "usr_1", Username: "a", DisplayName: "a", PasswordHash: "old", Role: RoleAdmin, Status: "active", SSOProvider: "local", MustChangePassword: true}
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

func TestAuditChainRefusesToOpenAfterMiddleDeletion(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	logN(t, st, 3)
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, st.rebind(`DELETE FROM audit_records WHERE seq = ?`), 2); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	if _, err := Open(ctx, cfg); !errors.Is(err, ErrAuditUnplaceable) {
		t.Fatalf("a record deleted from the middle must refuse to open: %v", err)
	}
}

func TestAuditChainRefusesEmptiedLog(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	logN(t, st, 2)
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, `DELETE FROM audit_records`); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	if _, err := Open(ctx, cfg); !errors.Is(err, ErrAuditUnplaceable) {
		t.Fatalf("an emptied log must refuse to open: %v", err)
	}
}

func TestAuditChainRefusesMixedRows(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	logN(t, st, 2)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := st.db.ExecContext(ctx, st.rebind(`INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at) VALUES (?, ?, ?, ?, ?, ?)`), "u1", "auth.login", "", "", "", now); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	if _, err := Open(ctx, cfg); !errors.Is(err, ErrAuditUnplaceable) {
		t.Fatalf("a keyed log with an unkeyed row mixed in must refuse to open: %v", err)
	}
}

// Ready must not flap under write load: a LogAudit committing while it places the chain
// must not make an intact log look unplaceable.
func TestReadyUnderConcurrentAppends(t *testing.T) {
	st := openAudit(t, testdb.Config(t))
	ctx := context.Background()

	var wg sync.WaitGroup
	var writeErrs, readyErrs [30]error

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range writeErrs {
			writeErrs[i] = st.Audit().LogAudit(ctx, &AuditRecord{UserID: "u1", Action: "test.event", Resource: "r", IPAddress: "10.0.0.1"})
		}
	}()
	go func() {
		defer wg.Done()
		for i := range readyErrs {
			readyErrs[i] = st.Audit().Ready(ctx)
		}
	}()
	wg.Wait()

	for i, err := range writeErrs {
		if err != nil {
			t.Fatalf("LogAudit %d: %v", i, err)
		}
	}
	for i, err := range readyErrs {
		if err != nil {
			t.Fatalf("Ready %d: %v", i, err)
		}
	}
}

func TestVerifyChainRefusesUnkeyedRows(t *testing.T) {
	st := openAudit(t, testdb.Config(t))
	logN(t, st, 2)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := st.db.ExecContext(ctx, st.rebind(`INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at) VALUES (?, ?, ?, ?, ?, ?)`), "u1", "auth.login", "", "", "", now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Audit().VerifyChain(ctx); err == nil {
		t.Fatal("an unkeyed row alongside a keyed chain must fail verification")
	}
}

func TestAuditRefusesTheWrongKey(t *testing.T) {
	cfg := testdb.Config(t)
	st := openAudit(t, cfg)
	logN(t, st, 2)
	_ = st.Close()
	cfg.AuditKey = make([]byte, 32)
	_, err := Open(context.Background(), cfg)
	if !errors.Is(err, ErrAuditUnplaceable) || !strings.Contains(err.Error(), "KYPULSE_AUDIT_KEY") {
		t.Fatalf("a foreign key must refuse to open and name the key: %v", err)
	}
}
