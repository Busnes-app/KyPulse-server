package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/password"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func bootstrapLogger(t *testing.T) (*logging.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	lg, err := logging.New(logging.Config{App: "kypulse", Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	return lg, &buf
}

func openBootstrapStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestEnsureBootstrapAdmin(t *testing.T) {
	ctx := context.Background()
	t.Setenv("KYPULSE_ADMIN_PASSWORD", "BootstrapTestPass123!")
	lg, logs := bootstrapLogger(t)
	dir := t.TempDir()
	st, err := store.Open(ctx, testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // the second start must not add another admin
		if err := ensureBootstrapAdmin(ctx, st, dir, lg); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := st.Users().CountUsers(ctx); err != nil || n != 1 {
		t.Fatalf("users = %d, %v; want exactly the bootstrap admin", n, err)
	}
	// A chosen password is the operator's already: no file, and never the log.
	if _, err := os.Stat(filepath.Join(dir, initialAdminPasswordFile)); !os.IsNotExist(err) {
		t.Errorf("KYPULSE_ADMIN_PASSWORD still wrote a password file: %v", err)
	}
	if strings.Contains(logs.String(), "BootstrapTestPass123!") {
		t.Errorf("log carries the chosen password: %s", logs)
	}

	// An unreadable user count fails startup instead of reading as an empty database.
	st.Close()
	if err := ensureBootstrapAdmin(ctx, st, dir, lg); err == nil {
		t.Fatal("a failed CountUsers must stop startup")
	}
}

// The generated password goes to a 0600 file in the data directory, never the log, and a
// retry after a start that died before the account was written reuses it.
func TestEnsureBootstrapAdminWritesThePasswordToAFileNotTheLog(t *testing.T) {
	ctx := context.Background()
	t.Setenv("KYPULSE_ADMIN_PASSWORD", "")
	lg, logs := bootstrapLogger(t)
	dir := filepath.Join(t.TempDir(), "data") // not yet created
	path := filepath.Join(dir, initialAdminPasswordFile)

	// A previous start wrote the file, then died before CreateUser.
	if _, err := initialAdminPassword(dir, path); err != nil {
		t.Fatal(err)
	}
	st := openBootstrapStore(t)
	if err := ensureBootstrapAdmin(ctx, st, dir, lg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no password file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("password file mode %o, want 0600", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pw := strings.TrimSpace(string(raw))
	u, err := st.Users().GetUserByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := password.Verify(pw, u.PasswordHash); !ok || err != nil {
		t.Fatalf("the file's password does not open the admin account: %v", err)
	}
	if !u.MustChangePassword {
		t.Error("bootstrap admin must change the generated password at first sign-in")
	}
	if pw == "" || strings.Contains(logs.String(), pw) {
		t.Errorf("log carries the generated password: %s", logs)
	}
	if !strings.Contains(logs.String(), "initial_admin_password_file") || !strings.Contains(logs.String(), path) {
		t.Errorf("log must name the password file: %s", logs)
	}

	// Users exist now: a restart leaves the account and the file alone.
	if err := ensureBootstrapAdmin(ctx, st, dir, lg); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.Users().CountUsers(ctx); n != 1 {
		t.Errorf("restart created another account: %d users", n)
	}
	if again, _ := os.ReadFile(path); string(again) != string(raw) {
		t.Error("restart rewrote the password file")
	}
}

// A data directory the file cannot be written into stops startup: no admin, and no password
// falls back to the log. A regular file in the data directory's place fails even as root.
func TestEnsureBootstrapAdminFailsWhenThePasswordFileCannotBeWritten(t *testing.T) {
	ctx := context.Background()
	t.Setenv("KYPULSE_ADMIN_PASSWORD", "")
	lg, logs := bootstrapLogger(t)
	dir := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	st := openBootstrapStore(t)
	if err := ensureBootstrapAdmin(ctx, st, dir, lg); err == nil {
		t.Fatal("an unwritable password file must stop startup")
	}
	if n, err := st.Users().CountUsers(ctx); err != nil || n != 0 {
		t.Fatalf("users = %d, %v; want none when the password could not be stored", n, err)
	}
	if logs.Len() != 0 {
		t.Errorf("a failed bootstrap logged: %s", logs)
	}

	// A leftover empty file is refused rather than silently replaced.
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, initialAdminPasswordFile), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureBootstrapAdmin(ctx, st, empty, lg); err == nil {
		t.Fatal("an empty password file must stop startup")
	}
}
