package store

import (
	"context"
	"path/filepath"
	"testing"
)

// A DSN that sets one pragma of its own still gets the other defaults, foreign keys above all.
func TestSQLiteCustomPragmaKeepsForeignKeys(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "kypulse.db") + "?_pragma=busy_timeout(10000)"
	st, err := openSQLite(context.Background(), dsn, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	db := st.(*SQLStore).db
	var fk, timeout int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, %v", fk, err)
	}
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout != 10000 {
		t.Fatalf("busy_timeout = %d, %v: the DSN's own value must win", timeout, err)
	}
}
