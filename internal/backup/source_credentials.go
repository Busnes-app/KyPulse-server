package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/google/uuid"
)

const sourceCredentialsPath = "config/installer-source-credentials.json"

// This member is secret payload, not a setup bundle or a public status response.
type sourceCredentials struct {
	Version string             `json:"version"`
	Sources []sourceCredential `json:"sources"`
}
type sourceCredential struct {
	ID        string `json:"id"`
	TokenFile string `json:"tokenFile"`
	Token     string `json:"token"`
}
type sourceReference struct {
	ID, TokenFile, Hash string
}

func decodeSourceJSON(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("invalid source credential JSON")
	}
	return nil
}

// Read ownership from the immutable snapshot, never from a newer live database.
func sourceReferences(ctx context.Context, path string) ([]sourceReference, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: "mode=ro"}).String())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, "SELECT key,value FROM server_settings WHERE key LIKE 'installer.source.%' ORDER BY key")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []sourceReference
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var own struct {
			SetupID    string                 `json:"setupId"`
			ID         string                 `json:"id"`
			Definition config.InstallerSource `json:"definition"`
		}
		if err := decodeSourceJSON([]byte(raw), &own); err != nil {
			return nil, errors.New("invalid source ownership")
		}
		wantID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(own.SetupID+"\x00source\x00"+own.Definition.Key)).String()
		keyHash := sha256.Sum256([]byte(own.Definition.Key))
		if own.SetupID == "" || own.Definition.Key == "" || own.ID != wantID || key != "installer.source."+hex.EncodeToString(keyHash[:]) || !filepath.IsAbs(own.Definition.TokenFile) || filepath.Clean(own.Definition.TokenFile) != own.Definition.TokenFile || strings.ContainsAny(own.Definition.TokenFile, "\x00\r\n") {
			return nil, errors.New("invalid source ownership binding")
		}
		var name, target, hash string
		var revoked sql.NullTime
		var alerts bool
		if err := db.QueryRowContext(ctx, "SELECT name,target_id,token_hash,revoked_at,offboarding_alerts FROM log_sources WHERE id=?", own.ID).Scan(&name, &target, &hash, &revoked, &alerts); err != nil {
			return nil, errors.New("owned source missing from snapshot")
		}
		if revoked.Valid {
			continue
		}
		wantTarget := uuid.NewSHA1(uuid.NameSpaceOID, []byte(own.SetupID+"\x00target\x00"+own.Definition.Target)).String()
		if name != own.Definition.Name || target != wantTarget || alerts != own.Definition.OffboardingAlerts {
			return nil, errors.New("owned source conflicts with snapshot")
		}
		refs = append(refs, sourceReference{own.ID, own.Definition.TokenFile, hash})
	}
	return refs, rows.Err()
}

func sourceTokenMatches(token, hash string) bool {
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != 32 || strings.ToLower(token) != token {
		return false
	}
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:]) == hash
}

func collectSourceCredentials(ctx context.Context, snapshot string) (recoveryclient.File, error) {
	refs, err := sourceReferences(ctx, snapshot)
	if err != nil {
		return recoveryclient.File{}, err
	}
	member := sourceCredentials{Version: "ky.pulse-source-credentials/1", Sources: []sourceCredential{}}
	for _, ref := range refs {
		token, err := applysetup.Secret(ref.TokenFile)
		if err != nil || !sourceTokenMatches(token, ref.Hash) {
			return recoveryclient.File{}, errors.New("backup: owned source credential is unavailable or conflicts with snapshot")
		}
		member.Sources = append(member.Sources, sourceCredential{ref.ID, ref.TokenFile, token})
	}
	raw, err := json.Marshal(member)
	if len(raw) > 1<<20 {
		return recoveryclient.File{}, errors.New("backup: source credential member exceeds limit")
	}
	return recoveryclient.File{Path: sourceCredentialsPath, Data: raw, Mode: 0600}, err
}

func sourceCredentialsCheck(dir string, memberPresent bool) recoveryclient.Check {
	check := recoveryclient.Check{Name: "Installer Source Credentials", Message: "Source credentials missing, unsafe or inconsistent with snapshot"}
	refs, err := sourceReferences(context.Background(), filepath.Join(dir, "data/kypulse.db"))
	if err != nil {
		return check
	}
	path := filepath.Join(dir, sourceCredentialsPath)
	info, err := os.Lstat(path)
	// Old capsules without installer sources remain drillable.
	if os.IsNotExist(err) && len(refs) == 0 && !memberPresent {
		check.Passed = true
		check.Message = "No installer sources"
		return check
	}
	if !memberPresent || err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return check
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return check
	}
	var member sourceCredentials
	if decodeSourceJSON(raw, &member) != nil || member.Version != "ky.pulse-source-credentials/1" || len(member.Sources) != len(refs) {
		return check
	}
	for i, ref := range refs {
		got := member.Sources[i]
		if got.ID != ref.ID || got.TokenFile != ref.TokenFile || !sourceTokenMatches(got.Token, ref.Hash) {
			return check
		}
	}
	check.Passed = true
	check.Message = "Owned source credentials match snapshot"
	return check
}
