package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store/migrations"
)

// SQLStore implements Store on top of database/sql.
type SQLStore struct {
	db       *sql.DB
	driver   string
	users    *userStore
	sessions *sessionStore
	audit    *auditStore
	settings *settingsStore
	targets  *targetStore
}

// newSQLStore creates and initializes a SQLStore, running migrations automatically.
func newSQLStore(ctx context.Context, db *sql.DB, driver string) (*SQLStore, error) {
	driver = strings.ToLower(driver)
	if driver == "postgresql" {
		driver = "postgres"
	}

	if err := migrations.Run(ctx, db, driver); err != nil {
		return nil, fmt.Errorf("migration failure on driver %s: %w", driver, err)
	}

	s := &SQLStore{
		db:     db,
		driver: driver,
	}

	s.users = &userStore{store: s}
	s.sessions = &sessionStore{store: s}
	s.audit = &auditStore{store: s}
	s.settings = &settingsStore{store: s}
	s.targets = &targetStore{store: s}

	return s, nil
}

func (s *SQLStore) Users() UserStore        { return s.users }
func (s *SQLStore) Sessions() SessionStore  { return s.sessions }
func (s *SQLStore) Audit() AuditStore       { return s.audit }
func (s *SQLStore) Settings() SettingsStore { return s.settings }
func (s *SQLStore) Targets() TargetStore    { return s.targets }

func (s *SQLStore) Driver() string                 { return s.driver }
func (s *SQLStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *SQLStore) Close() error                   { return s.db.Close() }

// rebind converts '?' placeholders to '$1, $2, ...' for Postgres
func (s *SQLStore) rebind(query string) string {
	if s.driver != "postgres" {
		return query
	}

	var b strings.Builder
	b.Grow(len(query) + 16)
	paramIdx := 1

	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(paramIdx))
			paramIdx++
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------
// User Store
// ---------------------------------------------------------------------

type userStore struct {
	store *SQLStore
}

func (u *userStore) CreateUser(ctx context.Context, user *User) error {
	if !validRole(user.Role) {
		return ErrInvalidRole
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	if user.RecoveryCodesHash == "" {
		user.RecoveryCodesHash = "[]"
	}

	q := u.store.rebind(`
INSERT INTO users (
    id, username, email, display_name, password_hash, role, status,
    sso_provider, sso_subject, totp_secret_enc, totp_enabled,
    recovery_codes_hash, push_device_id, must_change_password,
    created_at, updated_at, last_login_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`)

	var lastLogin sql.NullTime
	if user.LastLoginAt != nil {
		lastLogin = sql.NullTime{Time: *user.LastLoginAt, Valid: true}
	}

	_, err := u.store.db.ExecContext(ctx, q,
		user.ID, user.Username, user.Email, user.DisplayName, user.PasswordHash,
		user.Role, user.Status, user.SSOProvider, user.SSOSubject,
		user.TOTPSecretEnc, user.TOTPEnabled, user.RecoveryCodesHash,
		user.PushDeviceID, user.MustChangePassword,
		user.CreatedAt, user.UpdatedAt, lastLogin,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate key") {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

func (u *userStore) scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var user User
	var lastLogin sql.NullTime

	err := row.Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.PasswordHash,
		&user.Role, &user.Status, &user.SSOProvider, &user.SSOSubject,
		&user.TOTPSecretEnc, &user.TOTPEnabled, &user.RecoveryCodesHash,
		&user.PushDeviceID, &user.MustChangePassword, &user.TOTPLastCounter,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err != nil {
		if errorsIs(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func (u *userStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	q := u.store.rebind(`
SELECT id, username, email, display_name, password_hash, role, status,
       sso_provider, sso_subject, totp_secret_enc, totp_enabled,
       recovery_codes_hash, push_device_id, must_change_password,
       totp_last_counter, created_at, updated_at, last_login_at
FROM users WHERE id = ?
`)
	return u.scanUser(u.store.db.QueryRowContext(ctx, q, id))
}

func (u *userStore) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	q := u.store.rebind(`
SELECT id, username, email, display_name, password_hash, role, status,
       sso_provider, sso_subject, totp_secret_enc, totp_enabled,
       recovery_codes_hash, push_device_id, must_change_password,
       totp_last_counter, created_at, updated_at, last_login_at
FROM users WHERE LOWER(username) = LOWER(?)
`)
	return u.scanUser(u.store.db.QueryRowContext(ctx, q, username))
}

func (u *userStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	q := u.store.rebind(`
SELECT id, username, email, display_name, password_hash, role, status,
       sso_provider, sso_subject, totp_secret_enc, totp_enabled,
       recovery_codes_hash, push_device_id, must_change_password,
       totp_last_counter, created_at, updated_at, last_login_at
FROM users WHERE LOWER(email) = LOWER(?)
`)
	return u.scanUser(u.store.db.QueryRowContext(ctx, q, email))
}

func (u *userStore) GetUserBySSO(ctx context.Context, provider, subject string) (*User, error) {
	q := u.store.rebind(`
SELECT id, username, email, display_name, password_hash, role, status,
       sso_provider, sso_subject, totp_secret_enc, totp_enabled,
       recovery_codes_hash, push_device_id, must_change_password,
       totp_last_counter, created_at, updated_at, last_login_at
FROM users WHERE sso_provider = ? AND sso_subject = ?
`)
	return u.scanUser(u.store.db.QueryRowContext(ctx, q, provider, subject))
}

func (u *userStore) UpdateUser(ctx context.Context, user *User) error {
	if !validRole(user.Role) {
		return ErrInvalidRole
	}
	user.UpdatedAt = time.Now().UTC()
	var lastLogin sql.NullTime
	if user.LastLoginAt != nil {
		lastLogin = sql.NullTime{Time: *user.LastLoginAt, Valid: true}
	}

	q := u.store.rebind(`
UPDATE users SET
    username = ?, email = ?, display_name = ?, password_hash = ?,
    role = ?, status = ?, sso_provider = ?, sso_subject = ?,
    totp_secret_enc = ?, totp_enabled = ?, recovery_codes_hash = ?,
    push_device_id = ?, must_change_password = ?, updated_at = ?,
    last_login_at = ?
WHERE id = ?
`)

	res, err := u.store.db.ExecContext(ctx, q,
		user.Username, user.Email, user.DisplayName, user.PasswordHash,
		user.Role, user.Status, user.SSOProvider, user.SSOSubject,
		user.TOTPSecretEnc, user.TOTPEnabled, user.RecoveryCodesHash,
		user.PushDeviceID, user.MustChangePassword, user.UpdatedAt,
		lastLogin, user.ID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (u *userStore) UpdateRecoveryCodes(ctx context.Context, userID, oldHashes, newHashes string) error {
	q := u.store.rebind("UPDATE users SET recovery_codes_hash = ?, updated_at = ? WHERE id = ? AND recovery_codes_hash = ?")
	res, err := u.store.db.ExecContext(ctx, q, newHashes, time.Now().UTC(), userID, oldHashes)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrAlreadyExists
	}
	return nil
}

func (u *userStore) SpendTOTPCounter(ctx context.Context, userID string, counter int64) error {
	q := u.store.rebind("UPDATE users SET totp_last_counter = ?, updated_at = ? WHERE id = ? AND totp_last_counter < ?")
	res, err := u.store.db.ExecContext(ctx, q, counter, time.Now().UTC(), userID, counter)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrAlreadyExists
	}
	return nil
}

func (u *userStore) DeleteUser(ctx context.Context, id string) error {
	q := u.store.rebind("DELETE FROM users WHERE id = ?")
	res, err := u.store.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (u *userStore) ListUsers(ctx context.Context, offset, limit int, search string) ([]*User, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var countQuery, listQuery string
	var countArgs, listArgs []any

	if strings.TrimSpace(search) != "" {
		like := "%" + strings.ToLower(strings.TrimSpace(search)) + "%"
		countQuery = "SELECT COUNT(1) FROM users WHERE LOWER(username) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(email) LIKE ?"
		countArgs = []any{like, like, like}

		listQuery = `
SELECT id, username, email, display_name, password_hash, role, status,
       sso_provider, sso_subject, totp_secret_enc, totp_enabled,
       recovery_codes_hash, push_device_id, must_change_password,
       totp_last_counter, created_at, updated_at, last_login_at
FROM users
WHERE LOWER(username) LIKE ? OR LOWER(display_name) LIKE ? OR LOWER(email) LIKE ?
ORDER BY created_at DESC LIMIT ? OFFSET ?`
		listArgs = []any{like, like, like, limit, offset}
	} else {
		countQuery = "SELECT COUNT(1) FROM users"
		listQuery = `
SELECT id, username, email, display_name, password_hash, role, status,
       sso_provider, sso_subject, totp_secret_enc, totp_enabled,
       recovery_codes_hash, push_device_id, must_change_password,
       totp_last_counter, created_at, updated_at, last_login_at
FROM users
ORDER BY created_at DESC LIMIT ? OFFSET ?`
		listArgs = []any{limit, offset}
	}

	var total int
	err := u.store.db.QueryRowContext(ctx, u.store.rebind(countQuery), countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := u.store.db.QueryContext(ctx, u.store.rebind(listQuery), listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		user, err := u.scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}

	return users, total, rows.Err()
}

func (u *userStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := u.store.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM users").Scan(&count)
	return count, err
}

// ---------------------------------------------------------------------
// Session Store
// ---------------------------------------------------------------------

type sessionStore struct {
	store *SQLStore
}

// withPassword serializes credential-derived grants with password replacement.
// Updating the same user row takes a write lock on both supported databases.
func (s *SQLStore) withPassword(ctx context.Context, userID, expectedHash string, apply func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, s.rebind("UPDATE users SET id = id WHERE id = ? AND password_hash = ? AND status = 'active'"), userID, expectedHash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err := apply(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sessionStore) CreateSession(ctx context.Context, sess *Session, expectedPasswordHash string) error {
	return s.store.withPassword(ctx, sess.UserID, expectedPasswordHash, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, s.store.rebind(`INSERT INTO sessions (token_hash, user_id, user_agent, ip_address, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`), sess.TokenHash, sess.UserID, sess.UserAgent, sess.IPAddress, sess.CreatedAt, sess.ExpiresAt)
		return err
	})
}

func (u *userStore) CompletePasswordChange(ctx context.Context, userID, oldHash, newHash, ip string) error {
	return u.store.withPassword(ctx, userID, oldHash, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		result, err := tx.ExecContext(ctx, u.store.rebind(`UPDATE users SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE id = ? AND must_change_password = ? AND sso_provider = 'local'`), newHash, false, now, userID, true)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrNotFound
		}
		return u.revokePasswordGrants(ctx, tx, userID, "forced replacement; sessions revoked", ip, now)
	})
}

// ResetAdminPassword is the operator recovery path, including disabled local accounts.
func (u *userStore) ResetAdminPassword(ctx context.Context, userID, newHash string) error {
	tx, err := u.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, u.store.rebind(`UPDATE users SET password_hash = ?, must_change_password = ?, status = 'active', role = 'admin', updated_at = ? WHERE id = ? AND sso_provider = 'local'`), newHash, true, now, userID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err := u.revokePasswordGrants(ctx, tx, userID, "operator reset; sessions revoked", "", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (u *userStore) revokePasswordGrants(ctx context.Context, tx *sql.Tx, userID, details, ip string, now time.Time) error {
	for _, table := range []string{"sessions", "mfa_challenges"} {
		if _, err := tx.ExecContext(ctx, u.store.rebind("DELETE FROM "+table+" WHERE user_id = ?"), userID); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, u.store.rebind(`INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at) VALUES (?, ?, ?, ?, ?, ?)`), userID, "auth.password_changed", "user", details, ip, now)
	return err
}

func (s *sessionStore) GetSession(ctx context.Context, tokenHash string) (*Session, error) {
	q := s.store.rebind(`
SELECT token_hash, user_id, user_agent, ip_address, created_at, expires_at
FROM sessions WHERE token_hash = ?
`)
	var sess Session
	err := s.store.db.QueryRowContext(ctx, q, tokenHash).Scan(
		&sess.TokenHash, &sess.UserID, &sess.UserAgent, &sess.IPAddress,
		&sess.CreatedAt, &sess.ExpiresAt,
	)
	if err != nil {
		if errorsIs(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		_ = s.DeleteSession(ctx, tokenHash)
		return nil, ErrSessionExpired
	}
	return &sess, nil
}

func (s *sessionStore) DeleteSession(ctx context.Context, tokenHash string) error {
	q := s.store.rebind("DELETE FROM sessions WHERE token_hash = ?")
	_, err := s.store.db.ExecContext(ctx, q, tokenHash)
	return err
}

func (s *sessionStore) DeleteUserSessions(ctx context.Context, userID string) error {
	q := s.store.rebind("DELETE FROM sessions WHERE user_id = ?")
	_, err := s.store.db.ExecContext(ctx, q, userID)
	return err
}

func (s *sessionStore) CleanExpiredSessions(ctx context.Context) error {
	q := s.store.rebind("DELETE FROM sessions WHERE expires_at < ?")
	_, err := s.store.db.ExecContext(ctx, q, time.Now().UTC())
	return err
}

func (s *sessionStore) CreateMFAChallenge(ctx context.Context, challenge *MFAChallenge, expectedPasswordHash string) error {
	return s.store.withPassword(ctx, challenge.UserID, expectedPasswordHash, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, s.store.rebind("INSERT INTO mfa_challenges (token_hash, user_id, expires_at, password_hash) VALUES (?, ?, ?, ?)"), challenge.TokenHash, challenge.UserID, challenge.ExpiresAt, expectedPasswordHash)
		return err
	})
}

func (s *sessionStore) ConsumeMFAChallenge(ctx context.Context, tokenHash string) (string, string, error) {
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	q := s.store.rebind("SELECT user_id, expires_at, password_hash FROM mfa_challenges WHERE token_hash = ?")
	var userID, passwordHash string
	var expiresAt time.Time
	if err := tx.QueryRowContext(ctx, q, tokenHash).Scan(&userID, &expiresAt, &passwordHash); err != nil {
		if errorsIs(err, sql.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	if !time.Now().UTC().Before(expiresAt) {
		return "", "", ErrSessionExpired
	}
	deleteQ := s.store.rebind("DELETE FROM mfa_challenges WHERE token_hash = ?")
	res, err := tx.ExecContext(ctx, deleteQ, tokenHash)
	if err != nil {
		return "", "", err
	}
	rows, err := res.RowsAffected()
	if err != nil || rows != 1 {
		return "", "", ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return userID, passwordHash, nil
}

// ---------------------------------------------------------------------
// Audit Store
// ---------------------------------------------------------------------

type auditStore struct {
	store *SQLStore
}

func (a *auditStore) LogAudit(ctx context.Context, r *AuditRecord) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	q := a.store.rebind(`
INSERT INTO audit_records (user_id, action, resource, details, ip_address, created_at)
VALUES (?, ?, ?, ?, ?, ?)
`)
	_, err := a.store.db.ExecContext(ctx, q, r.UserID, r.Action, r.Resource, r.Details, r.IPAddress, r.CreatedAt)
	return err
}

func (a *auditStore) ListAuditRecords(ctx context.Context, offset, limit int) ([]*AuditRecord, int, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var count int
	err := a.store.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM audit_records").Scan(&count)
	if err != nil {
		return nil, 0, err
	}

	q := a.store.rebind(`
SELECT id, user_id, action, resource, details, ip_address, created_at
FROM audit_records
ORDER BY created_at DESC LIMIT ? OFFSET ?
`)
	rows, err := a.store.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []*AuditRecord
	for rows.Next() {
		var r AuditRecord
		if err := rows.Scan(&r.ID, &r.UserID, &r.Action, &r.Resource, &r.Details, &r.IPAddress, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, &r)
	}
	return records, count, rows.Err()
}

// ---------------------------------------------------------------------
// Settings Store
// ---------------------------------------------------------------------

type settingsStore struct {
	store *SQLStore
}

func (s *settingsStore) GetSetting(ctx context.Context, key string) (string, error) {
	q := s.store.rebind("SELECT value FROM server_settings WHERE key = ?")
	var val string
	err := s.store.db.QueryRowContext(ctx, q, key).Scan(&val)
	if err != nil {
		if errorsIs(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return val, nil
}

func (s *settingsStore) SetSetting(ctx context.Context, key, val string) error {
	now := time.Now().UTC()
	q := s.store.rebind(`
INSERT INTO server_settings (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
`)
	_, err := s.store.db.ExecContext(ctx, q, key, val, now)
	return err
}

func (s *settingsStore) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.store.db.ExecContext(ctx, s.store.rebind(`DELETE FROM server_settings WHERE key = ?`), key)
	return err
}

func (s *settingsStore) GetAllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.store.db.QueryContext(ctx, "SELECT key, value FROM server_settings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			settings[k] = v
		}
	}
	return settings, rows.Err()
}

func errorsIs(err, target error) bool {
	if err == nil || target == nil {
		return err == target
	}
	return err == target || strings.Contains(err.Error(), target.Error())
}
