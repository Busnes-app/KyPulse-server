package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Busnes-app/ky-primitives/keyfile"
)

// Config encapsulates all runtime configuration for kyPulse.
type Config struct {
	Server   ServerConfig   `json:"server"`
	Database DatabaseConfig `json:"database"`
	Security SecurityConfig `json:"security"`
	SSO      SSOConfig      `json:"sso"`
	Backup   BackupConfig   `json:"backup"`
	Captcha  CaptchaConfig  `json:"captcha"`
	Alerts   AlertsConfig   `json:"alerts"`
	Poll     PollConfig     `json:"poll"`
	KyYard   KyYardConfig   `json:"kyyard"`
	Logs     LogsConfig     `json:"logs"`
}

// LogsConfig limits retained log and activity payload bytes.
type LogsConfig struct {
	MaxBytes int64 `json:"max_bytes"`
}

// ServerConfig defines HTTP and network settings.
type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	AppURL       string        `json:"app_url"`
	AppName      string        `json:"app_name"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	Environment  string        `json:"environment"`
}

// DatabaseConfig holds connection settings for pluggable storage (SQLite, PostgreSQL, MySQL).
type DatabaseConfig struct {
	Driver          string        `json:"driver"` // "sqlite", "postgres", "mysql"
	DSN             string        `json:"dsn"`    // Connection string or file path
	DataDir         string        `json:"data_dir"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	// AuditKey keys the audit hash chain; 32 bytes, never serialised, never the encryption key.
	AuditKey []byte `json:"-"`
	// AuditKeyPath is the keyfile AuditKey came from; empty when KYPULSE_AUDIT_KEY supplied it.
	AuditKeyPath string `json:"-"`
}

// AuditKeyBesideDB reports an audit keyfile in the SQLite database's own directory, where
// whoever can rewrite the database can usually read the key and re-sign the chain.
func (d DatabaseConfig) AuditKeyBesideDB() bool {
	if d.Driver != "sqlite" || d.AuditKeyPath == "" {
		return false
	}
	db, _, _ := strings.Cut(strings.TrimPrefix(d.DSN, "file:"), "?")
	keyDir, err := filepath.Abs(filepath.Dir(d.AuditKeyPath))
	if err != nil {
		return false
	}
	dbDir, err := filepath.Abs(filepath.Dir(db))
	return err == nil && keyDir == dbDir
}

// SecurityConfig holds encryption keys, cookie secrets, and session settings.
type SecurityConfig struct {
	SessionSecret string `json:"session_secret"`
	EncryptionKey []byte `json:"-"` // 32 bytes for AES-256-GCM; never serialised
	CookieSecure  bool   `json:"cookie_secure"`
	CookieDomain  string `json:"cookie_domain"`
	SessionTTL    time.Duration
	// TrustedProxies is the parsed KYPULSE_TRUSTED_PROXIES allowlist. Only a request whose peer
	// address falls inside it may speak for a client other than itself.
	TrustedProxies []netip.Prefix `json:"-"`
}

// SSOConfig holds identity provider and federation parameters.
type SSOConfig struct {
	Enabled             bool   `json:"enabled"`
	KySignOnIssuer      string `json:"kysignon_issuer"`
	KySignOnClientID    string `json:"kysignon_client_id"`
	KySignOnSecret      string `json:"kysignon_secret"`
	KySignOnHMACSecret  string `json:"kysignon_hmac_secret"`
	GenericOIDCIssuer   string `json:"generic_oidc_issuer"`
	GenericOIDCClientID string `json:"generic_oidc_client_id"`
	GenericOIDCSecret   string `json:"generic_oidc_secret"`
	SAMLEntityID        string `json:"saml_entity_id"`
	SAMLMetadataURL     string `json:"saml_metadata_url"`
	AutoProvision       bool   `json:"auto_provision"`
}

// BackupConfig holds parameters for KyBackup capsules & recovery drills.
type BackupConfig struct {
	Dir string `json:"dir"`
	// Keep is how many capsule generations recoveryclient retains; it refuses values below 1.
	Keep int `json:"keep"`
	// DepositInterval is how often a paired instance seals and deposits a capsule to
	// KyRecovery. Zero disables the schedule; deposits then happen only on request.
	DepositInterval time.Duration `json:"deposit_interval"`
	// AllowPrivateRecovery admits private and CGNAT KyRecovery destinations (HTTPS still
	// required). Off by default: KyRecovery destinations must be public.
	AllowPrivateRecovery bool `json:"allow_private_recovery"`
}

// CaptchaConfig holds anti-abuse settings (PoW default, Turnstile, Friendly).
type CaptchaConfig struct {
	Provider      string `json:"provider"` // "pow", "turnstile", "friendly", "none"
	SiteKey       string `json:"site_key"`
	SecretKey     string `json:"secret_key"`
	DifficultyPoW int    `json:"difficulty_pow"`
}

// AlertsConfig governs the outbound webhook.
type AlertsConfig struct {
	// AllowHTTP admits a plain-http webhook URL. Off by default: the webhook carries the
	// alert stream and, for Gotify and ntfy, a token in a header.
	AllowHTTP bool `json:"allow_http"`
}

// PollConfig sizes the health poller.
type PollConfig struct {
	Workers int `json:"workers"` // concurrent polls; one slow app never delays the rest
}

// KyYardConfig is the KyYard integration's process configuration; the pairing itself is a
// sealed setting.
type KyYardConfig struct {
	// AllowHTTP admits a plain-http KyYard URL. Off by default: the token rides in a header.
	AllowHTTP bool `json:"allow_http"`
}

// MinDepositInterval is the shortest schedule accepted: each run snapshots the whole database
// and uploads it, and KyRecovery admits 60 deposits per token per 15 minutes.
const MinDepositInterval = 15 * time.Minute

// DefaultAppName is the service name an unconfigured instance runs under. Capsules are sealed
// under it, so the restore CLI has to agree with it without loading a whole Config.
const DefaultAppName = "kyPulse"

// LoadFromEnv initializes a Config struct populated from environment variables with sensible defaults.
func LoadFromEnv() (*Config, error) {
	var ev envReader
	port := ev.int("KYPULSE_PORT", ev.int("PORT", 8080))
	host := getEnv("KYPULSE_HOST", "127.0.0.1")
	appURL := getEnv("KYPULSE_APP_URL", fmt.Sprintf("http://localhost:%d", port))
	appName := getEnv("KYPULSE_APP_NAME", DefaultAppName)
	env := getEnv("KYPULSE_ENV", "development")
	if env != "development" && env != "production" {
		return nil, fmt.Errorf("KYPULSE_ENV: must be development or production, got %q", env)
	}
	// Development mints a throwaway session secret and sends cookies without Secure; it is
	// for a single machine, not a network.
	allowInsecureDev := ev.bool("KYPULSE_ALLOW_INSECURE_DEV", false)
	if err := ev.err(); err != nil {
		return nil, err
	}
	if env == "development" && !isLoopback(host) && !allowInsecureDev {
		return nil, fmt.Errorf("KYPULSE_HOST %q is not loopback: set KYPULSE_ENV=production to serve a network (KYPULSE_ALLOW_INSECURE_DEV=true overrides)", host)
	}

	driver := strings.ToLower(getEnv("KYPULSE_DB_DRIVER", "sqlite"))
	dataDir := getEnv("KYPULSE_DATA_DIR", "./data")
	defaultDSN := fmt.Sprintf("%s/kypulse.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dataDir)
	if driver == "postgres" || driver == "postgresql" {
		driver = "postgres"
		defaultDSN = "" // no built-in credentials
	}
	dsn := getEnv("KYPULSE_DB_DSN", defaultDSN)
	if dsn == "" {
		return nil, errors.New("KYPULSE_DB_DSN is required with KYPULSE_DB_DRIVER=postgres")
	}

	sessionSecret := getEnv("KYPULSE_SESSION_SECRET", "")
	if sessionSecret == "" && env == "production" {
		key, err := keyfile.LoadOrCreate(filepath.Join(dataDir, "session.key"), 32)
		if err != nil {
			return nil, fmt.Errorf("session secret: %w", err)
		}
		sessionSecret = hex.EncodeToString(key)
	}
	if sessionSecret == "" {
		sessionSecret = generateRandomHex(32)
	}

	encryptionKey, ok, err := keyfile.FromEnv("KYPULSE_ENCRYPTION_KEY", 32)
	if err != nil {
		return nil, fmt.Errorf("KYPULSE_ENCRYPTION_KEY: %w", err)
	}
	if !ok {
		encryptionKey, err = keyfile.LoadOrCreate(filepath.Join(dataDir, "encryption.key"), 32)
		if err != nil {
			return nil, fmt.Errorf("encryption key: %w", err)
		}
	}

	auditKey, ok, err := keyfile.FromEnv("KYPULSE_AUDIT_KEY", 32)
	if err != nil {
		return nil, fmt.Errorf("KYPULSE_AUDIT_KEY: %w", err)
	}
	auditKeyPath := ""
	if !ok {
		auditKeyPath = resolveAuditKeyPath(dataDir)
		auditKey, err = keyfile.LoadOrCreate(auditKeyPath, 32)
		if err != nil {
			return nil, fmt.Errorf("audit key: %w", err)
		}
	}
	if bytes.Equal(auditKey, encryptionKey) {
		return nil, errors.New("KYPULSE_AUDIT_KEY must not equal KYPULSE_ENCRYPTION_KEY")
	}

	depositInterval, err := getEnvDuration("KYPULSE_BACKUP_DEPOSIT_INTERVAL", 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("KYPULSE_BACKUP_DEPOSIT_INTERVAL: %w", err)
	}
	if depositInterval != 0 && depositInterval < MinDepositInterval {
		return nil, fmt.Errorf("KYPULSE_BACKUP_DEPOSIT_INTERVAL: %s is below the %s minimum (0 disables)", depositInterval, MinDepositInterval)
	}

	backupKeep := ev.int("KYPULSE_BACKUP_KEEP", 7)
	if backupKeep < 1 {
		return nil, fmt.Errorf("KYPULSE_BACKUP_KEEP: must be at least 1, got %d", backupKeep)
	}

	trustedProxies, err := ParseTrustedProxies(getEnv("KYPULSE_TRUSTED_PROXIES", ""))
	if err != nil {
		return nil, fmt.Errorf("KYPULSE_TRUSTED_PROXIES: %w", err)
	}

	pollWorkers := ev.int("KYPULSE_POLL_WORKERS", 4)
	if pollWorkers < 1 || pollWorkers > 32 {
		return nil, fmt.Errorf("KYPULSE_POLL_WORKERS: must be 1..32, got %d", pollWorkers)
	}
	logMaxBytes := int64(1 << 30)
	if raw, ok := os.LookupEnv("KYPULSE_LOG_MAX_BYTES"); ok {
		logMaxBytes, err = strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("KYPULSE_LOG_MAX_BYTES: %w", err)
		}
	}
	if logMaxBytes < 128<<10 {
		return nil, fmt.Errorf("KYPULSE_LOG_MAX_BYTES: must be at least %d bytes", 128<<10)
	}

	cfg := &Config{
		Server: ServerConfig{
			Host:         host,
			Port:         port,
			AppURL:       strings.TrimRight(appURL, "/"),
			AppName:      appName,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			Environment:  env,
		},
		Database: DatabaseConfig{
			Driver:          driver,
			DSN:             dsn,
			DataDir:         dataDir,
			MaxOpenConns:    ev.int("KYPULSE_DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    ev.int("KYPULSE_DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: 15 * time.Minute,
			AuditKey:        auditKey,
			AuditKeyPath:    auditKeyPath,
		},
		Security: SecurityConfig{
			SessionSecret:  sessionSecret,
			EncryptionKey:  encryptionKey,
			CookieSecure:   ev.bool("KYPULSE_COOKIE_SECURE", env == "production" || strings.HasPrefix(strings.ToLower(appURL), "https://")),
			CookieDomain:   getEnv("KYPULSE_COOKIE_DOMAIN", ""),
			SessionTTL:     7 * 24 * time.Hour,
			TrustedProxies: trustedProxies,
		},
		SSO: SSOConfig{
			Enabled:             ev.bool("KYPULSE_SSO_ENABLED", true),
			KySignOnIssuer:      getEnv("KYPULSE_KYSIGNON_ISSUER", ""),
			KySignOnClientID:    getEnv("KYPULSE_KYSIGNON_CLIENT_ID", ""),
			KySignOnSecret:      getEnv("KYPULSE_KYSIGNON_SECRET", ""),
			KySignOnHMACSecret:  getEnv("KYPULSE_KYSIGNON_HMAC_SECRET", ""),
			GenericOIDCIssuer:   getEnv("KYPULSE_OIDC_ISSUER", ""),
			GenericOIDCClientID: getEnv("KYPULSE_OIDC_CLIENT_ID", ""),
			GenericOIDCSecret:   getEnv("KYPULSE_OIDC_SECRET", ""),
			SAMLEntityID:        getEnv("KYPULSE_SAML_ENTITY_ID", ""),
			SAMLMetadataURL:     getEnv("KYPULSE_SAML_METADATA_URL", ""),
			AutoProvision:       ev.bool("KYPULSE_SSO_AUTO_PROVISION", true),
		},
		Backup: BackupConfig{
			Dir:                  getEnv("KYPULSE_BACKUP_DIR", ""),
			Keep:                 backupKeep,
			DepositInterval:      depositInterval,
			AllowPrivateRecovery: ev.bool("KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY", false),
		},
		Captcha: CaptchaConfig{
			Provider:      getEnv("KYPULSE_CAPTCHA_PROVIDER", "pow"),
			SiteKey:       getEnv("KYPULSE_CAPTCHA_SITE_KEY", ""),
			SecretKey:     getEnv("KYPULSE_CAPTCHA_SECRET_KEY", ""),
			DifficultyPoW: ev.int("KYPULSE_CAPTCHA_POW_DIFFICULTY", 50000),
		},
		Alerts: AlertsConfig{AllowHTTP: ev.bool("KYPULSE_ALERT_ALLOW_HTTP", false)},
		Poll:   PollConfig{Workers: pollWorkers},
		KyYard: KyYardConfig{AllowHTTP: ev.bool("KYPULSE_KYYARD_ALLOW_HTTP", false)},
		Logs:   LogsConfig{MaxBytes: logMaxBytes},
	}
	if err := ev.err(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

// envReader collects unparsable values, so a typo such as KYPULSE_COOKIE_SECURE=ture fails
// startup instead of quietly reading as the default. A variable set but blank is unparsable.
type envReader struct{ errs []error }

func (r *envReader) int(key string, defaultVal int) int {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	n, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: %q is not an integer", key, val))
		return defaultVal
	}
	return n
}

func (r *envReader) bool(key string, defaultVal bool) bool {
	val, ok := os.LookupEnv(key)
	if !ok {
		return defaultVal
	}
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	r.errs = append(r.errs, fmt.Errorf("%s: %q is not true or false", key, val))
	return defaultVal
}

func (r *envReader) err() error { return errors.Join(r.errs...) }

// isLoopback reports a listen host reachable only from this machine.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

// resolveAuditKeyPath is KYPULSE_AUDIT_KEY_FILE, default <dataDir>/audit.key. While the
// configured file does not exist yet, a key already at the default path is used instead, so
// pointing the setting elsewhere never mints a second key over an existing chain.
func resolveAuditKeyPath(dataDir string) string {
	legacy := filepath.Join(dataDir, "audit.key")
	path := getEnv("KYPULSE_AUDIT_KEY_FILE", legacy)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Lstat(legacy); err == nil {
			return legacy
		}
	}
	return path
}

// getEnvDuration parses a Go duration such as "24h" or "90m". Negative is refused; "0" disables.
func getEnvDuration(key string, defaultVal time.Duration) (time.Duration, error) {
	val, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(val) == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(val))
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("%s is negative", val)
	}
	return d, nil
}

func generateRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ParseTrustedProxies parses a comma-separated list of IPs and CIDR blocks into prefixes.
// A bare IP becomes a single-address prefix. An unparsable entry is a startup error: a
// silently dropped proxy would make the server ignore its X-Forwarded-For and lump every
// client behind it into one rate-limit bucket.
func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if strings.Contains(field, "/") {
			prefix, err := netip.ParsePrefix(field)
			if err != nil {
				return nil, fmt.Errorf("invalid CIDR %q: %w", field, err)
			}
			// Checked after unmapping: a mapped form like ::ffff:0.0.0.0/96 has Bits() == 96
			// here but means 0.0.0.0/0 once rewritten, and would otherwise slip the guard.
			masked := unmapPrefix(prefix).Masked()
			if masked.Bits() == 0 {
				return nil, fmt.Errorf("trusted proxy %q trusts every address; list the proxy's own address or subnet instead", field)
			}
			out = append(out, masked)
			continue
		}
		addr, err := netip.ParseAddr(field)
		if err != nil {
			return nil, fmt.Errorf("invalid IP %q: %w", field, err)
		}
		addr = addr.Unmap()
		out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return out, nil
}

// unmapPrefix rewrites an IPv4-mapped IPv6 prefix as the plain IPv4 one it means. Peers are
// unmapped before the allowlist is consulted, and netip.Prefix.Contains never matches across
// families, so a mapped entry would otherwise match nothing at all.
func unmapPrefix(p netip.Prefix) netip.Prefix {
	if !p.Addr().Is4In6() || p.Bits() < 96 {
		return p
	}
	return netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
}
