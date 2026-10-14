package main

import (
	"context"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func TestEnsureBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	t.Setenv("KYPULSE_ADMIN_PASSWORD", "BootstrapTestPass123!")
	st, err := store.Open(ctx, testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // the second start must not add another admin
		if err := ensureBootstrapAdmin(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := st.Users().CountUsers(ctx); err != nil || n != 1 {
		t.Fatalf("users = %d, %v; want exactly the bootstrap admin", n, err)
	}

	// An unreadable user count fails startup instead of reading as an empty database.
	st.Close()
	if err := ensureBootstrapAdmin(ctx, st); err == nil {
		t.Fatal("a failed CountUsers must stop startup")
	}
}
