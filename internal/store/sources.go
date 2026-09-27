package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var sourceNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var ErrInvalidSourceName = errors.New("invalid source name")

type LogSource struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	TargetID  string     `json:"target_id"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at"`
}

type SourceStore interface {
	CreateCode(context.Context, string, string, time.Time) error
	Claim(context.Context, string, string, string, time.Time) (LogSource, error)
	Authenticate(context.Context, string) (LogSource, error)
	List(context.Context) ([]LogSource, error)
	Revoke(context.Context, string, time.Time) error
}

type sourceStore struct{ store *SQLStore }

func isUniqueViolation(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE") || strings.Contains(msg, "duplicate key")
}

func (s *sourceStore) CreateCode(ctx context.Context, codeHash, targetID string, expiresAt time.Time) error {
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.store.logs.lock(ctx, tx); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.store.rebind("DELETE FROM log_pairing_codes WHERE expires_at <= ?"), time.Now().UTC()); err != nil {
		return err
	}
	if targetID != "" {
		var exists string
		err = tx.QueryRowContext(ctx, s.store.rebind("SELECT id FROM targets WHERE id=?"), targetID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, s.store.rebind("INSERT INTO log_pairing_codes(code_hash,target_id,expires_at) VALUES(?,?,?)"), codeHash, nullableID(targetID), expiresAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return tx.Commit()
}

func (s *sourceStore) Claim(ctx context.Context, codeHash, tokenHash, name string, now time.Time) (LogSource, error) {
	var source LogSource
	if !sourceNameRE.MatchString(name) {
		return source, ErrInvalidSourceName
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return source, err
	}
	defer tx.Rollback()
	if _, err = s.store.logs.lock(ctx, tx); err != nil {
		return source, err
	}
	if _, err = tx.ExecContext(ctx, s.store.rebind("DELETE FROM log_pairing_codes WHERE expires_at <= ?"), now); err != nil {
		return source, err
	}
	var target sql.NullString
	err = tx.QueryRowContext(ctx, s.store.rebind("DELETE FROM log_pairing_codes WHERE code_hash=? AND expires_at > ? RETURNING target_id"), codeHash, now).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return source, err
		}
		return source, ErrNotFound
	}
	if err != nil {
		return source, err
	}
	// A failed insert rolls back the code deletion, leaving it claimable.
	source = LogSource{ID: uuid.NewString(), Name: name, TargetID: target.String, CreatedAt: now}
	_, err = tx.ExecContext(ctx, s.store.rebind("INSERT INTO log_sources(id,name,target_id,token_hash,created_at) VALUES(?,?,?,?,?)"), source.ID, source.Name, nullableID(source.TargetID), tokenHash, now)
	if err != nil {
		if isUniqueViolation(err) {
			return LogSource{}, ErrAlreadyExists
		}
		return LogSource{}, err
	}
	if err = tx.Commit(); err != nil {
		return LogSource{}, err
	}
	return source, nil
}

func (s *sourceStore) Authenticate(ctx context.Context, tokenHash string) (LogSource, error) {
	row := s.store.db.QueryRowContext(ctx, s.store.rebind("SELECT id,name,target_id,created_at,revoked_at FROM log_sources WHERE token_hash=? AND revoked_at IS NULL"), tokenHash)
	return scanSource(row)
}

func scanSource(row interface{ Scan(...any) error }) (LogSource, error) {
	var source LogSource
	var target sql.NullString
	var revoked sql.NullTime
	err := row.Scan(&source.ID, &source.Name, &target, &source.CreatedAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return source, ErrNotFound
	}
	if err != nil {
		return source, err
	}
	source.TargetID = target.String
	if revoked.Valid {
		source.RevokedAt = &revoked.Time
	}
	return source, nil
}

func (s *sourceStore) List(ctx context.Context) ([]LogSource, error) {
	rows, err := s.store.db.QueryContext(ctx, "SELECT id,name,target_id,created_at,revoked_at FROM log_sources ORDER BY created_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]LogSource, 0)
	for rows.Next() {
		item, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *sourceStore) Revoke(ctx context.Context, id string, now time.Time) error {
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.store.logs.lock(ctx, tx); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, s.store.rebind("UPDATE log_sources SET revoked_at=? WHERE id=? AND revoked_at IS NULL"), now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}
