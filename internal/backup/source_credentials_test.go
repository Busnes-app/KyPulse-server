package backup_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/capsule"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/backup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const credentialMember = "config/installer-source-credentials.json"

func sourceInstance(t *testing.T) (*config.Config, store.Store, string, string, string) {
	t.Helper()
	cfg, st := sqliteInstance(t)
	path, token := filepath.Join(t.TempDir(), "source-token"), strings.Repeat("ab", 32)
	if err := os.WriteFile(path, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	b := config.InstallerBundle{Common: applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530"}, Monitoring: &config.InstallerMonitoring{Targets: []config.InstallerTarget{{Key: "identity", Name: "KyIdentity", URL: "https://identity.example/healthz"}}, Sources: []config.InstallerSource{{Key: "offboard", Name: "offboard", Target: "identity", TokenFile: path, OffboardingAlerts: true}}}}
	if err := b.PrepareMonitoring(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplyInstaller(context.Background(), b, false); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(token))
	source, err := st.Sources().Authenticate(context.Background(), hex.EncodeToString(hash[:]))
	if err != nil {
		t.Fatal(err)
	}
	return cfg, st, path, token, source.ID
}

func TestInstallerSourceCredentialsSealedDrill(t *testing.T) {
	t.Setenv("KYPULSE_PORT", "8080")
	t.Setenv("KYPULSE_DB_DRIVER", "sqlite")
	cfg, _, sourcePath, token, id := sourceInstance(t)
	payload, err := backup.Collect(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	f := findFile(payload.Files, credentialMember)
	if f == nil || f.Mode != 0600 {
		t.Fatal("missing private credential member")
	}
	for _, member := range payload.Files {
		if member.Path != credentialMember && bytesContain(member.Data, token) {
			t.Fatal("plaintext token escaped credential member")
		}
	}
	public, _ := json.Marshal(struct{ Recipe, Deps any }{payload.VerificationRecipe, payload.Dependencies})
	if bytesContain(public, token) || strings.Contains(strings.Join(backup.Members(cfg), " "), token) {
		t.Fatal("credential leaked in metadata")
	}
	// Losing the original file after collection must not affect sealed restore checks.
	if err := os.Remove(sourcePath); err != nil {
		t.Fatal(err)
	}
	result, err := recoveryclient.Drill(context.Background(), t.TempDir(), payload, func(dir string, m capsule.Manifest) []recoveryclient.Check {
		var member struct {
			Version string                                  `json:"version"`
			Sources []struct{ ID, TokenFile, Token string } `json:"sources"`
		}
		raw, err := os.ReadFile(filepath.Join(dir, credentialMember))
		if err != nil || json.Unmarshal(raw, &member) != nil || len(member.Sources) != 1 || member.Sources[0].ID != id || member.Sources[0].Token != token || member.Sources[0].TokenFile != sourcePath {
			t.Fatal("sealed source credential did not restore")
		}
		copyStore, err := store.Open(context.Background(), config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(dir, "data/kypulse.db"), AuditKey: cfg.Database.AuditKey})
		if err != nil {
			t.Fatal(err)
		}
		defer copyStore.Close()
		hash := sha256.Sum256([]byte(member.Sources[0].Token))
		source, err := copyStore.Sources().Authenticate(context.Background(), hex.EncodeToString(hash[:]))
		if err != nil || source.ID != id || !source.OffboardingAlerts {
			t.Fatal("restored token cannot authenticate native source")
		}
		return backup.Checks(dir, m)
	})
	if err != nil || !result.Passed {
		t.Fatalf("sealed drill: %+v %v", result, err)
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatal("drill wrote original absolute path")
	}
}

func bytesContain(raw []byte, token string) bool { return strings.Contains(string(raw), token) }

func TestInstallerSourceBackupRefusesUnavailableOrChangedCredentials(t *testing.T) {
	for _, mode := range []string{"missing", "changed", "public", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			cfg, _, path, _, _ := sourceInstance(t)
			switch mode {
			case "missing":
				os.Remove(path)
			case "changed":
				os.WriteFile(path, []byte(strings.Repeat("cd", 32)), 0600)
			case "public":
				os.Chmod(path, 0644)
			case "symlink":
				os.Rename(path, path+".real")
				os.Symlink(path+".real", path)
			}
			if _, err := backup.Collect(context.Background(), cfg, "test"); err == nil {
				t.Fatal("unsafe or inconsistent source backup accepted")
			}
		})
	}
}

func TestInstallerSourceDrillRejectsCredentialMismatchAndOmission(t *testing.T) {
	t.Setenv("KYPULSE_PORT", "8080")
	t.Setenv("KYPULSE_DB_DRIVER", "sqlite")
	for _, mode := range []string{"token", "empty", "omitted"} {
		t.Run(mode, func(t *testing.T) {
			cfg, _, _, token, _ := sourceInstance(t)
			payload, err := backup.Collect(context.Background(), cfg, "test")
			if err != nil {
				t.Fatal(err)
			}
			f := findFile(payload.Files, credentialMember)
			if mode == "token" {
				f.Data = []byte(strings.ReplaceAll(string(f.Data), token, strings.Repeat("cd", 32)))
			}
			if mode == "empty" {
				f.Data = []byte(`{"version":"ky.pulse-source-credentials/1","sources":[]}`)
			}
			if mode == "omitted" {
				var files []recoveryclient.File
				for _, f := range payload.Files {
					if f.Path != credentialMember {
						files = append(files, f)
					}
				}
				payload.Files = files
				var required []string
				for _, f := range files {
					required = append(required, f.Path)
				}
				payload.VerificationRecipe["required_files"] = required
			}
			result, err := recoveryclient.Drill(context.Background(), t.TempDir(), payload, backup.Checks)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed {
				t.Fatal("incomplete credential restore passed")
			}
		})
	}
}

func TestInstallerSourceBackupDoesNotReactivateRevokedSource(t *testing.T) {
	cfg, st, path, _, id := sourceInstance(t)
	if err := st.Sources().Revoke(context.Background(), id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	payload, err := backup.Collect(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	var member struct {
		Sources []any `json:"sources"`
	}
	if err := json.Unmarshal(findFile(payload.Files, credentialMember).Data, &member); err != nil || len(member.Sources) != 0 {
		t.Fatal("revoked credential retained", err)
	}
}
