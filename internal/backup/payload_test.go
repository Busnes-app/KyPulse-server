package backup_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/backup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestCollectExcludesCollectedDataAndPreservesState(t *testing.T) {
	ctx := context.Background()
	cfg, live := sqliteInstance(t)
	user := &store.User{ID: "backup_user", Username: "backup-user", Role: store.RoleAdmin, Status: "active", SSOProvider: "local"}
	if err := live.Users().CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := live.Targets().CreateTarget(ctx, &store.Target{ID: "backup_target", Name: "Backup Target", URL: "https://example.com/", IntervalSec: 30}); err != nil {
		t.Fatal(err)
	}
	if err := live.Settings().SetSetting(ctx, "backup_canary", "preserved"); err != nil {
		t.Fatal(err)
	}
	if err := live.Audit().LogAudit(ctx, &store.AuditRecord{UserID: user.ID, Action: "backup.test", Resource: "backup_target"}); err != nil {
		t.Fatal(err)
	}
	if err := live.Sources().CreateCode(ctx, "claimed-code", "backup_target", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	source, err := live.Sources().Claim(ctx, "claimed-code", "token-hash", "backup-host")
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Sources().CreateCode(ctx, "pending-code", "backup_target", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	marker := hex.EncodeToString(secret)
	now := time.Now().UTC()
	if err := live.Logs().Append(ctx, source.ID, store.LogBatch{
		Logs:     []store.LogLine{{Time: now, ReceivedAt: now, Raw: marker, Message: marker}},
		Activity: []store.Activity{{Time: now, ReceivedAt: now, Action: "imported." + marker}},
	}, 1<<20); err != nil {
		t.Fatal(err)
	}

	cursorMarker := "cursor-" + marker
	if err := live.Logs().AppendImported(ctx, store.LogBatch{}, store.LogCursor{Key: cursorMarker}, store.LogCursor{Key: cursorMarker, Value: cursorMarker}, 1<<20); err != nil {
		t.Fatal(err)
	}
	payload, err := backup.Collect(ctx, cfg, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	f := findFile(payload.Files, "data/kypulse.db")
	if f == nil {
		t.Fatal("missing database")
	}
	if bytes.Contains(f.Data, []byte(marker)) {
		t.Fatal("raw collected secret remains in SQLite payload")
	}
	path := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(path, f.Data, 0600); err != nil {
		t.Fatal(err)
	}
	copyStore, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: path, AuditKey: cfg.Database.AuditKey})
	if err != nil {
		t.Fatal(err)
	}
	defer copyStore.Close()
	for _, tc := range []struct {
		table string
		want  int
	}{{"log_cursors", 0}, {"log_lines", 0}, {"activity", 0}, {"log_pairing_codes", 0}, {"log_sources", 1}, {"targets", 1}, {"users", 1}, {"audit_records", 1}} {
		var got int
		if err := sqlQueryCount(path, tc.table, &got); err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("snapshot %s rows = %d, want %d", tc.table, got, tc.want)
		}
	}
	var usage int64
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRowContext(ctx, "SELECT bytes FROM log_usage WHERE id=1").Scan(&usage); err != nil {
		t.Fatal(err)
	}
	if usage != 0 {
		t.Errorf("snapshot log usage = %d", usage)
	}
	if _, err := copyStore.Users().GetUserByID(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := copyStore.Targets().GetTarget(ctx, "backup_target"); err != nil {
		t.Fatal(err)
	}
	if got, err := copyStore.Settings().GetSetting(ctx, "backup_canary"); err != nil || got != "preserved" {
		t.Fatalf("snapshot setting = %q, %v", got, err)
	}
	if got, err := copyStore.Sources().Authenticate(ctx, "token-hash"); err != nil || got.ID != source.ID {
		t.Fatalf("snapshot source = %+v, %v", got, err)
	}
	if chain, err := copyStore.Audit().VerifyChain(ctx); err != nil || chain.Count != 1 {
		t.Fatalf("snapshot audit chain = %+v, %v", chain, err)
	}
	if lines, err := live.Logs().List(ctx, store.LogFilter{}); err != nil || len(lines) != 1 {
		t.Fatalf("live logs = %+v, %v", lines, err)
	}
	if activity, err := live.Logs().ListActivity(ctx, store.ActivityFilter{}); err != nil || len(activity) != 1 {
		t.Fatalf("live activity = %+v, %v", activity, err)
	}
	if cur, err := live.Logs().Cursor(ctx, cursorMarker); err != nil || cur != cursorMarker {
		t.Fatalf("live cursor %q %v", cur, err)
	}
	var pending int
	if err := sqlQueryCount(cfg.Database.DSN, "log_pairing_codes", &pending); err != nil || pending != 1 {
		t.Fatalf("live pending codes = %d, %v", pending, err)
	}
}

func TestCollectRefusesFailedSnapshotSanitization(t *testing.T) {
	cfg, st := sqliteInstance(t)
	now := time.Now().UTC()
	if err := st.Logs().Append(context.Background(), "", store.LogBatch{Logs: []store.LogLine{{Time: now, ReceivedAt: now, Raw: "canary"}}}, 1024); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.Database.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TRIGGER refuse_log_delete BEFORE DELETE ON log_lines BEGIN SELECT RAISE(ABORT, 'refuse deletion'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = backup.Collect(context.Background(), cfg, "1.0.0")
	if !errors.Is(err, backup.ErrNoDatabaseSnapshot) {
		t.Fatalf("sanitization failure returned %v, want ErrNoDatabaseSnapshot", err)
	}
}

func sqlQueryCount(dsn, table string, count *int) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(count)
}

// payloadConfig is a real SQLite store in a temp data dir: the collectors snapshot the live
// database, so there has to be one.
func payloadConfig(t *testing.T) (*config.Config, []byte) {
	t.Helper()
	cfg, _ := sqliteInstance(t)
	return cfg, cfg.Security.EncryptionKey
}

// The encryption key must ride in the capsule: users.totp_secret_enc is AES-GCM under it, so
// a restore without it hands the operator a database whose MFA secrets are gone for good.
func TestCollectCarriesTheEncryptionKey(t *testing.T) {
	cfg, key := payloadConfig(t)
	payload, err := backup.Collect(context.Background(), cfg, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, f := range payload.Files {
		if f.Path != "data/encryption.key" {
			continue
		}
		found = true
		if want := hex.EncodeToString(key) + "\n"; string(f.Data) != want {
			t.Errorf("content: got %q, want the lowercase hex keyfile reads", f.Data)
		}
		if f.Mode != 0600 {
			t.Errorf("mode: got %o, want 600", f.Mode)
		}
	}
	if !found {
		t.Fatal("payload has no data/encryption.key")
	}
	req, _ := payload.VerificationRecipe["required_files"].([]string)
	if !slices.Contains(req, "data/encryption.key") {
		t.Errorf("required_files: got %v, want data/encryption.key among them", req)
	}
}

// A restore must come back paired, not half-paired: recovery.pub is public and the capsule is
// sealed to that very key, so it rides along whenever the instance has one.
func TestCollectCarriesTheRecoveryPublicKeyWhenPaired(t *testing.T) {
	cfg, _ := payloadConfig(t)
	ctx := context.Background()

	unpaired, err := backup.Collect(ctx, cfg, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if f := findFile(unpaired.Files, "data/recovery.pub"); f != nil {
		t.Error("unpaired instance shipped a recovery.pub")
	}
	if req, _ := unpaired.VerificationRecipe["required_files"].([]string); slices.Contains(req, "data/recovery.pub") {
		t.Errorf("unpaired required_files: got %v", req)
	}

	pubPath := recoveryclient.RecoveryKeyPath(cfg.Database.DataDir)
	if err := os.MkdirAll(filepath.Dir(pubPath), 0700); err != nil {
		t.Fatal(err)
	}
	pub := []byte("a-recovery-public-key")
	if err := os.WriteFile(pubPath, pub, 0600); err != nil {
		t.Fatal(err)
	}
	paired, err := backup.Collect(ctx, cfg, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	f := findFile(paired.Files, "data/recovery.pub")
	if f == nil {
		t.Fatal("paired instance has no data/recovery.pub in the payload")
	}
	if string(f.Data) != string(pub) {
		t.Error("data/recovery.pub is not the pinned public key byte for byte")
	}
	if f.Mode != 0600 {
		t.Errorf("mode: got %o, want 600", f.Mode)
	}
	if req, _ := paired.VerificationRecipe["required_files"].([]string); !slices.Contains(req, "data/recovery.pub") {
		t.Errorf("required_files: got %v, want data/recovery.pub among them", req)
	}
}

func findFile(files []recoveryclient.File, path string) *recoveryclient.File {
	for i := range files {
		if files[i].Path == path {
			return &files[i]
		}
	}
	return nil
}

func TestCollectRefusesAShortKey(t *testing.T) {
	cfg, _ := payloadConfig(t)
	cfg.Security.EncryptionKey = cfg.Security.EncryptionKey[:16]
	if _, err := backup.Collect(context.Background(), cfg, "1.0.0"); err == nil {
		t.Fatal("a 16-byte encryption key was accepted")
	}
}

// The audit chain key must ride in the capsule: without it a restored audit log cannot be
// verified against its hash chain.
func TestCollectCarriesTheAuditKey(t *testing.T) {
	cfg, _ := payloadConfig(t)
	payload, err := backup.Collect(context.Background(), cfg, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	f := findFile(payload.Files, "data/audit.key")
	if f == nil {
		t.Fatal("payload has no data/audit.key")
	}
	if want := hex.EncodeToString(cfg.Database.AuditKey) + "\n"; string(f.Data) != want {
		t.Errorf("content: got %q, want the lowercase hex keyfile reads", f.Data)
	}
	if f.Mode != 0600 {
		t.Errorf("mode: got %o, want 600", f.Mode)
	}
	req, _ := payload.VerificationRecipe["required_files"].([]string)
	if !slices.Contains(req, "data/audit.key") {
		t.Errorf("required_files: got %v, want data/audit.key among them", req)
	}
	if !slices.Contains(backup.Members(cfg), "data/audit.key") {
		t.Errorf("Members: got %v, want data/audit.key among them", backup.Members(cfg))
	}
}

func TestCollectRefusesAShortAuditKey(t *testing.T) {
	cfg, _ := payloadConfig(t)
	cfg.Database.AuditKey = cfg.Database.AuditKey[:16]
	if _, err := backup.Collect(context.Background(), cfg, "1.0.0"); err == nil {
		t.Fatal("a 16-byte audit key was accepted")
	}
}

// The store runs in WAL mode, so a plain read of the main file misses every commit still in
// the -wal. The snapshot must carry a row committed moments ago and never checkpointed.
func TestSnapshotSeesUncheckpointedCommit(t *testing.T) {
	cfg, st := sqliteInstance(t)
	ctx := context.Background()
	if err := st.Settings().SetSetting(ctx, "canary", "still-in-the-wal"); err != nil {
		t.Fatal(err)
	}
	payload, err := backup.Collect(ctx, cfg, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	f := findFile(payload.Files, "data/kypulse.db")
	if f == nil {
		t.Fatal("no data/kypulse.db in the payload")
	}
	restored := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(restored, f.Data, 0600); err != nil {
		t.Fatal(err)
	}
	copyStore, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: restored, AuditKey: cfg.Database.AuditKey})
	if err != nil {
		t.Fatalf("snapshot does not open: %v", err)
	}
	defer copyStore.Close()
	if got, err := copyStore.Settings().GetSetting(ctx, "canary"); err != nil || got != "still-in-the-wal" {
		t.Fatalf("snapshot lacks the uncheckpointed row: %q, %v", got, err)
	}
	if check, _ := payload.VerificationRecipe["check_sqlite_integrity"].(bool); !check {
		t.Error("recipe does not ask the drill to check the database")
	}
}

// A driver the collectors cannot snapshot must refuse, not seal a keys-and-config capsule
// that a receipt would then call a backup.
func TestCollectRefusesADriverItCannotSnapshot(t *testing.T) {
	cfg, _ := payloadConfig(t)
	cfg.Database.Driver = "postgres"
	if _, err := backup.Collect(context.Background(), cfg, "1.0.0"); !errors.Is(err, backup.ErrNoDatabaseSnapshot) {
		t.Fatalf("got %v, want ErrNoDatabaseSnapshot", err)
	}
}
