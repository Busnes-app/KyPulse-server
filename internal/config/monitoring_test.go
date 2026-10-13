package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

func TestMonitoringIntentAndPrivateSourceCredentials(t *testing.T) {
	for _, mode := range []string{"valid", "unknown-target", "duplicate-target", "invalid-target-url", "invalid-interval", "public-token", "symlink-token", "invalid-token", "same-token", "active-sso-token", "self"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "token")
			token := strings.Repeat("ab", 32)
			if err := os.WriteFile(file, []byte(token), 0600); err != nil {
				t.Fatal(err)
			}
			b := config.InstallerBundle{Common: applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530"}, Monitoring: &config.InstallerMonitoring{Targets: []config.InstallerTarget{{Key: "identity", Name: "KyIdentity", URL: "https://identity.example/healthz"}}, Sources: []config.InstallerSource{{Key: "offboard", Name: "offboard", Target: "identity", TokenFile: file, OffboardingAlerts: true}}}}
			cfg := &config.Config{}
			switch mode {
			case "self":
				cfg.Server.AppURL = "https://identity.example"
			case "unknown-target":
				b.Monitoring.Sources[0].Target = "foreign"
			case "duplicate-target":
				b.Monitoring.Targets = append(b.Monitoring.Targets, config.InstallerTarget{Key: "other", Name: "kyidentity", URL: "https://other.example/healthz"})
			case "invalid-target-url":
				b.Monitoring.Targets[0].URL = "http://127.0.0.1/"
			case "invalid-interval":
				b.Monitoring.Targets[0].IntervalSec = 9
			case "public-token":
				if err := os.Chmod(file, 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink-token":
				link := filepath.Join(dir, "link")
				if err := os.Symlink(file, link); err != nil {
					t.Fatal(err)
				}
				b.Monitoring.Sources[0].TokenFile = link
			case "invalid-token":
				if err := os.WriteFile(file, []byte("fixture token invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			case "same-token":
				second := filepath.Join(dir, "second")
				if err := os.WriteFile(second, []byte(token), 0600); err != nil {
					t.Fatal(err)
				}
				b.Monitoring.Sources = append(b.Monitoring.Sources, config.InstallerSource{Key: "other", Name: "other", Target: "identity", TokenFile: second})
			case "active-sso-token":
				cfg.SSO.KySignOnSecret = token
			}
			err := b.PrepareMonitoring(cfg)
			if mode == "valid" {
				if err != nil || b.Monitoring.Sources[0].TokenHash() == "" {
					t.Fatal("valid enrollment refused", err)
				}
			} else if err == nil {
				t.Fatal("unsafe enrollment accepted")
			}
			if err != nil && strings.Contains(err.Error(), token) {
				t.Fatal("credential in error")
			}
		})
	}
}
