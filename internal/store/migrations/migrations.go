package migrations

import (
	"context"
	"database/sql"
	sqldriver "database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// Migration represents an incremental schema change step.
type Migration struct {
	Version  int
	Name     string
	SQLite   string
	Postgres string
}

var registry = []Migration{
	{
		Version: 1,
		Name:    "initial_schema",
		SQLite: `
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT 'viewer',
    status TEXT NOT NULL DEFAULT 'active',
    sso_provider TEXT NOT NULL DEFAULT 'local',
    sso_subject TEXT NOT NULL DEFAULT '',
    totp_secret_enc TEXT NOT NULL DEFAULT '',
    totp_enabled INTEGER NOT NULL DEFAULT 0,
    recovery_codes_hash TEXT NOT NULL DEFAULT '[]',
    push_device_id TEXT NOT NULL DEFAULT '',
    must_change_password INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    last_login_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_sso ON users(sso_provider, sso_subject);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_agent TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS audit_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_user_created ON audit_records(user_id, created_at);

CREATE TABLE IF NOT EXISTS server_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL
);
`,
		Postgres: `
CREATE TABLE IF NOT EXISTS users (
    id VARCHAR(64) PRIMARY KEY,
    username VARCHAR(128) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL DEFAULT '',
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    role VARCHAR(32) NOT NULL DEFAULT 'viewer',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    sso_provider VARCHAR(32) NOT NULL DEFAULT 'local',
    sso_subject VARCHAR(255) NOT NULL DEFAULT '',
    totp_secret_enc TEXT NOT NULL DEFAULT '',
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    recovery_codes_hash TEXT NOT NULL DEFAULT '[]',
    push_device_id VARCHAR(255) NOT NULL DEFAULT '',
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    last_login_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_sso ON users(sso_provider, sso_subject);

CREATE TABLE IF NOT EXISTS sessions (
    token_hash VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_agent TEXT NOT NULL DEFAULT '',
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS audit_records (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL DEFAULT '',
    action VARCHAR(64) NOT NULL,
    resource VARCHAR(255) NOT NULL DEFAULT '',
    details TEXT NOT NULL DEFAULT '',
    ip_address VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_user_created ON audit_records(user_id, created_at);

CREATE TABLE IF NOT EXISTS server_settings (
    key VARCHAR(128) PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL
);
`,
	},
	{
		Version: 2,
		Name:    "one_time_auth_challenges",
		SQLite: `
CREATE TABLE IF NOT EXISTS mfa_challenges (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mfa_challenges_expires ON mfa_challenges(expires_at);
`,
		Postgres: `
CREATE TABLE IF NOT EXISTS mfa_challenges (
    token_hash VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mfa_challenges_expires ON mfa_challenges(expires_at);
`,
	},
	{
		Version:  3,
		Name:     "totp_last_counter",
		SQLite:   `ALTER TABLE users ADD COLUMN totp_last_counter INTEGER NOT NULL DEFAULT 0;`,
		Postgres: `ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_counter BIGINT NOT NULL DEFAULT 0;`,
	},
	{
		Version: 4,
		Name:    "mfa_credential_snapshot",
		SQLite: `DELETE FROM mfa_challenges;
ALTER TABLE mfa_challenges ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';`,
		Postgres: `DELETE FROM mfa_challenges;
ALTER TABLE mfa_challenges ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';`,
	},
	{
		Version: 5,
		Name:    "targets",
		SQLite: `
CREATE TABLE IF NOT EXISTS targets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    interval_sec INTEGER NOT NULL DEFAULT 30,
    enabled INTEGER NOT NULL DEFAULT 1,
    container TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'pending',
    state_since DATETIME NOT NULL,
    cause TEXT NOT NULL DEFAULT '',
    silenced_until DATETIME,
    until_fixed INTEGER NOT NULL DEFAULT 0,
    track_json TEXT NOT NULL DEFAULT '',
    last_result TEXT NOT NULL DEFAULT '',
    last_polled_at DATETIME,
    last_latency_ms INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS target_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    at DATETIME NOT NULL,
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
    cause TEXT NOT NULL DEFAULT '',
    reminder INTEGER NOT NULL DEFAULT 0,
    notified INTEGER NOT NULL DEFAULT 0,
    notify_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_target_events_target_at ON target_events(target_id, at);
CREATE INDEX IF NOT EXISTS idx_target_events_at ON target_events(at);
`,
		Postgres: `
CREATE TABLE IF NOT EXISTS targets (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    url TEXT NOT NULL,
    interval_sec INTEGER NOT NULL DEFAULT 30,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    container VARCHAR(255) NOT NULL DEFAULT '',
    state VARCHAR(16) NOT NULL DEFAULT 'pending',
    state_since TIMESTAMPTZ NOT NULL,
    cause VARCHAR(64) NOT NULL DEFAULT '',
    silenced_until TIMESTAMPTZ,
    until_fixed BOOLEAN NOT NULL DEFAULT FALSE,
    track_json TEXT NOT NULL DEFAULT '',
    last_result TEXT NOT NULL DEFAULT '',
    last_polled_at TIMESTAMPTZ,
    last_latency_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS target_events (
    id BIGSERIAL PRIMARY KEY,
    target_id VARCHAR(64) NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    at TIMESTAMPTZ NOT NULL,
    from_state VARCHAR(16) NOT NULL,
    to_state VARCHAR(16) NOT NULL,
    cause VARCHAR(64) NOT NULL DEFAULT '',
    reminder BOOLEAN NOT NULL DEFAULT FALSE,
    notified BOOLEAN NOT NULL DEFAULT FALSE,
    notify_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_target_events_target_at ON target_events(target_id, at);
CREATE INDEX IF NOT EXISTS idx_target_events_at ON target_events(at);
`,
	},
	{
		Version: 6,
		Name:    "audit_chain",
		SQLite: `
ALTER TABLE audit_records ADD COLUMN seq INTEGER;
ALTER TABLE audit_records ADD COLUMN prev_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_records ADD COLUMN hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_seq ON audit_records(seq);
`,
		Postgres: `
ALTER TABLE audit_records ADD COLUMN seq BIGINT;
ALTER TABLE audit_records ADD COLUMN prev_hash VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE audit_records ADD COLUMN hash VARCHAR(64) NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_seq ON audit_records(seq);
`,
	},
	{
		Version: 7,
		Name:    "collected_logs",
		SQLite: `
CREATE TABLE log_lines (
 id INTEGER PRIMARY KEY AUTOINCREMENT, time DATETIME NOT NULL, received_at DATETIME NOT NULL,
 source_id TEXT NOT NULL DEFAULT '', source TEXT NOT NULL, target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
 app TEXT NOT NULL DEFAULT '', level TEXT NOT NULL DEFAULT '', event TEXT NOT NULL DEFAULT '',
 message TEXT NOT NULL DEFAULT '', raw TEXT NOT NULL, truncated INTEGER NOT NULL DEFAULT 0, bytes INTEGER NOT NULL
);
CREATE INDEX idx_log_lines_target_app_id ON log_lines(target_id, app, id);
CREATE INDEX idx_log_lines_received_id ON log_lines(received_at, id);
CREATE TABLE activity (
 id INTEGER PRIMARY KEY AUTOINCREMENT, time DATETIME NOT NULL, received_at DATETIME NOT NULL,
 source_id TEXT NOT NULL DEFAULT '', target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
 app TEXT NOT NULL DEFAULT '', actor TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
 target TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL DEFAULT '',
 external_key TEXT NOT NULL DEFAULT '', bytes INTEGER NOT NULL
);
CREATE INDEX idx_activity_target_app_id ON activity(target_id, app, id);
CREATE INDEX idx_activity_received_id ON activity(received_at, id);
CREATE UNIQUE INDEX idx_activity_external_key ON activity(external_key) WHERE external_key <> '';
CREATE TABLE log_usage (id INTEGER PRIMARY KEY CHECK(id=1), bytes INTEGER NOT NULL DEFAULT 0);
INSERT INTO log_usage(id, bytes) VALUES (1, 0);
`,
		Postgres: `
CREATE TABLE log_lines (
 id BIGSERIAL PRIMARY KEY, time TIMESTAMPTZ NOT NULL, received_at TIMESTAMPTZ NOT NULL,
 source_id TEXT NOT NULL DEFAULT '', source TEXT NOT NULL, target_id VARCHAR(64) REFERENCES targets(id) ON DELETE SET NULL,
 app TEXT NOT NULL DEFAULT '', level TEXT NOT NULL DEFAULT '', event TEXT NOT NULL DEFAULT '',
 message TEXT NOT NULL DEFAULT '', raw TEXT NOT NULL, truncated BOOLEAN NOT NULL DEFAULT FALSE, bytes BIGINT NOT NULL
);
CREATE INDEX idx_log_lines_target_app_id ON log_lines(target_id, app, id);
CREATE INDEX idx_log_lines_received_id ON log_lines(received_at, id);
CREATE TABLE activity (
 id BIGSERIAL PRIMARY KEY, time TIMESTAMPTZ NOT NULL, received_at TIMESTAMPTZ NOT NULL,
 source_id TEXT NOT NULL DEFAULT '', target_id VARCHAR(64) REFERENCES targets(id) ON DELETE SET NULL,
 app TEXT NOT NULL DEFAULT '', actor TEXT NOT NULL DEFAULT '', action TEXT NOT NULL,
 target TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL DEFAULT '',
 external_key TEXT NOT NULL DEFAULT '', bytes BIGINT NOT NULL
);
CREATE INDEX idx_activity_target_app_id ON activity(target_id, app, id);
CREATE INDEX idx_activity_received_id ON activity(received_at, id);
CREATE UNIQUE INDEX idx_activity_external_key ON activity(external_key) WHERE external_key <> '';
CREATE TABLE log_usage (id INTEGER PRIMARY KEY CHECK(id=1), bytes BIGINT NOT NULL DEFAULT 0);
INSERT INTO log_usage(id, bytes) VALUES (1, 0);
`,
	},
	{
		Version: 8,
		Name:    "log_sources",
		SQLite: `
CREATE TABLE log_sources (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, target_id TEXT REFERENCES targets(id) ON DELETE SET NULL,
 token_hash TEXT NOT NULL UNIQUE, created_at DATETIME NOT NULL, revoked_at DATETIME
);
CREATE TABLE log_pairing_codes (
 code_hash TEXT PRIMARY KEY, target_id TEXT REFERENCES targets(id) ON DELETE CASCADE,
 expires_at DATETIME NOT NULL
);
CREATE INDEX idx_log_pairing_codes_expires ON log_pairing_codes(expires_at);
`,
		Postgres: `
CREATE TABLE log_sources (
 id VARCHAR(64) PRIMARY KEY, name VARCHAR(64) NOT NULL, target_id VARCHAR(64) REFERENCES targets(id) ON DELETE SET NULL,
 token_hash VARCHAR(64) NOT NULL UNIQUE, created_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ
);
CREATE TABLE log_pairing_codes (
 code_hash VARCHAR(64) PRIMARY KEY, target_id VARCHAR(64) REFERENCES targets(id) ON DELETE CASCADE,
 expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_log_pairing_codes_expires ON log_pairing_codes(expires_at);
`,
	},
	{Version: 9, Name: "log_cursors",
		SQLite:   `CREATE TABLE log_cursors (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
		Postgres: `CREATE TABLE log_cursors (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
	},
	{Version: 10, Name: "log_source_receipts",
		SQLite: `ALTER TABLE log_sources ADD COLUMN ingest_seq BIGINT NOT NULL DEFAULT 0;
ALTER TABLE log_sources ADD COLUMN ingest_digest TEXT NOT NULL DEFAULT '';`,
		Postgres: `ALTER TABLE log_sources ADD COLUMN ingest_seq BIGINT NOT NULL DEFAULT 0;
ALTER TABLE log_sources ADD COLUMN ingest_digest TEXT NOT NULL DEFAULT '';`,
	},
	{Version: 11, Name: "offboarding_alert_sources",
		SQLite: `ALTER TABLE log_sources ADD COLUMN offboarding_alerts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE log_pairing_codes ADD COLUMN offboarding_alerts INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_pending_offboarding_alerts ON target_events(id) WHERE cause='offboarding_failed' AND notify_error='pending' AND notified=FALSE;`,
		Postgres: `ALTER TABLE log_sources ADD COLUMN offboarding_alerts BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE log_pairing_codes ADD COLUMN offboarding_alerts BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX idx_pending_offboarding_alerts ON target_events(id) WHERE cause='offboarding_failed' AND notify_error='pending' AND notified=FALSE;`,
	},
	// sso_sid ties a session to the identity provider's login. sso_logouts fences a sid,
	// and sso_subject_logouts a subject's logins issued before cutoff, that back-channel
	// logout ended, so a callback still in flight cannot revive them. An SSO session from
	// before this migration has no sid, so no logout could reach it: it is signed out here,
	// once; local password sessions stay.
	{Version: 12, Name: "backchannel_logout",
		SQLite: `ALTER TABLE sessions ADD COLUMN sso_sid TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_sessions_sso_sid ON sessions(sso_sid) WHERE sso_sid <> '';
CREATE TABLE sso_logouts (sid TEXT PRIMARY KEY, expires_at DATETIME NOT NULL);
CREATE TABLE sso_subject_logouts (provider TEXT NOT NULL, subject TEXT NOT NULL, cutoff DATETIME NOT NULL, expires_at DATETIME NOT NULL, PRIMARY KEY (provider, subject));
DELETE FROM sessions WHERE sso_sid = '' AND user_id IN (SELECT id FROM users WHERE sso_provider <> 'local');`,
		Postgres: `ALTER TABLE sessions ADD COLUMN sso_sid TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_sessions_sso_sid ON sessions(sso_sid) WHERE sso_sid <> '';
CREATE TABLE sso_logouts (sid TEXT PRIMARY KEY, expires_at TIMESTAMPTZ NOT NULL);
CREATE TABLE sso_subject_logouts (provider TEXT NOT NULL, subject TEXT NOT NULL, cutoff TIMESTAMPTZ NOT NULL, expires_at TIMESTAMPTZ NOT NULL, PRIMARY KEY (provider, subject));
DELETE FROM sessions WHERE sso_sid = '' AND user_id IN (SELECT id FROM users WHERE sso_provider <> 'local');`,
	},
}

// lockKey is the Postgres session advisory lock Run holds, so replicas starting together
// migrate one at a time and each re-reads what the other applied.
const lockKey int64 = 7345102

// Run executes all pending migrations for the specified database driver and returns the
// versions it applied in this call, empty when the schema was already current.
func Run(ctx context.Context, db *sql.DB, driver string) (applied []int, err error) {
	driver = strings.ToLower(driver)
	if driver == "postgresql" {
		driver = "postgres"
	}

	// One pinned connection: a session advisory lock belongs to the connection that took it.
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get a migration connection: %w", err)
	}
	defer conn.Close()
	if driver == "postgres" {
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
			return nil, fmt.Errorf("failed to take the migration lock: %w", err)
		}
		defer func() {
			// A connection that may still hold the lock is discarded, never pooled.
			if _, uerr := conn.ExecContext(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockKey); uerr != nil {
				_ = conn.Raw(func(any) error { return sqldriver.ErrBadConn })
			}
		}()
	}

	// Create schema_migrations table
	initTableQuery := `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at DATETIME NOT NULL
);`
	if driver == "postgres" {
		initTableQuery = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL
);`
	}

	if _, err := conn.ExecContext(ctx, initTableQuery); err != nil {
		return nil, fmt.Errorf("failed to init schema_migrations: %w", err)
	}

	for _, m := range registry {
		var exists int
		err := conn.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = $1", m.Version).Scan(&exists)
		if err != nil {
			// Try SQLite positional ? parameter if $1 failed
			err = conn.QueryRowContext(ctx, "SELECT COUNT(1) FROM schema_migrations WHERE version = ?", m.Version).Scan(&exists)
			if err != nil {
				return nil, fmt.Errorf("failed to check migration version %d: %w", m.Version, err)
			}
		}

		if exists > 0 {
			continue
		}

		ddl := m.SQLite
		if driver == "postgres" {
			ddl = m.Postgres
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to begin migration tx for v%d: %w", m.Version, err)
		}

		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed executing migration v%d (%s): %w", m.Version, m.Name, err)
		}

		recordQuery := "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)"
		if driver == "postgres" {
			recordQuery = "INSERT INTO schema_migrations (version, name, applied_at) VALUES ($1, $2, $3)"
		}

		if _, err := tx.ExecContext(ctx, recordQuery, m.Version, m.Name, time.Now().UTC()); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("failed to record migration v%d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit migration v%d: %w", m.Version, err)
		}
		applied = append(applied, m.Version)
	}

	return applied, nil
}
