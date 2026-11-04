package store_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
	"github.com/google/uuid"
)

func rawExec(t *testing.T, driver, dsn string, statements ...string) {
	t.Helper()
	name := "sqlite"
	if driver == "postgres" {
		name = "pgx"
	}
	db, err := sql.Open(name, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range statements {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func newUser(name string) *store.User {
	return &store.User{ID: uuid.NewString(), Username: name, Role: store.RoleViewer, Status: "active", SSOProvider: "local"}
}

// Q5 C: the case-insensitive unique index is created only when no names clash; until
// then the server runs, reports the clashes and refuses new case variants, renaming nothing.
func TestUsernameCaseVariants(t *testing.T) {
	ctx := context.Background()
	cfg := testdb.Config(t)
	st, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Users().CreateUser(ctx, newUser("admin")); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().CreateUser(ctx, newUser("Admin")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("case variant with the index: got %v, want ErrAlreadyExists", err)
	}
	other := newUser("other")
	if err := st.Users().CreateUser(ctx, other); err != nil {
		t.Fatal(err)
	}
	st.Close()

	// An install from before the index, with a clash already in it.
	now := time.Now().UTC().Format(time.RFC3339)
	rawExec(t, cfg.Driver, cfg.DSN, `DROP INDEX idx_users_username_lower`,
		`INSERT INTO users (id, username, role, status, sso_provider, created_at, updated_at) VALUES ('`+uuid.NewString()+`', 'ADMIN', 'viewer', 'active', 'kysignon', '`+now+`', '`+now+`')`)

	st, err = store.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("a clash must not stop the server: %v", err)
	}
	clashes, err := st.Users().UsernameClashes(ctx)
	if err != nil || len(clashes) != 1 || clashes[0][0] != "ADMIN" || clashes[0][1] != "admin" {
		t.Fatalf("clashes: got %v %v", clashes, err)
	}
	if err := st.Users().CreateUser(ctx, newUser("aDmin")); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("new case variant without the index: got %v, want ErrAlreadyExists", err)
	}
	other.Username = "Admin"
	if err := st.Users().UpdateUser(ctx, other); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("rename to a case variant: got %v, want ErrAlreadyExists", err)
	}
	existing, err := st.Users().GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	existing.DisplayName = "unchanged name still updates"
	if err := st.Users().UpdateUser(ctx, existing); err != nil {
		t.Fatalf("update of an existing clashing user: %v", err)
	}
	other.Username = "other2"
	if err := st.Users().UpdateUser(ctx, other); err != nil {
		t.Fatalf("plain rename: %v", err)
	}
	st.Close()

	// Once the operator resolves the clash, the next start creates the index.
	rawExec(t, cfg.Driver, cfg.DSN, `DELETE FROM users WHERE username = 'ADMIN'`)
	st, err = store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	db := "sqlite"
	if cfg.Driver == "postgres" {
		db = "pgx"
	}
	conn, err := sql.Open(db, cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`INSERT INTO users (id, username, role, status, sso_provider, created_at, updated_at) VALUES ('` + uuid.NewString() + `', 'ADMIN', 'viewer', 'active', 'local', '` + now + `', '` + now + `')`); err == nil {
		t.Fatal("index not created after the clash was resolved")
	}
}
