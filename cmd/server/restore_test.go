package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/capsule"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/ky-primitives/recoverykey"
	"github.com/Busnes-app/kypulse-server/internal/backup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func sealFixture(t *testing.T, service string) (string, []string) {
	t.Helper()
	priv, err := recoverykey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	shares, err := recoverykey.Split(priv, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	key := recoveryclient.RecoveryKey{Public: priv.Public(), Threshold: 2, TotalShares: 3}
	payload := recoveryclient.Payload{ServiceName: service, AppVersion: "1.0.0",
		Files: []recoveryclient.File{{Path: "data/x.db", Data: []byte("payload"), Mode: 0600}}}
	raw, _, err := recoveryclient.Seal(payload, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x.kycap")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// Non-consecutive indices: {1,2} would let an XOR-shaped bug pass.
	return path, []string{shares[0].String(), shares[2].String()}
}

func TestRestoreExtractsWithTwoShares(t *testing.T) {
	path, shares := sealFixture(t, "busnes_app")
	target := t.TempDir()
	var out bytes.Buffer
	if err := restore(path, target, "busnes_app", shares, &out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(target, "data", "x.db"))
	if err != nil || string(got) != "payload" {
		t.Fatalf("restored file: %q %v", got, err)
	}
	if !strings.Contains(out.String(), "busnes_app") {
		t.Fatalf("manifest not printed: %s", out.String())
	}
}

func TestRestoreRefusesAnotherService(t *testing.T) {
	path, shares := sealFixture(t, "someone_else")
	err := restore(path, t.TempDir(), "busnes_app", shares, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "someone_else") {
		t.Fatalf("got %v, want a service-name refusal naming the capsule's service", err)
	}
}

func TestRestoreRefusesTheWrongKit(t *testing.T) {
	path, _ := sealFixture(t, "busnes_app")
	_, otherShares := sealFixture(t, "busnes_app")
	err := restore(path, t.TempDir(), "busnes_app", otherShares, &bytes.Buffer{})
	if !errors.Is(err, capsule.ErrWrongRecoveryKey) {
		t.Fatalf("got %v, want ErrWrongRecoveryKey", err)
	}
}

func TestRestoreRefusesOneShare(t *testing.T) {
	path, shares := sealFixture(t, "busnes_app")
	if err := restore(path, t.TempDir(), "busnes_app", shares[:1], &bytes.Buffer{}); err == nil {
		t.Fatal("one share of a 2-of-3 kit was accepted")
	}
}

// KyNotes Q9 A: a restored capsule does not deposit on its own.
func TestRestoreTurnsScheduledDepositsOff(t *testing.T) {
	ctx := context.Background()
	live := filepath.Join(t.TempDir(), "kypulse.db")
	st, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: live, AuditKey: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Settings().SetSetting(ctx, "backup_interval_sec", "86400"); err != nil {
		t.Fatal(err)
	}
	if err := st.Settings().SetSetting(ctx, backup.LastRunErrorKey, "the old server's failure"); err != nil {
		t.Fatal(err)
	}
	st.Close()
	raw, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := recoverykey.Generate()
	shares, _ := recoverykey.Split(priv, 2, 3)
	sealed, _, err := recoveryclient.Seal(recoveryclient.Payload{ServiceName: "busnes_app", AppVersion: "1.0.0",
		Files: []recoveryclient.File{{Path: "data/kypulse.db", Data: raw, Mode: 0600}}},
		recoveryclient.RecoveryKey{Public: priv.Public(), Threshold: 2, TotalShares: 3})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x.kycap")
	if err := os.WriteFile(path, sealed, 0600); err != nil {
		t.Fatal(err)
	}
	// RESTORE.md restores into ./restored: a relative target.
	base := t.TempDir()
	t.Chdir(base)
	target := filepath.Join(base, "restored")
	var out bytes.Buffer
	if err := restore(path, "restored", "busnes_app", []string{shares[0].String(), shares[2].String()}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Scheduled deposits are off") {
		t.Fatalf("restore did not say so: %s", out.String())
	}
	st, err = store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(target, "data", "kypulse.db"), AuditKey: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if v, err := st.Settings().GetSetting(ctx, "backup_interval_sec"); err != nil || v != "0" {
		t.Fatalf("restored schedule: %q %v, want 0 (off)", v, err)
	}
	if _, err := st.Settings().GetSetting(ctx, backup.LastRunErrorKey); err == nil {
		t.Fatal("restored server shows the old server's backup failure")
	}
}
