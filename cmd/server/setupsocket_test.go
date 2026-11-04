package main

import (
	"bytes"
	"context"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
)

// A group-writable data directory (what `mkdir` makes under umask 002) turns installer
// setup off with a logged reason; it does not stop the server.
func TestSetupSocketOffForAGroupWritableDataDir(t *testing.T) {
	data := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(data, 0o775); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	prev := slog.Default()
	lg, err := newLogger(&buf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
		slog.SetDefault(prev)
	})
	apply := func(context.Context, []byte, bool) (applysetup.Report, error) { return applysetup.Report{}, nil }
	closeSetup := startSetupSocket(context.Background(), lg, data, apply, func() {})
	if err := closeSetup(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"event":"installer_setup_off"`) {
		t.Fatalf("no installer_setup_off event logged:\n%s", buf.String())
	}
	if _, err := os.Lstat(filepath.Join(data, "installer", "setup.sock")); err == nil {
		t.Fatal("setup socket opened in a group-writable data directory")
	}

	// A private data directory opens it.
	if err := os.Chmod(data, 0o700); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	closeSetup = startSetupSocket(context.Background(), lg, data, apply, func() {})
	defer closeSetup()
	if strings.Contains(buf.String(), "installer_setup_off") {
		t.Fatalf("private data directory refused:\n%s", buf.String())
	}
	if _, err := os.Lstat(filepath.Join(data, "installer", "setup.sock")); err != nil {
		t.Fatalf("setup socket not opened: %v", err)
	}
}

// Only the directory's ancestry degrades: a second server on the same data directory (the
// socket is already owned) still stops, as before.
func TestSetupSocketOwnedByAnotherProcessIsFatal(t *testing.T) {
	data := os.Getenv("KYPULSE_TEST_SECOND_SERVER")
	if data != "" {
		lg, _ := newLogger(os.Stderr)
		apply := func(context.Context, []byte, bool) (applysetup.Report, error) { return applysetup.Report{}, nil }
		startSetupSocket(context.Background(), lg, data, apply, func() {})
		os.Exit(0)
	}
	data = filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	lg, err := newLogger(&buf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.SetOutput(os.Stderr); log.SetFlags(log.LstdFlags) })
	apply := func(context.Context, []byte, bool) (applysetup.Report, error) { return applysetup.Report{}, nil }
	closeSetup := startSetupSocket(context.Background(), lg, data, apply, func() {})
	defer closeSetup()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSetupSocketOwnedByAnotherProcessIsFatal$")
	cmd.Env = append(os.Environ(), "KYPULSE_TEST_SECOND_SERVER="+data)
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "another product process owns the setup socket") {
		t.Fatalf("second server on the same data directory did not stop: %v\n%s", err, out)
	}
}
