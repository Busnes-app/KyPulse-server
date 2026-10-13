package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

const InstallerConfigKey = "installer.config"

type installerIdentity struct {
	SetupID string `json:"setupId"`
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
	UserID  string `json:"userId"`
	Role    string `json:"role"`
}

// ApplyInstaller commits accounts, ownership and audits together. It never adopts
// another account, changes credentials or takes ownership of pre-existing settings.
func (s *SQLStore) ApplyInstaller(ctx context.Context, b config.InstallerBundle, check bool) (applysetup.Report, error) {
	r := applysetup.NewReport(b.Common)
	if err := b.Validate(); err != nil {
		return r, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	if b.Monitoring != nil && !check {
		if _, err := s.logs.lock(ctx, tx); err != nil {
			return r, err
		}
	}
	now := time.Now().UTC()
	setting := func(key string) (string, error) {
		var value string
		err := tx.QueryRowContext(ctx, s.rebind(`SELECT value FROM server_settings WHERE key=?`), key).Scan(&value)
		return value, err
	}
	put := func(key, value string) error {
		if _, err := tx.ExecContext(ctx, s.rebind(`INSERT INTO server_settings(key,value,updated_at) VALUES(?,?,?)`), key, value, now); err != nil {
			return err
		}
		return nil
	}
	// Configuration records hold target-side file references, never credentials.
	configBundle := applysetup.Common{Schema: b.Schema, Service: b.Service, SetupID: b.SetupID, SSO: b.SSO, Backup: b.Backup}
	if b.SSO != nil || b.Backup != nil {
		raw, _ := json.Marshal(configBundle)
		existing, err := setting(InstallerConfigKey)
		obj := applysetup.Object{Kind: "configuration", Name: "runtime"}
		switch {
		case err == nil:
			if existing != string(raw) {
				return r, errors.New("installer configuration ownership or values conflict")
			}
			r.Present = append(r.Present, obj)
		case errors.Is(err, sql.ErrNoRows):
			if check {
				return r, errors.New("installer configuration is absent")
			}
			if err = put(InstallerConfigKey, string(raw)); err != nil {
				return r, err
			}
			r.Created = append(r.Created, obj)
		default:
			return r, err
		}
	}
	for i, list := range [][]applysetup.Identity{b.Admins, b.Everyday} {
		role := "viewer"
		if i == 0 {
			role = "admin"
		}
		for _, identity := range list {
			hash := sha256.Sum256([]byte(identity.Issuer + "\x00" + identity.Subject))
			key := "installer.user." + hex.EncodeToString(hash[:])
			existing, err := setting(key)
			obj := applysetup.Object{Kind: "identity", Name: identity.Username}
			if err == nil {
				var owned installerIdentity
				if json.Unmarshal([]byte(existing), &owned) != nil || owned.SetupID != b.SetupID || owned.Issuer != identity.Issuer || owned.Subject != identity.Subject || owned.Role != role {
					return r, errors.New("installer identity ownership conflicts")
				}
				var provider, subject, name, currentRole, status string
				if err = tx.QueryRowContext(ctx, s.rebind(`SELECT sso_provider,sso_subject,username,role,status FROM users WHERE id=?`), owned.UserID).Scan(&provider, &subject, &name, &currentRole, &status); err != nil {
					return r, err
				}
				if provider != "kysignon" || subject != identity.Subject || name != identity.Username || currentRole != role || status != "active" {
					return r, errors.New("installer identity drift requires explicit operator resolution")
				}
				obj.ID = owned.UserID
				r.Present = append(r.Present, obj)
				continue
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return r, err
			}
			if check {
				return r, errors.New("installer identity is absent")
			}
			var count int
			if err = tx.QueryRowContext(ctx, s.rebind(`SELECT COUNT(*) FROM users WHERE lower(username)=lower(?) OR (sso_subject<>'' AND sso_subject=?)`), identity.Username, identity.Subject).Scan(&count); err != nil {
				return r, err
			}
			if count != 0 {
				return r, errors.New("installer refuses to adopt an existing identity")
			}
			id := "usr_" + hex.EncodeToString(hash[:12])
			if _, err = tx.ExecContext(ctx, s.rebind(`INSERT INTO users(id,username,email,display_name,role,status,sso_provider,sso_subject,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`), id, identity.Username, identity.Email, identity.Username, role, "active", "kysignon", identity.Subject, now, now); err != nil {
				return r, err
			}
			owned, _ := json.Marshal(installerIdentity{b.SetupID, identity.Issuer, identity.Subject, id, role})
			if err = put(key, string(owned)); err != nil {
				return r, err
			}
			obj.ID = id
			r.Created = append(r.Created, obj)
		}
	}
	if err := s.applyInstallerMonitoring(ctx, tx, b, check, &r, now); err != nil {
		return r, err
	}
	if check {
		return r, nil
	}
	var finish func(bool)
	if len(r.Created) > 0 {
		finish, err = s.audit.append(ctx, tx, &AuditRecord{Action: "installer.setup_created", Resource: b.Service, Details: b.SetupID, IPAddress: "installer-local", CreatedAt: now})
		if err != nil {
			return r, err
		}
	}
	committed := false
	if finish != nil {
		defer func() { finish(committed) }()
	}
	if err = tx.Commit(); err != nil {
		return r, err
	}
	committed = true
	return r, nil
}
