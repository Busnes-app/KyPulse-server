package backup_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/capsule"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/backup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const ssoMember = "config/installer-sso-credentials.json"

func ssoInstance(t *testing.T) (*config.Config, string, string) {
	t.Helper()
	cfg, st := sqliteInstance(t)
	dir := t.TempDir()
	client, directory := filepath.Join(dir, "client-secret"), filepath.Join(dir, "directory-secret")
	for path, value := range map[string]string{client: "client-secret-value", directory: "directory-secret-value-0123456789"} {
		if err := os.WriteFile(path, []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530", SSO: &applysetup.SSO{IssuerURL: "https://identity.example.test", ClientID: "pulse", ClientSecretFile: client, DirectorySecretFile: directory, RedirectURI: "https://pulse.example.test/api/sso/kysignon/callback"}})
	if err := st.Settings().SetSetting(context.Background(), store.InstallerConfigKey, string(raw)); err != nil {
		t.Fatal(err)
	}
	return cfg, client, directory
}

func TestInstallerSSOCredentialsSealedDrill(t *testing.T) {
	t.Setenv("KYPULSE_PORT", "8080")
	t.Setenv("KYPULSE_DB_DRIVER", "sqlite")
	cfg, client, directory := ssoInstance(t)
	payload, err := backup.Collect(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	f := findFile(payload.Files, ssoMember)
	if f == nil || f.Mode != 0600 {
		t.Fatal("missing private SSO credential member")
	}
	for _, member := range payload.Files {
		if member.Path != ssoMember && bytesContain(member.Data, "client-secret-value") {
			t.Fatal("plaintext secret escaped its member")
		}
	}
	// The restore must not depend on the originals still existing.
	os.Remove(client)
	os.Remove(directory)
	result, err := recoveryclient.Drill(context.Background(), t.TempDir(), payload, func(dir string, m capsule.Manifest) []recoveryclient.Check {
		var got struct{ ClientSecretFile, ClientSecret, DirectorySecretFile, DirectorySecret string }
		raw, err := os.ReadFile(filepath.Join(dir, ssoMember))
		if err != nil || json.Unmarshal(raw, &got) != nil || got.ClientSecret != "client-secret-value" || got.DirectorySecret != "directory-secret-value-0123456789" || got.ClientSecretFile != client || got.DirectorySecretFile != directory {
			t.Fatalf("sealed SSO credentials did not restore: %s", raw)
		}
		return backup.Checks(dir, m)
	})
	if err != nil || !result.Passed {
		t.Fatalf("sealed drill: %+v %v", result, err)
	}
}

func TestInstallerSSOBackupRefusesUnavailableSecret(t *testing.T) {
	cfg, client, _ := ssoInstance(t)
	os.Remove(client)
	if _, err := backup.Collect(context.Background(), cfg, "test"); err == nil {
		t.Fatal("backup sealed without the installer KyIdentity secret")
	}
}

func TestInstallerSSODrillRejectsMismatchAndOmission(t *testing.T) {
	t.Setenv("KYPULSE_PORT", "8080")
	t.Setenv("KYPULSE_DB_DRIVER", "sqlite")
	for _, mode := range []string{"path", "config", "empty", "omitted"} {
		t.Run(mode, func(t *testing.T) {
			cfg, client, _ := ssoInstance(t)
			payload, err := backup.Collect(context.Background(), cfg, "test")
			if err != nil {
				t.Fatal(err)
			}
			f := findFile(payload.Files, ssoMember)
			switch mode {
			case "path":
				f.Data = []byte(strings.ReplaceAll(string(f.Data), client, client+".other"))
			case "config":
				var m map[string]any
				json.Unmarshal(f.Data, &m)
				m["configSha256"] = strings.Repeat("0", 64)
				f.Data, _ = json.Marshal(m)
			case "empty":
				f.Data = []byte(`{"version":"ky.pulse-sso-credentials/1"}`)
			case "omitted":
				var files []recoveryclient.File
				var required []string
				for _, f := range payload.Files {
					if f.Path != ssoMember {
						files = append(files, f)
						required = append(required, f.Path)
					}
				}
				payload.Files = files
				payload.VerificationRecipe["required_files"] = required
			}
			result, err := recoveryclient.Drill(context.Background(), t.TempDir(), payload, backup.Checks)
			if err != nil {
				t.Fatal(err)
			}
			if result.Passed {
				t.Fatal("incomplete SSO credential restore passed")
			}
		})
	}
}

// A capsule from before the member existed, of an instance that is not installer-managed,
// still passes its drill.
func TestOlderCapsuleWithoutInstallerConfigPassesSSOCheck(t *testing.T) {
	t.Setenv("KYPULSE_PORT", "8080")
	t.Setenv("KYPULSE_DB_DRIVER", "sqlite")
	cfg, _ := sqliteInstance(t)
	payload, err := backup.Collect(context.Background(), cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	var files []recoveryclient.File
	var required []string
	for _, f := range payload.Files {
		if f.Path != ssoMember {
			files = append(files, f)
			required = append(required, f.Path)
		}
	}
	payload.Files = files
	payload.VerificationRecipe["required_files"] = required
	result, err := recoveryclient.Drill(context.Background(), t.TempDir(), payload, backup.Checks)
	if err != nil || !result.Passed {
		t.Fatalf("older capsule drill: %+v %v", result, err)
	}
}
