package config_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
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
	if cfg.Logs.MaxBytes != 1<<30 {
		t.Fatalf("log cap = %d", cfg.Logs.MaxBytes)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("default host = %q, want loopback", cfg.Server.Host)
	}
	if cfg.Captcha.DifficultyPoW != 50000 {
		t.Errorf("default PoW difficulty = %d, want 50000", cfg.Captcha.DifficultyPoW)
	}
}

func TestUnparsableIntOrBoolFailsStartup(t *testing.T) {
	for key, raw := range map[string]string{
		"KYPULSE_COOKIE_SECURE":          "ture",
		"KYPULSE_SSO_AUTO_PROVISION":     "",
		"KYPULSE_ALLOW_INSECURE_DEV":     "y",
		"KYPULSE_PORT":                   "80a",
		"KYPULSE_CAPTCHA_POW_DIFFICULTY": "50k",
		"KYPULSE_POLL_WORKERS":           " ",
	} {
		t.Run(key, func(t *testing.T) {
			t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
			t.Setenv(key, raw)
			if _, err := config.LoadFromEnv(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("%s=%q: want a startup error naming it, got %v", key, raw, err)
			}
		})
	}
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	t.Setenv("KYPULSE_COOKIE_SECURE", "Off")
	t.Setenv("KYPULSE_APP_URL", "https://pulse.example")
	if cfg, err := config.LoadFromEnv(); err != nil || cfg.Security.CookieSecure {
		t.Fatalf("explicit off must parse and win: %v", err)
	}
}

func TestCookieSecureDefaults(t *testing.T) {
	for _, tc := range []struct {
		env, appURL string
		want        bool
	}{
		{"", "", false},
		{"", "http://localhost:8080", false},
		{"", "HTTPS://pulse.example", true},
		{"production", "http://localhost:8080", true},
	} {
		t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
		t.Setenv("KYPULSE_ENV", tc.env)
		t.Setenv("KYPULSE_APP_URL", tc.appURL)
		cfg, err := config.LoadFromEnv()
		if err != nil || cfg.Security.CookieSecure != tc.want {
			t.Fatalf("%+v: cfg=%v err=%v", tc, cfg != nil && cfg.Security.CookieSecure, err)
		}
	}
}

func TestDevelopmentListensOnlyOnLoopback(t *testing.T) {
	for _, tc := range []struct {
		env, host, override string
		ok                  bool
	}{
		{"", "127.0.0.1", "", true},
		{"", "::1", "", true},
		{"", "localhost", "", true},
		{"", "0.0.0.0", "", false},
		{"development", "192.168.1.5", "", false},
		{"", "0.0.0.0", "true", true},
		{"production", "0.0.0.0", "", true},
		{"prod", "127.0.0.1", "", false},
	} {
		t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
		t.Setenv("KYPULSE_ENV", tc.env)
		t.Setenv("KYPULSE_HOST", tc.host)
		if tc.override == "" {
			os.Unsetenv("KYPULSE_ALLOW_INSECURE_DEV")
		} else {
			t.Setenv("KYPULSE_ALLOW_INSECURE_DEV", tc.override)
		}
		if _, err := config.LoadFromEnv(); (err == nil) != tc.ok {
			t.Errorf("%+v: err=%v", tc, err)
		}
	}
}

func TestProductionSessionSecretPersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KYPULSE_DATA_DIR", dir)
	t.Setenv("KYPULSE_ENV", "production")
	t.Setenv("KYPULSE_SESSION_SECRET", "")
	a, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	b, err := config.LoadFromEnv()
	if err != nil || a.Security.SessionSecret != b.Security.SessionSecret || len(a.Security.SessionSecret) != 64 {
		t.Fatalf("production session secret not durable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "session.key")); err != nil {
		t.Fatalf("session.key not written: %v", err)
	}
	t.Setenv("KYPULSE_SESSION_SECRET", "explicit-session-secret-fixture")
	if c, err := config.LoadFromEnv(); err != nil || c.Security.SessionSecret != "explicit-session-secret-fixture" {
		t.Fatalf("explicit secret not used: %v", err)
	}
}

func TestPostgresRequiresDSN(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	t.Setenv("KYPULSE_DB_DRIVER", "postgres")
	if _, err := config.LoadFromEnv(); err == nil || !strings.Contains(err.Error(), "KYPULSE_DB_DSN") {
		t.Fatalf("postgres without a DSN must refuse to start: %v", err)
	}
}

func TestLogMaxBytesValidation(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	for _, tc := range []struct {
		raw   string
		want  int64
		valid bool
	}{
		{"131072", 131072, true}, {"131071", 0, false}, {"0", 0, false},
		{"-1", 0, false}, {"9223372036854775808", 0, false}, {"oops", 0, false}, {"", 0, false},
	} {
		t.Setenv("KYPULSE_LOG_MAX_BYTES", tc.raw)
		cfg, err := config.LoadFromEnv()
		if tc.valid {
			if err != nil || cfg.Logs.MaxBytes != tc.want {
				t.Fatalf("%q: cfg=%+v err=%v", tc.raw, cfg, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "KYPULSE_LOG_MAX_BYTES") {
			t.Fatalf("%q: expected log cap error, got %v", tc.raw, err)
		}
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

func TestAuditKeyFromEnvOrFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KYPULSE_DATA_DIR", dir)
	t.Setenv("KYPULSE_AUDIT_KEY", "")
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Database.AuditKey) != 32 {
		t.Fatalf("minted audit key is %d bytes", len(cfg.Database.AuditKey))
	}
	if _, err := os.Stat(filepath.Join(dir, "audit.key")); err != nil {
		t.Fatalf("audit.key not written: %v", err)
	}
	again, err := config.LoadFromEnv()
	if err != nil || !bytes.Equal(again.Database.AuditKey, cfg.Database.AuditKey) {
		t.Fatalf("second load did not reuse the file: %v", err)
	}

	t.Setenv("KYPULSE_AUDIT_KEY", strings.Repeat("ab", 32))
	cfg, err = config.LoadFromEnv()
	if err != nil || hex.EncodeToString(cfg.Database.AuditKey) != strings.Repeat("ab", 32) {
		t.Fatalf("env key not used: %v", err)
	}

	t.Setenv("KYPULSE_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	if _, err := config.LoadFromEnv(); err == nil || !strings.Contains(err.Error(), "must not equal") {
		t.Fatalf("an audit key equal to the encryption key must refuse to start: %v", err)
	}
	t.Setenv("KYPULSE_ENCRYPTION_KEY", "")

	t.Setenv("KYPULSE_AUDIT_KEY", "short")
	if _, err := config.LoadFromEnv(); err == nil {
		t.Fatal("a malformed KYPULSE_AUDIT_KEY must refuse to start")
	}
}

func TestAuditKeyFileKeptApartFromTheDatabase(t *testing.T) {
	data, secrets := t.TempDir(), t.TempDir()
	t.Setenv("KYPULSE_DATA_DIR", data)
	t.Setenv("KYPULSE_AUDIT_KEY", "")

	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Database.AuditKeyBesideDB() {
		t.Fatal("default <data>/audit.key must be reported as beside the database")
	}

	// An existing data/audit.key keeps being honoured after the setting moves.
	legacy := cfg.Database.AuditKey
	t.Setenv("KYPULSE_AUDIT_KEY_FILE", filepath.Join(secrets, "audit.key"))
	cfg, err = config.LoadFromEnv()
	if err != nil || !bytes.Equal(cfg.Database.AuditKey, legacy) || !cfg.Database.AuditKeyBesideDB() {
		t.Fatalf("legacy key not honoured: %v", err)
	}
	if _, err := os.Stat(filepath.Join(secrets, "audit.key")); !os.IsNotExist(err) {
		t.Fatalf("a second key was minted beside the legacy one: %v", err)
	}

	// Once moved, the configured file is used and no warning applies.
	if err := os.Rename(filepath.Join(data, "audit.key"), filepath.Join(secrets, "audit.key")); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.LoadFromEnv()
	if err != nil || !bytes.Equal(cfg.Database.AuditKey, legacy) || cfg.Database.AuditKeyBesideDB() {
		t.Fatalf("moved key not used: %v", err)
	}

	// A fresh install mints straight into the configured directory.
	fresh, freshSecrets := t.TempDir(), t.TempDir()
	t.Setenv("KYPULSE_DATA_DIR", fresh)
	t.Setenv("KYPULSE_AUDIT_KEY_FILE", filepath.Join(freshSecrets, "audit.key"))
	if cfg, err = config.LoadFromEnv(); err != nil || cfg.Database.AuditKeyBesideDB() {
		t.Fatalf("fresh install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh, "audit.key")); !os.IsNotExist(err) {
		t.Fatalf("fresh install wrote the key into the data dir: %v", err)
	}

	t.Setenv("KYPULSE_AUDIT_KEY", strings.Repeat("cd", 32))
	if cfg, err = config.LoadFromEnv(); err != nil || cfg.Database.AuditKeyPath != "" || cfg.Database.AuditKeyBesideDB() {
		t.Fatalf("env key: %v", err)
	}
}
