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
	"github.com/google/uuid"
)

type installerMonitoringOwnership struct {
	SetupID    string          `json:"setupId"`
	ID         string          `json:"id"`
	Definition json.RawMessage `json:"definition"`
}

func (s *SQLStore) applyInstallerMonitoring(ctx context.Context, tx *sql.Tx, b config.InstallerBundle, check bool, r *applysetup.Report, now time.Time) error {
	if b.Monitoring == nil {
		return nil
	}
	// Ownership and native objects share the caller's account/config/audit transaction.
	owned := func(kind, key, id string, definition any) (string, bool, error) {
		hash := sha256.Sum256([]byte(key))
		settingKey := "installer." + kind + "." + hex.EncodeToString(hash[:])
		raw, _ := json.Marshal(definition)
		want, _ := json.Marshal(installerMonitoringOwnership{SetupID: b.SetupID, ID: id, Definition: raw})
		var existing string
		err := tx.QueryRowContext(ctx, s.rebind("SELECT value FROM server_settings WHERE key=?"), settingKey).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			return string(want), false, nil
		}
		if err != nil {
			return "", false, err
		}
		if existing != string(want) {
			return "", false, errors.New("monitoring ownership or definition conflicts")
		}
		return existing, true, nil
	}
	put := func(kind, key, value string) error {
		hash := sha256.Sum256([]byte(key))
		_, err := tx.ExecContext(ctx, s.rebind("INSERT INTO server_settings(key,value,updated_at) VALUES(?,?,?)"), "installer."+kind+"."+hex.EncodeToString(hash[:]), value, now)
		return err
	}
	ids := make(map[string]string, len(b.Monitoring.Targets))
	for _, want := range b.Monitoring.Targets {
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(b.SetupID+"\x00target\x00"+want.Key)).String()
		definition, present, err := owned("target", want.Key, id, want)
		if err != nil {
			return err
		}
		obj := applysetup.Object{Kind: "configuration", Name: "monitoring.target." + want.Key, ID: id}
		if present {
			got, err := s.targets.scanTarget(tx.QueryRowContext(ctx, s.rebind("SELECT "+targetColumns+" FROM targets WHERE id=?"), id))
			if err != nil {
				return err
			}
			if got.Name != want.Name || got.URL != want.URL || got.IntervalSec != want.Interval() || !got.Enabled || got.Container != want.Container {
				return errors.New("monitoring target drift requires operator resolution")
			}
			r.Present = append(r.Present, obj)
		} else {
			if check {
				return errors.New("monitoring target is absent")
			}
			var count int
			if err := tx.QueryRowContext(ctx, s.rebind("SELECT COUNT(*) FROM targets WHERE id=? OR lower(name)=lower(?)"), id, want.Name).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("installer refuses to adopt a monitoring target")
			}
			if _, err := tx.ExecContext(ctx, s.rebind(`INSERT INTO targets(id,name,url,interval_sec,enabled,container,state_since,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`), id, want.Name, want.URL, want.Interval(), true, want.Container, now, now, now); err != nil {
				return err
			}
			if err := put("target", want.Key, definition); err != nil {
				return err
			}
			r.Created = append(r.Created, obj)
		}
		ids[want.Key] = id
	}
	for _, want := range b.Monitoring.Sources {
		if want.TokenHash() == "" {
			return errors.New("monitoring source credentials were not prepared")
		}
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(b.SetupID+"\x00source\x00"+want.Key)).String()
		definition, present, err := owned("source", want.Key, id, want)
		if err != nil {
			return err
		}
		obj := applysetup.Object{Kind: "configuration", Name: "monitoring.source." + want.Key, ID: id}
		if present {
			got, err := scanSource(tx.QueryRowContext(ctx, s.rebind("SELECT id,name,target_id,created_at,revoked_at,offboarding_alerts FROM log_sources WHERE id=?"), id))
			if err != nil {
				return err
			}
			var tokenHash string
			if err := tx.QueryRowContext(ctx, s.rebind("SELECT token_hash FROM log_sources WHERE id=?"), id).Scan(&tokenHash); err != nil {
				return err
			}
			if got.Name != want.Name || got.TargetID != ids[want.Target] || got.RevokedAt != nil || got.OffboardingAlerts != want.OffboardingAlerts || tokenHash != want.TokenHash() {
				return errors.New("monitoring source drift requires operator resolution")
			}
			r.Present = append(r.Present, obj)
		} else {
			if check {
				return errors.New("monitoring source is absent")
			}
			var count int
			if err := tx.QueryRowContext(ctx, s.rebind("SELECT COUNT(*) FROM log_sources WHERE id=? OR lower(name)=lower(?) OR token_hash=?"), id, want.Name, want.TokenHash()).Scan(&count); err != nil {
				return err
			}
			if count != 0 {
				return errors.New("installer refuses to adopt a monitoring source")
			}
			if _, err := tx.ExecContext(ctx, s.rebind("INSERT INTO log_sources(id,name,target_id,token_hash,created_at,offboarding_alerts) VALUES(?,?,?,?,?,?)"), id, want.Name, ids[want.Target], want.TokenHash(), now, want.OffboardingAlerts); err != nil {
				return err
			}
			if err := put("source", want.Key, definition); err != nil {
				return err
			}
			r.Created = append(r.Created, obj)
		}
	}
	return nil
}
