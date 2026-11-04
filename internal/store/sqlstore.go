package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"slices"
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
	logs     *logStore
	sources  *sourceStore
}

// newSQLStore creates and initializes a SQLStore, running migrations automatically.
func newSQLStore(ctx context.Context, db *sql.DB, driver string, auditKey []byte) (*SQLStore, error) {
	driver = strings.ToLower(driver)
	if driver == "postgresql" {
		driver = "postgres"
	}

	applied, err := migrations.Run(ctx, db, driver)
	if err != nil {
		return nil, fmt.Errorf("migration failure on driver %s: %w", driver, err)
	}

	s := &SQLStore{
		db:     db,
		driver: driver,
	}

	s.users = &userStore{store: s}
	s.sessions = &sessionStore{store: s}
	s.audit = &auditStore{store: s, key: auditKey}
	s.settings = &settingsStore{store: s}
	s.targets = &targetStore{store: s}
	s.logs = &logStore{store: s}
	s.sources = &sourceStore{store: s}

	if err := s.users.ensureUsernameIndex(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	// A log with no digests is keyed only when migration 6 has just added the columns; any
	// later start that finds one means the chain was stripped.
	if err := s.audit.open(ctx, slices.Contains(applied, 6)); err != nil {
		_ = db.Close()
		return nil, err
	}

	return s, nil
}

func (s *SQLStore) Users() UserStore        { return s.users }
func (s *SQLStore) Sessions() SessionStore  { return s.sessions }
func (s *SQLStore) Audit() AuditStore       { return s.audit }
func (s *SQLStore) Settings() SettingsStore { return s.settings }
func (s *SQLStore) Targets() TargetStore    { return s.targets }
func (s *SQLStore) Logs() LogStore          { return s.logs }
func (s *SQLStore) Sources() SourceStore    { return s.sources }

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

	tx, err := u.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Refuse a case variant of a taken name even where existing duplicates keep the
	// case-insensitive unique index from being created.
	if err := u.lockUsernames(ctx, tx); err != nil {
		return err
	}
	var taken int
	if err := tx.QueryRowContext(ctx, u.store.rebind(`SELECT COUNT(1) FROM users WHERE LOWER(username) = LOWER(?)`), user.Username).Scan(&taken); err != nil {
		return err
	}
	if taken > 0 {
		return ErrAlreadyExists
	}
	_, err = tx.ExecContext(ctx, q,
		user.ID, user.Username, user.Email, user.DisplayName, user.PasswordHash,
		user.Role, user.Status, user.SSOProvider, user.SSOSubject,
		user.TOTPSecretEnc, user.TOTPEnabled, user.RecoveryCodesHash,
		user.PushDeviceID, user.MustChangePassword,
		user.CreatedAt, user.UpdatedAt, lastLogin,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return err
	}
	return tx.Commit()
}

// ensureUsernameIndex adds the case-insensitive unique username index at every start
// once no two names differ only in case. While some do, it renames nothing: the server
// runs, UsernameClashes reports them, and CreateUser/UpdateUser refuse new ones.
func (u *userStore) ensureUsernameIndex(ctx context.Context) error {
	clashes, err := u.UsernameClashes(ctx)
	if err != nil || len(clashes) > 0 {
		return err
	}
	_, err = u.store.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_lower ON users (LOWER(username))`)
	if err != nil && (isUniqueViolation(err) || strings.Contains(err.Error(), "could not create unique index")) {
		return nil // a clash committed since the check; the next start reports it
	}
	return err
}

// usernameLockKey serialises username writes on PostgreSQL, whose read committed check
// would otherwise let two transactions both see a name free while the index is absent.
// SQLite runs one connection, so writes are serial already.
const usernameLockKey int64 = 7345103

func (u *userStore) lockUsernames(ctx context.Context, tx *sql.Tx) error {
	if u.store.driver != "postgres" {
		return nil
	}
	_, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", usernameLockKey)
	return err
}

// lockUsernamesOn takes the same lock for a whole session on conn, for a writer whose
// transaction snapshot must postdate it. unlock releases it; if that fails the connection
// is discarded, which also releases it.
func (u *userStore) lockUsernamesOn(ctx context.Context, conn *sql.Conn) (unlock func(), err error) {
	if u.store.driver != "postgres" {
		return func() {}, nil
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", usernameLockKey); err != nil {
		return nil, err
	}
	return func() {
		if _, err := conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", usernameLockKey); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}, nil
}

func (u *userStore) UsernameClashes(ctx context.Context) ([][]string, error) {
	rows, err := u.store.db.QueryContext(ctx, `
SELECT a.username, b.username FROM users a JOIN users b
  ON LOWER(a.username) = LOWER(b.username) AND a.username < b.username
ORDER BY a.username, b.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var clashes [][]string
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		clashes = append(clashes, []string{a, b})
	}
	return clashes, rows.Err()
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

	// A renamed user may not take a case variant of another user's name; an unchanged
	// name passes, so an existing duplicate pair keeps working.
	q := u.store.rebind(`
UPDATE users SET
    username = ?, email = ?, display_name = ?, password_hash = ?,
    role = ?, status = ?, sso_provider = ?, sso_subject = ?,
    totp_secret_enc = ?, totp_enabled = ?, recovery_codes_hash = ?,
    push_device_id = ?, must_change_password = ?, updated_at = ?,
    last_login_at = ?
WHERE id = ? AND (username = ? OR NOT EXISTS (
    SELECT 1 FROM users other WHERE LOWER(other.username) = LOWER(?) AND other.id <> ?))
`)

	tx, err := u.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := u.lockUsernames(ctx, tx); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, q,
		user.Username, user.Email, user.DisplayName, user.PasswordHash,
		user.Role, user.Status, user.SSOProvider, user.SSOSubject,
		user.TOTPSecretEnc, user.TOTPEnabled, user.RecoveryCodesHash,
		user.PushDeviceID, user.MustChangePassword, user.UpdatedAt,
		lastLogin, user.ID, user.Username, user.Username, user.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		var exists int
		if err := tx.QueryRowContext(ctx, u.store.rebind(`SELECT COUNT(1) FROM users WHERE id = ?`), user.ID).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return ErrAlreadyExists
		}
		return ErrNotFound
	}
	return tx.Commit()
}

func (u *userStore) AdvanceSyncOrder(ctx context.Context, provider, subject string, at, floor int64) (bool, error) {
	if _, err := u.store.db.ExecContext(ctx, u.store.rebind(`DELETE FROM sso_sync_order WHERE latest < ?`), floor); err != nil {
		return false, err
	}
	res, err := u.store.db.ExecContext(ctx, u.store.rebind(`INSERT INTO sso_sync_order (provider, subject, latest) VALUES (?, ?, ?)
ON CONFLICT (provider, subject) DO UPDATE SET latest = excluded.latest WHERE sso_sync_order.latest <= excluded.latest`), provider, subject, at)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (u *userStore) ChangeRole(ctx context.Context, userID, from, to string) error {
	if !validRole(to) {
		return ErrInvalidRole
	}
	tx, err := u.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, u.store.rebind(`UPDATE users SET role = ?, updated_at = ? WHERE id = ? AND role = ? AND status = 'active'`), to, time.Now().UTC(), userID, from)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAlreadyExists
	}
	if _, err := tx.ExecContext(ctx, u.store.rebind(`DELETE FROM sessions WHERE user_id = ?`), userID); err != nil {
		return err
	}
	return tx.Commit()
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
// apply may return a non-nil finish, which withPassword calls with the commit outcome once
// the transaction is settled — that is how a caller chaining an audit row keeps the chain's
// lock held until its own write is durable or rolled back.
func (s *SQLStore) withPassword(ctx context.Context, userID, expectedHash string, apply func(*sql.Tx) (finish func(bool), err error)) error {
	return s.withGuard(ctx, "id = ? AND password_hash = ? AND status = 'active'", []any{userID, expectedHash}, apply)
}

// withCredentials is withPassword that also holds the TOTP state the caller checked: a grant
// earned without a second factor fails once TOTP is on, and one earned with it fails once
// TOTP is off or its secret changed.
func (s *SQLStore) withCredentials(ctx context.Context, userID string, c Credentials, apply func(*sql.Tx) (finish func(bool), err error)) error {
	return s.withGuard(ctx, "id = ? AND password_hash = ? AND status = 'active' AND totp_enabled = ? AND (totp_enabled = ? OR totp_secret_enc = ?)",
		[]any{userID, c.PasswordHash, c.TOTPEnabled, false, c.TOTPSecretEnc}, apply)
}

func (s *SQLStore) withGuard(ctx context.Context, where string, args []any, apply func(*sql.Tx) (finish func(bool), err error)) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, s.rebind("UPDATE users SET id = id WHERE "+where), args...)
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
	finish, err := apply(tx)
	if err != nil {
		return err
	}
	err = tx.Commit()
	if finish != nil {
		finish(err == nil)
	}
	return err
}

func (s *sessionStore) CreateSession(ctx context.Context, sess *Session, expected Credentials) error {
	err := s.store.withCredentials(ctx, sess.UserID, expected, func(tx *sql.Tx) (func(bool), error) {
		var stepUp sql.NullTime
		if sess.StepUpAt != nil {
			stepUp = sql.NullTime{Time: sess.StepUpAt.UTC(), Valid: true}
		}
		_, err := tx.ExecContext(ctx, s.store.rebind(`INSERT INTO sessions (token_hash, user_id, user_agent, ip_address, created_at, expires_at, sso_sid, step_up_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), sess.TokenHash, sess.UserID, sess.UserAgent, sess.IPAddress, sess.CreatedAt, sess.ExpiresAt, sess.SSOSID, stepUp)
		return nil, err
	})
	if err != nil {
		return err
	}
	// Check the fences only after the session committed: EndSSOSession and EndSSOSubject
	// commit their fence before deleting, so under read committed one of the two always
	// sees the other.
	now := time.Now().UTC()
	var fenced int
	if err := s.store.db.QueryRowContext(ctx, s.store.rebind(`SELECT
 (SELECT COUNT(1) FROM sso_logouts WHERE sid = ? AND sid <> '' AND expires_at > ?) +
 (SELECT COUNT(1) FROM sso_subject_logouts f JOIN users u ON u.sso_provider = f.provider AND u.sso_subject = f.subject
  WHERE u.id = ? AND f.cutoff > ? AND f.expires_at > ?)`), sess.SSOSID, now, sess.UserID, sess.CreatedAt.UTC(), now).Scan(&fenced); err != nil || fenced > 0 {
		_ = s.DeleteSession(ctx, sess.TokenHash)
		if err != nil {
			return err
		}
		return ErrSSOSessionEnded
	}
	return nil
}

func (s *sessionStore) EndSSOSession(ctx context.Context, sid string, fenceUntil time.Time) error {
	now := time.Now().UTC()
	if _, err := s.store.db.ExecContext(ctx, s.store.rebind(`DELETE FROM sso_logouts WHERE expires_at <= ?`), now); err != nil {
		return err
	}
	if _, err := s.store.db.ExecContext(ctx, s.store.rebind(`INSERT INTO sso_logouts (sid, expires_at) VALUES (?, ?) ON CONFLICT (sid) DO UPDATE SET expires_at = excluded.expires_at`), sid, fenceUntil.UTC()); err != nil {
		return err
	}
	_, err := s.store.db.ExecContext(ctx, s.store.rebind(`DELETE FROM sessions WHERE sso_sid = ?`), sid)
	return err
}

func (s *sessionStore) EndSSOSubject(ctx context.Context, provider, subject string, cutoff, fenceUntil time.Time) error {
	if _, err := s.store.db.ExecContext(ctx, s.store.rebind(`DELETE FROM sso_subject_logouts WHERE expires_at <= ?`), time.Now().UTC()); err != nil {
		return err
	}
	// Keyed by subject, not user, so it also fences a first login that is still provisioning.
	if _, err := s.store.db.ExecContext(ctx, s.store.rebind(`INSERT INTO sso_subject_logouts (provider, subject, cutoff, expires_at) VALUES (?, ?, ?, ?)
ON CONFLICT (provider, subject) DO UPDATE SET cutoff = excluded.cutoff, expires_at = excluded.expires_at WHERE sso_subject_logouts.cutoff < excluded.cutoff`), provider, subject, cutoff.UTC(), fenceUntil.UTC()); err != nil {
		return err
	}
	_, err := s.store.db.ExecContext(ctx, s.store.rebind(`DELETE FROM sessions WHERE created_at < ? AND user_id IN (SELECT id FROM users WHERE sso_provider = ? AND sso_subject = ?)`), cutoff.UTC(), provider, subject)
	return err
}

func (u *userStore) CompletePasswordChange(ctx context.Context, userID, oldHash, newHash, ip string) error {
	return u.store.withPassword(ctx, userID, oldHash, func(tx *sql.Tx) (func(bool), error) {
		now := time.Now().UTC()
		result, err := tx.ExecContext(ctx, u.store.rebind(`UPDATE users SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE id = ? AND sso_provider = 'local'`), newHash, false, now, userID)
		if err != nil {
			return nil, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, ErrNotFound
		}
		return u.revokePasswordGrants(ctx, tx, userID, "password replaced; sessions revoked", ip, now)
	})
}

// ResetAdminPassword is the operator recovery path, including disabled local accounts. It also
// turns TOTP off, so a lost or hostile second factor cannot shut the operator out.
func (u *userStore) ResetAdminPassword(ctx context.Context, userID, newHash string) error {
	tx, err := u.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, u.store.rebind(`UPDATE users SET password_hash = ?, must_change_password = ?, status = 'active', role = 'admin', totp_enabled = ?, totp_secret_enc = '', recovery_codes_hash = '[]', updated_at = ? WHERE id = ? AND sso_provider = 'local'`), newHash, true, false, now, userID)
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
	finish, err := u.revokePasswordGrants(ctx, tx, userID, "operator reset; sessions revoked", "", now)
	if err != nil {
		return err
	}
	err = tx.Commit()
	finish(err == nil)
	return err
}

// revokePasswordGrants deletes sessions and MFA challenges and chains the audit row on tx.
// finish must be called by the caller with the transaction's commit outcome once it is
// settled; it releases the audit chain lock this call took.
func (u *userStore) revokePasswordGrants(ctx context.Context, tx *sql.Tx, userID, details, ip string, now time.Time) (finish func(bool), err error) {
	for _, table := range []string{"sessions", "mfa_challenges"} {
		if _, err := tx.ExecContext(ctx, u.store.rebind("DELETE FROM "+table+" WHERE user_id = ?"), userID); err != nil {
			return nil, err
		}
	}
	return u.store.audit.append(ctx, tx, &AuditRecord{UserID: userID, Action: "auth.password_changed", Resource: "user", Details: details, IPAddress: ip, CreatedAt: now})
}

func (s *sessionStore) GetSession(ctx context.Context, tokenHash string) (*Session, error) {
	q := s.store.rebind(`
SELECT token_hash, user_id, user_agent, ip_address, created_at, expires_at, step_up_at
FROM sessions WHERE token_hash = ?
`)
	var sess Session
	var stepUp sql.NullTime
	err := s.store.db.QueryRowContext(ctx, q, tokenHash).Scan(
		&sess.TokenHash, &sess.UserID, &sess.UserAgent, &sess.IPAddress,
		&sess.CreatedAt, &sess.ExpiresAt, &stepUp,
	)
	if stepUp.Valid {
		sess.StepUpAt = &stepUp.Time
	}
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
