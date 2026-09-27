package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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

// MinDepositInterval is the shortest schedule accepted: each run snapshots the whole database
// and uploads it, and KyRecovery admits 60 deposits per token per 15 minutes.
const MinDepositInterval = 15 * time.Minute

// DefaultAppName is the service name an unconfigured instance runs under. Capsules are sealed
// under it, so the restore CLI has to agree with it without loading a whole Config.
const DefaultAppName = "kyPulse"

// LoadFromEnv initializes a Config struct populated from environment variables with sensible defaults.
func LoadFromEnv() (*Config, error) {
	port := getEnvInt("KYPULSE_PORT", getEnvInt("PORT", 8080))
	host := getEnv("KYPULSE_HOST", "0.0.0.0")
	appURL := getEnv("KYPULSE_APP_URL", fmt.Sprintf("http://localhost:%d", port))
	appName := getEnv("KYPULSE_APP_NAME", DefaultAppName)
	env := getEnv("KYPULSE_ENV", "development")

	driver := strings.ToLower(getEnv("KYPULSE_DB_DRIVER", "sqlite"))
	dataDir := getEnv("KYPULSE_DATA_DIR", "./data")
	defaultDSN := fmt.Sprintf("%s/kypulse.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dataDir)
	if driver == "postgres" || driver == "postgresql" {
		driver = "postgres"
		defaultDSN = "postgres://postgres:postgres@localhost:5432/kypulse?sslmode=disable"
	}
	dsn := getEnv("KYPULSE_DB_DSN", defaultDSN)

	sessionSecret := getEnv("KYPULSE_SESSION_SECRET", "")
	if env == "production" && sessionSecret == "" {
		return nil, fmt.Errorf("KYPULSE_SESSION_SECRET is required in production")
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

	depositInterval, err := getEnvDuration("KYPULSE_BACKUP_DEPOSIT_INTERVAL", 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("KYPULSE_BACKUP_DEPOSIT_INTERVAL: %w", err)
	}
	if depositInterval != 0 && depositInterval < MinDepositInterval {
		return nil, fmt.Errorf("KYPULSE_BACKUP_DEPOSIT_INTERVAL: %s is below the %s minimum (0 disables)", depositInterval, MinDepositInterval)
	}

	backupKeep := getEnvInt("KYPULSE_BACKUP_KEEP", 7)
	if backupKeep < 1 {
		return nil, fmt.Errorf("KYPULSE_BACKUP_KEEP: must be at least 1, got %d", backupKeep)
	}

	trustedProxies, err := ParseTrustedProxies(getEnv("KYPULSE_TRUSTED_PROXIES", ""))
	if err != nil {
		return nil, fmt.Errorf("KYPULSE_TRUSTED_PROXIES: %w", err)
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
			MaxOpenConns:    getEnvInt("KYPULSE_DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("KYPULSE_DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: 15 * time.Minute,
		},
		Security: SecurityConfig{
			SessionSecret:  sessionSecret,
			EncryptionKey:  encryptionKey,
			CookieSecure:   getEnvBool("KYPULSE_COOKIE_SECURE", env == "production"),
			CookieDomain:   getEnv("KYPULSE_COOKIE_DOMAIN", ""),
			SessionTTL:     7 * 24 * time.Hour,
			TrustedProxies: trustedProxies,
		},
		SSO: SSOConfig{
			Enabled:             getEnvBool("KYPULSE_SSO_ENABLED", true),
			KySignOnIssuer:      getEnv("KYPULSE_KYSIGNON_ISSUER", ""),
			KySignOnClientID:    getEnv("KYPULSE_KYSIGNON_CLIENT_ID", ""),
			KySignOnSecret:      getEnv("KYPULSE_KYSIGNON_SECRET", ""),
			KySignOnHMACSecret:  getEnv("KYPULSE_KYSIGNON_HMAC_SECRET", ""),
			GenericOIDCIssuer:   getEnv("KYPULSE_OIDC_ISSUER", ""),
			GenericOIDCClientID: getEnv("KYPULSE_OIDC_CLIENT_ID", ""),
			GenericOIDCSecret:   getEnv("KYPULSE_OIDC_SECRET", ""),
			SAMLEntityID:        getEnv("KYPULSE_SAML_ENTITY_ID", ""),
			SAMLMetadataURL:     getEnv("KYPULSE_SAML_METADATA_URL", ""),
			AutoProvision:       getEnvBool("KYPULSE_SSO_AUTO_PROVISION", true),
		},
		Backup: BackupConfig{
			Dir:                  getEnv("KYPULSE_BACKUP_DIR", ""),
			Keep:                 backupKeep,
			DepositInterval:      depositInterval,
			AllowPrivateRecovery: getEnvBool("KYPULSE_BACKUP_ALLOW_PRIVATE_RECOVERY", false),
		},
		Captcha: CaptchaConfig{
			Provider:      getEnv("KYPULSE_CAPTCHA_PROVIDER", "pow"),
			SiteKey:       getEnv("KYPULSE_CAPTCHA_SITE_KEY", ""),
			SecretKey:     getEnv("KYPULSE_CAPTCHA_SECRET_KEY", ""),
			DifficultyPoW: getEnvInt("KYPULSE_CAPTCHA_POW_DIFFICULTY", 4),
		},
	}

	return cfg, nil
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val, ok := os.LookupEnv(key); ok {
		lower := strings.ToLower(strings.TrimSpace(val))
		return lower == "true" || lower == "1" || lower == "yes" || lower == "on"
	}
	return defaultVal
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
