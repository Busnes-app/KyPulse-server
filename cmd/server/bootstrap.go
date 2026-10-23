package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/password"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// initialAdminPasswordFile holds the generated bootstrap password, never the log.
const initialAdminPasswordFile = "initial-admin-password"

// initialAdminPasswordWritten names the file, never the password. Warn, so KY_LOG_LEVEL=warn
// does not hide where the operator's first sign-in password is.
var initialAdminPasswordWritten = logging.DeclareEvent("initial_admin_password_file",
	`created the "admin" account; its one-time password is in this file: sign in, replace it, then delete the file`,
	slog.LevelWarn)

// ensureBootstrapAdmin creates the first admin on an empty database. A failed count is an
// error, never zero: guessing "empty" would mint a second admin on a database that has one.
//
// The password is KYPULSE_ADMIN_PASSWORD, or one generated into
// <dataDir>/initial-admin-password (0600). A file that cannot be written fails startup; the
// password never falls back to the log. A file left by a start that died before the account
// was written is reused, so a retry never strands a password nobody can read.
func ensureBootstrapAdmin(ctx context.Context, st store.Store, dataDir string, lg *logging.Logger) error {
	count, err := st.Users().CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	adminPass := os.Getenv("KYPULSE_ADMIN_PASSWORD")
	generatedAt := ""
	if adminPass == "" {
		generatedAt = filepath.Join(dataDir, initialAdminPasswordFile)
		if adminPass, err = initialAdminPassword(dataDir, generatedAt); err != nil {
			return fmt.Errorf("failed to write the initial admin password file: %w", err)
		}
	}
	hash, err := password.Hash(adminPass)
	if err != nil {
		return fmt.Errorf("failed to hash bootstrap admin password: %w", err)
	}
	if err := st.Users().CreateUser(ctx, &store.User{
		ID:                 fmt.Sprintf("usr_%s", crypto.RandomHex(12)),
		Username:           "admin",
		DisplayName:        "Administrator",
		PasswordHash:       hash,
		Role:               "admin",
		Status:             "active",
		SSOProvider:        "local",
		MustChangePassword: true,
	}); err != nil {
		return fmt.Errorf("failed to create bootstrap admin: %w", err)
	}
	if generatedAt != "" {
		lg.Log(ctx, initialAdminPasswordWritten, fKeyPath(generatedAt))
	}
	return nil
}

// initialAdminPassword creates path exclusively with a fresh password, or returns the one
// already in it.
func initialAdminPassword(dataDir, path string) (string, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		raw, err := os.ReadFile(path)
		if pw := strings.TrimSpace(string(raw)); err == nil && pw != "" {
			return pw, nil
		}
		return "", fmt.Errorf("%s exists but holds no password; remove it and restart", path)
	}
	if err != nil {
		return "", err
	}
	pw := crypto.RandomHex(12)
	if _, err := f.WriteString(pw + "\n"); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return pw, nil
}
