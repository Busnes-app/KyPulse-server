package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const ssoCredentialsPath = "config/installer-sso-credentials.json"
const ssoCredentialsVersion = "ky.pulse-sso-credentials/1"

// The installer config row holds only file references; startup reads the secrets from
// them. This member carries the secrets, bound to that row by its SHA-256 and the paths.
type ssoCredentials struct {
	Version             string `json:"version"`
	ConfigSHA256        string `json:"configSha256,omitempty"`
	ClientSecretFile    string `json:"clientSecretFile,omitempty"`
	ClientSecret        string `json:"clientSecret,omitempty"`
	DirectorySecretFile string `json:"directorySecretFile,omitempty"`
	DirectorySecret     string `json:"directorySecret,omitempty"`
}

// installerConfig reads the snapshot's installer configuration row; ok is false when
// the instance is not installer-managed.
func installerConfig(ctx context.Context, path string) (raw string, b applysetup.Common, ok bool, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", b, false, err
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: "mode=ro"}).String())
	if err != nil {
		return "", b, false, err
	}
	defer db.Close()
	err = db.QueryRowContext(ctx, "SELECT value FROM server_settings WHERE key=?", store.InstallerConfigKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", b, false, nil
	}
	if err != nil {
		return "", b, false, err
	}
	if err := applysetup.DecodeRaw([]byte(raw), &b); err != nil {
		return "", b, false, errors.New("invalid installer configuration")
	}
	return raw, b, true, nil
}

func expectedSSOCredentials(raw string, b applysetup.Common) ssoCredentials {
	sum := sha256.Sum256([]byte(raw))
	want := ssoCredentials{Version: ssoCredentialsVersion, ConfigSHA256: hex.EncodeToString(sum[:])}
	if b.SSO != nil {
		want.ClientSecretFile, want.DirectorySecretFile = b.SSO.ClientSecretFile, b.SSO.DirectorySecretFile
	}
	return want
}

func validSSOSecret(s string) bool { return s != "" && !strings.ContainsAny(s, "\x00\r\n") }

func collectSSOCredentials(ctx context.Context, snapshot string) (recoveryclient.File, error) {
	raw, b, ok, err := installerConfig(ctx, snapshot)
	if err != nil {
		return recoveryclient.File{}, err
	}
	member := ssoCredentials{Version: ssoCredentialsVersion}
	if ok {
		member = expectedSSOCredentials(raw, b)
		if b.SSO != nil {
			member.ClientSecret, err = applysetup.Secret(b.SSO.ClientSecretFile)
			if err == nil {
				member.DirectorySecret, err = applysetup.Secret(b.SSO.DirectorySecretFile)
			}
			if err != nil {
				return recoveryclient.File{}, errors.New("backup: installer KyIdentity secret file is unavailable or unsafe")
			}
		}
	}
	data, err := json.Marshal(member)
	return recoveryclient.File{Path: ssoCredentialsPath, Data: data, Mode: 0600}, err
}

// ssoCredentialsCheck fails when the snapshot is installer-managed and the member is
// missing or does not match it. Older capsules without an installer config still pass.
func ssoCredentialsCheck(dir string, memberPresent bool) recoveryclient.Check {
	check := recoveryclient.Check{Name: "Installer SSO Credentials", Message: "KyIdentity secrets missing, unsafe or inconsistent with snapshot"}
	raw, b, ok, err := installerConfig(context.Background(), filepath.Join(dir, "data/kypulse.db"))
	if err != nil {
		return check
	}
	if !ok && !memberPresent {
		check.Passed, check.Message = true, "Not installer-managed"
		return check
	}
	path := filepath.Join(dir, ssoCredentialsPath)
	info, err := os.Lstat(path)
	if !memberPresent || err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return check
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return check
	}
	var got ssoCredentials
	if decodeSourceJSON(data, &got) != nil {
		return check
	}
	want := ssoCredentials{Version: ssoCredentialsVersion}
	if ok {
		want = expectedSSOCredentials(raw, b)
	}
	secretsOK := got.ClientSecret == "" && got.DirectorySecret == ""
	if ok && b.SSO != nil {
		secretsOK = validSSOSecret(got.ClientSecret) && validSSOSecret(got.DirectorySecret)
	}
	got.ClientSecret, got.DirectorySecret = "", ""
	if got != want || !secretsOK {
		return check
	}
	check.Passed, check.Message = true, "KyIdentity secrets match the installer configuration"
	return check
}
