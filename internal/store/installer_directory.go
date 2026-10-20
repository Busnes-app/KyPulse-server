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
)

// DirectoryUpdate is one verified KyIdentity suite-directory delivery for an installer-owned
// identity. Revision is the sender's monotonic per-resource revision; Digest binds its body.
type DirectoryUpdate struct {
	Issuer, Subject, Event, Digest string
	Revision                       uint64
	Assigned, Admin                bool
}

type directoryFence struct {
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
}

// ErrDirectoryRefused covers a foreign identity, an unsupported event or a stale/conflicting
// revision. The sender keeps the event and an operator resolves it.
var ErrDirectoryRefused = errors.New("directory update refused")

// ApplyInstallerDirectory sets an installer-owned identity's availability. Unavailable
// (unassigned, deactivated, deleted, or an owned admin without the admin role) ends its
// sessions and pending MFA challenges. It never changes a role, so a delivery cannot escalate.
func (s *SQLStore) ApplyInstallerDirectory(ctx context.Context, u DirectoryUpdate) error {
	switch u.Event {
	case "user.created", "user.updated", "user.deactivated", "user.deleted":
	default:
		return ErrDirectoryRefused
	}
	if u.Revision == 0 || len(u.Digest) != 64 {
		return ErrDirectoryRefused
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	setting := func(key string) (string, error) {
		var v string
		err := tx.QueryRowContext(ctx, s.rebind(`SELECT value FROM server_settings WHERE key=?`), key).Scan(&v)
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrDirectoryRefused
		}
		return v, err
	}
	raw, err := setting(InstallerConfigKey)
	if err != nil {
		return err
	}
	var cfg applysetup.Common
	if err = json.Unmarshal([]byte(raw), &cfg); err != nil {
		return err
	}
	if cfg.SSO == nil || cfg.SSO.IssuerURL != u.Issuer {
		return ErrDirectoryRefused
	}
	hash := sha256.Sum256([]byte(u.Issuer + "\x00" + u.Subject))
	if raw, err = setting("installer.user." + hex.EncodeToString(hash[:])); err != nil {
		return err
	}
	var owned installerIdentity
	if err = json.Unmarshal([]byte(raw), &owned); err != nil {
		return err
	}
	if owned.SetupID != cfg.SetupID || owned.Issuer != u.Issuer || owned.Subject != u.Subject {
		return ErrDirectoryRefused
	}
	fenceKey := "installer.directory." + owned.UserID
	raw, err = setting(fenceKey)
	switch {
	case err == nil:
		var last directoryFence
		if err = json.Unmarshal([]byte(raw), &last); err != nil {
			return err
		}
		if u.Revision < last.Revision || u.Revision == last.Revision && u.Digest != last.Digest {
			return ErrDirectoryRefused
		}
		if u.Revision == last.Revision {
			return nil // a retry of the delivery already applied
		}
	case !errors.Is(err, ErrDirectoryRefused):
		return err
	}
	available := u.Assigned && u.Event != "user.deactivated" && u.Event != "user.deleted" && (u.Admin || owned.Role != RoleAdmin)
	status := "inactive"
	if available {
		status = "active"
	}
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, s.rebind(`UPDATE users SET status=?,updated_at=? WHERE id=? AND sso_provider='kysignon' AND sso_subject=?`), status, now, owned.UserID, u.Subject)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return errors.Join(ErrDirectoryRefused, err)
	}
	if !available {
		for _, q := range []string{`DELETE FROM sessions WHERE user_id=?`, `DELETE FROM mfa_challenges WHERE user_id=?`} {
			if _, err = tx.ExecContext(ctx, s.rebind(q), owned.UserID); err != nil {
				return err
			}
		}
	}
	fence, _ := json.Marshal(directoryFence{u.Revision, u.Digest})
	if _, err = tx.ExecContext(ctx, s.rebind(`INSERT INTO server_settings(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`), fenceKey, string(fence), now); err != nil {
		return err
	}
	finish, err := s.audit.append(ctx, tx, &AuditRecord{UserID: owned.UserID, Action: "installer.directory_applied", Resource: status, Details: u.Event, IPAddress: "directory", CreatedAt: now})
	if err != nil {
		return err
	}
	committed := false
	defer func() { finish(committed) }()
	if err = tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
