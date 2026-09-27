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
