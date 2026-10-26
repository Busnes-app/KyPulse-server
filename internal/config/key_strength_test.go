package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

func TestSigningSecretFloorsPreserveEncryptionKey(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg.Database.DataDir, "encryption.key")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"KYPULSE_SESSION_SECRET", "KYPULSE_KYSIGNON_HMAC_SECRET"} {
		for _, size := range []int{15, 16, 32} {
			t.Setenv(name, strings.Repeat("x", size))
			_, err := config.LoadFromEnv()
			if (err != nil) != (size < 16) {
				t.Errorf("%s length %d: %v", name, size, err)
			}
			if err != nil && !strings.Contains(err.Error(), name) {
				t.Errorf("error does not identify its configuration field: %v", err)
			}
		}
		t.Setenv(name, "")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("signing-key validation changed the retained encryption key")
	}
}

func TestInstallerDirectoryKeyFloorBeforeConfiguration(t *testing.T) {
	for _, size := range []int{15, 16, 32} {
		dir := t.TempDir()
		clientFile, directoryFile := filepath.Join(dir, "client.secret"), filepath.Join(dir, "directory.secret")
		if err := os.WriteFile(clientFile, []byte("independent-client-fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(directoryFile, []byte(strings.Repeat("d", size)), 0600); err != nil {
			t.Fatal(err)
		}
		cfg := &config.Config{Server: config.ServerConfig{AppURL: "https://pulse.example"}}
		before := cfg.SSO
		bundle := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530", SSO: &applysetup.SSO{
			IssuerURL: "https://identity.example", ClientID: "client", RedirectURI: "https://pulse.example/api/sso/kysignon/callback",
			ClientSecretFile: clientFile, DirectorySecretFile: directoryFile,
		}}
		err := config.ApplyInstallerConfiguration(cfg, bundle)
		if (err != nil) != (size < 16) {
			t.Errorf("directory credential length %d: %v", size, err)
		}
		if size < 16 && cfg.SSO != before {
			t.Fatal("short directory credential partially changed active configuration")
		}
	}
}
