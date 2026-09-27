package config_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/config"
)

func TestConfigLoadDefaults(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("expected default driver sqlite, got %s", cfg.Database.Driver)
	}
	if cfg.Captcha.Provider != "pow" {
		t.Errorf("expected default captcha provider pow, got %s", cfg.Captcha.Provider)
	}
	if cfg.Backup.Dir != "" {
		t.Errorf("expected empty default backup dir (sealed local copies off), got %q", cfg.Backup.Dir)
	}
}

func TestConfigLoadFromEnvOverrides(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	t.Setenv("KYPULSE_PORT", "9090")
	t.Setenv("KYPULSE_DB_DRIVER", "postgres")
	t.Setenv("KYPULSE_DB_DSN", "postgres://user:pass@localhost:5432/testdb")
	t.Setenv("KYPULSE_APP_NAME", "CustomBusnesApp")
	t.Setenv("KYPULSE_CAPTCHA_PROVIDER", "turnstile")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.Driver != "postgres" {
		t.Errorf("expected driver postgres, got %s", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "postgres://user:pass@localhost:5432/testdb" {
		t.Errorf("expected custom DSN, got %s", cfg.Database.DSN)
	}
	if cfg.Server.AppName != "CustomBusnesApp" {
		t.Errorf("expected custom app name, got %s", cfg.Server.AppName)
	}
	if cfg.Captcha.Provider != "turnstile" {
		t.Errorf("expected captcha provider turnstile, got %s", cfg.Captcha.Provider)
	}
}

func TestEncryptionKeyPersistsAcrossLoads(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	t.Setenv("KYPULSE_ENCRYPTION_KEY", "")
	a, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	b, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Security.EncryptionKey) != 32 || !bytes.Equal(a.Security.EncryptionKey, b.Security.EncryptionKey) {
		t.Fatal("encryption key was not persisted between loads")
	}
}

func TestEncryptionKeyFromEnvMustBe32Bytes(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	t.Setenv("KYPULSE_ENCRYPTION_KEY", "deadbeef")
	if _, err := config.LoadFromEnv(); err == nil {
		t.Fatal("8-byte key accepted")
	}
}

func TestDepositIntervalFromEnv(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	for _, tc := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"", 24 * time.Hour, true},
		{"90m", 90 * time.Minute, true},
		{"15m", 15 * time.Minute, true},
		{"0", 0, true},
		{"1s", 0, false},
		{"14m", 0, false},
		{"-1h", 0, false},
		{"daily", 0, false},
	} {
		t.Setenv("KYPULSE_BACKUP_DEPOSIT_INTERVAL", tc.in)
		cfg, err := config.LoadFromEnv()
		if (err == nil) != tc.ok {
			t.Errorf("%q: err=%v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if tc.ok && cfg.Backup.DepositInterval != tc.want {
			t.Errorf("%q: got %v, want %v", tc.in, cfg.Backup.DepositInterval, tc.want)
		}
	}
}

func TestBackupConfigFromEnv(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	t.Setenv("KYPULSE_BACKUP_DIR", "/tmp/x")
	t.Setenv("KYPULSE_BACKUP_KEEP", "3")
	t.Setenv("KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY", "true")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Dir != "/tmp/x" || cfg.Backup.Keep != 3 || !cfg.Backup.AllowPrivateRecovery {
		t.Fatalf("%+v", cfg.Backup)
	}
}

func TestBackupKeepBelowOneIsRefused(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	t.Setenv("KYPULSE_BACKUP_KEEP", "0")
	if _, err := config.LoadFromEnv(); err == nil || !strings.Contains(err.Error(), "KYPULSE_BACKUP_KEEP") {
		t.Fatalf("want KYPULSE_BACKUP_KEEP error, got %v", err)
	}
}

func TestAlertAndPollConfig(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alerts.AllowHTTP || cfg.Poll.Workers != 4 {
		t.Fatalf("defaults: %+v %+v", cfg.Alerts, cfg.Poll)
	}
	t.Setenv("KYPULSE_ALERT_ALLOW_HTTP", "true")
	t.Setenv("KYPULSE_POLL_WORKERS", "8")
	cfg, _ = config.LoadFromEnv()
	if !cfg.Alerts.AllowHTTP || cfg.Poll.Workers != 8 {
		t.Fatalf("set: %+v %+v", cfg.Alerts, cfg.Poll)
	}
	t.Setenv("KYPULSE_POLL_WORKERS", "0")
	if _, err := config.LoadFromEnv(); err == nil || !strings.Contains(err.Error(), "KYPULSE_POLL_WORKERS") {
		t.Fatalf("workers=0 must fail startup: %v", err)
	}
}
