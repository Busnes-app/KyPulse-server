package main

import (
	"context"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

// `kypulse deposit` uses the installer's saved backup directory and retention, as the
// server does; with the environment defaults it would prune copies the policy keeps.
func TestDepositAppliesInstallerBackupSettings(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	dir := t.TempDir()
	b := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530",
		Backup: &applysetup.Backup{Directory: dir, Keep: 30}}
	if _, err := st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, false); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Backup: config.BackupConfig{Keep: 7}}
	_, _ = deposit(ctx, cfg, st) // the run itself fails here: no recovery key
	if cfg.Backup.Dir != dir || cfg.Backup.Keep != 30 {
		t.Fatalf("deposit ran with directory %q keep %d, want %q keep 30", cfg.Backup.Dir, cfg.Backup.Keep, dir)
	}
}
