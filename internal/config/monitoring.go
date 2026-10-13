package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// InstallerBundle extends the common envelope only for kyPulse-owned monitoring.
type InstallerBundle struct {
	applysetup.Common
	Monitoring *InstallerMonitoring `json:"monitoring,omitempty"`
}
type InstallerMonitoring struct {
	Targets []InstallerTarget `json:"targets"`
	Sources []InstallerSource `json:"sources,omitempty"`
}
type InstallerTarget struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	IntervalSec int    `json:"intervalSec,omitempty"`
	Container   string `json:"container,omitempty"`
}
type InstallerSource struct {
	Key               string `json:"key"`
	Name              string `json:"name"`
	Target            string `json:"target"`
	TokenFile         string `json:"tokenFile"`
	OffboardingAlerts bool   `json:"offboardingAlerts,omitempty"`
	tokenHash         string
}

func (s InstallerSource) TokenHash() string { return s.tokenHash }
func (t InstallerTarget) Interval() int {
	if t.IntervalSec == 0 {
		return 30
	}
	return t.IntervalSec
}

var installerKeyRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var installerTargetNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$`)

func (b InstallerBundle) Validate() error {
	if err := b.Common.Validate("kypulse", "/api/sso/kysignon/callback"); err != nil {
		return err
	}
	if b.Monitoring == nil {
		return nil
	}
	m := b.Monitoring
	if len(m.Targets) == 0 || len(m.Targets) > 32 || len(m.Sources) > 32 {
		return errors.New("monitoring requires 1-32 targets and at most 32 sources")
	}
	keys, names := map[string]bool{}, map[string]bool{}
	for _, t := range m.Targets {
		if !installerKeyRE.MatchString(t.Key) || keys[t.Key] || !installerTargetNameRE.MatchString(t.Name) || names[strings.ToLower(t.Name)] || len(t.URL) > 2048 || strings.TrimSpace(t.URL) != t.URL || t.Interval() < 10 || t.Interval() > 3600 || len(t.Container) > 128 || strings.ContainsAny(t.Container, "\x00\r\n") {
			return errors.New("invalid or duplicate monitoring target")
		}
		if err := egress.ValidateURL(t.URL, true); err != nil {
			return errors.New("invalid monitoring target URL")
		}
		keys[t.Key], names[strings.ToLower(t.Name)] = true, true
	}
	sourceKeys, sourceNames, files := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, s := range m.Sources {
		if !installerKeyRE.MatchString(s.Key) || sourceKeys[s.Key] || !installerKeyRE.MatchString(s.Name) || sourceNames[strings.ToLower(s.Name)] || !keys[s.Target] || !filepath.IsAbs(s.TokenFile) || filepath.Clean(s.TokenFile) != s.TokenFile || strings.ContainsAny(s.TokenFile, "\x00\r\n") || files[s.TokenFile] {
			return errors.New("invalid or duplicate monitoring source")
		}
		if b.SSO != nil && (s.TokenFile == b.SSO.ClientSecretFile || s.TokenFile == b.SSO.DirectorySecretFile) {
			return errors.New("monitoring source requires separate credentials")
		}
		sourceKeys[s.Key], sourceNames[strings.ToLower(s.Name)], files[s.TokenFile] = true, true, true
	}
	return nil
}

// ValidateFor applies the runtime's own origin before any credential file is read.
func (b InstallerBundle) ValidateFor(appURL string) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if b.Monitoring == nil {
		return nil
	}
	self, _ := url.Parse(appURL)
	for _, target := range b.Monitoring.Targets {
		u, _ := url.Parse(target.URL)
		if self != nil && self.Host != "" && u.Scheme == self.Scheme && strings.EqualFold(u.Host, self.Host) {
			return errors.New("kyPulse cannot monitor its own configured origin")
		}
	}
	return nil
}

// PrepareMonitoring reads private target files only after all public intent is valid.
// The prepared hashes are private Go fields and cannot enter a bundle or report.
func (b *InstallerBundle) PrepareMonitoring(cfg *Config) error {
	if err := b.ValidateFor(cfg.Server.AppURL); err != nil {
		return err
	}
	if b.Monitoring == nil {
		return nil
	}
	seen := map[string]bool{}
	for i := range b.Monitoring.Sources {
		s := &b.Monitoring.Sources[i]
		token, err := applysetup.Secret(s.TokenFile)
		decoded, decodeErr := hex.DecodeString(token)
		if err != nil || decodeErr != nil || len(decoded) != 32 || strings.ToLower(token) != token || token == cfg.SSO.KySignOnSecret || token == cfg.SSO.KySignOnHMACSecret {
			return errors.New("monitoring source requires its own private 32-byte hexadecimal token")
		}
		hash := sha256.Sum256([]byte(token))
		s.tokenHash = hex.EncodeToString(hash[:])
		if seen[s.tokenHash] {
			return errors.New("monitoring source credentials must be distinct")
		}
		seen[s.tokenHash] = true
	}
	return nil
}
