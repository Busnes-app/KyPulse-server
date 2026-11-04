package store_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
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

// While a legacy clash keeps the index away, concurrent writers still cannot add a new
// case variant: on PostgreSQL the check would otherwise race under read committed.
func TestConcurrentCaseVariantsWithoutTheIndex(t *testing.T) {
	ctx := context.Background()
	cfg := testdb.Config(t)
	st, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Users().CreateUser(ctx, newUser("admin")); err != nil {
		t.Fatal(err)
	}
	st.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	rawExec(t, cfg.Driver, cfg.DSN, `DROP INDEX idx_users_username_lower`,
		`INSERT INTO users (id, username, role, status, sso_provider, created_at, updated_at) VALUES ('`+uuid.NewString()+`', 'ADMIN', 'viewer', 'active', 'kysignon', '`+now+`', '`+now+`')`)
	st, err = store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	names := []string{"bob", "Bob", "BOB", "bOb", "boB", "BOb", "bOB", "BoB"}
	results := make(chan error, len(names))
	var start sync.WaitGroup
	start.Add(1)
	for _, n := range names {
		go func() { start.Wait(); results <- st.Users().CreateUser(ctx, newUser(n)) }()
	}
	start.Done()
	created := 0
	for range names {
		if err := <-results; err == nil {
			created++
		} else if !errors.Is(err, store.ErrAlreadyExists) {
			t.Fatal(err)
		}
	}
	if created != 1 {
		t.Fatalf("%d case variants of bob created, want 1", created)
	}
}

// The installer's identity insert takes the same lock, so it cannot race an ordinary
// provisioning of a case variant while the index is absent. It holds that lock on one
// connection, so even a pool of one or two connections never deadlocks.
func TestInstallerAndProvisioningCannotBothTakeACaseVariant(t *testing.T) {
	for _, pool := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("pool=%d", pool), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cfg := testdb.Config(t)
			cfg.MaxOpenConns = pool
			st, err := store.Open(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := st.Users().CreateUser(ctx, newUser("admin")); err != nil {
				t.Fatal(err)
			}
			st.Close()
			now := time.Now().UTC().Format(time.RFC3339)
			rawExec(t, cfg.Driver, cfg.DSN, `DROP INDEX idx_users_username_lower`,
				`INSERT INTO users (id, username, role, status, sso_provider, created_at, updated_at) VALUES ('`+uuid.NewString()+`', 'ADMIN', 'viewer', 'active', 'kysignon', '`+now+`', '`+now+`')`)
			st, err = store.Open(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			for i := range 10 {
				name := fmt.Sprintf("bob%d", i)
				b := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530",
					Everyday: []applysetup.Identity{{Issuer: "https://identity.test", Subject: "sub-" + name, Username: strings.ToUpper(name)}}}
				var wg sync.WaitGroup
				var applyErr, createErr error
				wg.Add(2)
				go func() {
					defer wg.Done()
					_, applyErr = st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, false)
				}()
				go func() { defer wg.Done(); createErr = st.Users().CreateUser(ctx, newUser(name)) }()
				wg.Wait()
				if ctx.Err() != nil {
					t.Fatalf("round %d: deadlocked (installer %v, create %v)", i, applyErr, createErr)
				}
				if (applyErr == nil) == (createErr == nil) {
					t.Fatalf("round %d: want exactly one of installer (%v) and provisioning (%v) to succeed", i, applyErr, createErr)
				}
				if clashes, err := st.Users().UsernameClashes(ctx); err != nil || len(clashes) != 1 {
					t.Fatalf("round %d: installer and provisioning both created %s: %v %v", i, name, clashes, err)
				}
			}
		})
	}
}
