package migrations

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/testdb"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// An SSO session from before migration 12 carries no sid, so no back-channel logout could
// end it: migration 12 drops those SSO sessions and keeps local sign-ins. It runs once, so a
// later start keeps an SSO session issued without a sid after the upgrade.
func TestMigration12DropsUnreachableSSOSessions(t *testing.T) {
	cfg := testdb.Config(t)
	driverName := "sqlite"
	if cfg.Driver == "postgres" {
		driverName = "pgx"
	}
	db, err := sql.Open(driverName, cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	q := func(query string) string {
		if cfg.Driver != "postgres" {
			return query
		}
		for i := 1; strings.Contains(query, "?"); i++ {
			query = strings.Replace(query, "?", "$"+strconv.Itoa(i), 1)
		}
		return query
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q(query), args...); err != nil {
			t.Fatal(err)
		}
	}
	all := registry
	t.Cleanup(func() { registry = all })
	runThrough := func(version int) []int {
		t.Helper()
		for i, m := range all {
			if m.Version == version {
				registry = all[:i+1]
			}
		}
		applied, err := Run(ctx, db, cfg.Driver)
		if err != nil {
			t.Fatal(err)
		}
		return applied
	}
	sessions := func(want map[string]int) {
		t.Helper()
		for token, n := range want {
			var got int
			if err := db.QueryRowContext(ctx, q(`SELECT COUNT(1) FROM sessions WHERE token_hash = ?`), token).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != n {
				t.Errorf("session %s: got %d rows, want %d", token, got, n)
			}
		}
	}

	now := time.Now().UTC()
	runThrough(11)
	for _, u := range []struct{ id, provider string }{{"usr_local", "local"}, {"usr_sso", "kysignon"}} {
		exec(`INSERT INTO users (id, username, sso_provider, sso_subject, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, u.id, u.id, u.provider, u.id, now, now)
		exec(`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`, "legacy_"+u.id, u.id, now, now.Add(time.Hour))
	}
	if applied := runThrough(12); len(applied) != 1 || applied[0] != 12 {
		t.Fatalf("upgrade applied %v, want [12]", applied)
	}
	sessions(map[string]int{"legacy_usr_local": 1, "legacy_usr_sso": 0})

	// A later start applies nothing, so it never signs out a session issued after the upgrade.
	exec(`INSERT INTO sessions (token_hash, user_id, created_at, expires_at, sso_sid) VALUES (?, ?, ?, ?, ?)`, "after_no_sid", "usr_sso", now, now.Add(time.Hour), "")
	exec(`INSERT INTO sessions (token_hash, user_id, created_at, expires_at, sso_sid) VALUES (?, ?, ?, ?, ?)`, "after_sid", "usr_sso", now, now.Add(time.Hour), "sid-1")
	registry = all
	if applied, err := Run(ctx, db, cfg.Driver); err != nil || len(applied) != 0 {
		t.Fatalf("second start applied %v, %v; want none", applied, err)
	}
	sessions(map[string]int{"legacy_usr_local": 1, "after_no_sid": 1, "after_sid": 1})
}
