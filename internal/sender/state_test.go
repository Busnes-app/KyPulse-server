package sender

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Busnes-app/ky-primitives/keyfile"
)

func TestStateSafetyAndReplacement(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{"file:a": {Kind: "file", Input: "a", Offset: 1}}}
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(dir, "token"), make([]byte, 32), keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	state.Positions["file:a"] = Position{Kind: "file", Input: "a", Offset: 2}
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	got, _, err := LoadState(dir)
	if err != nil || got.Positions["file:a"].Offset != 2 {
		t.Fatalf("state = %+v, %v", got, err)
	}
	if err := os.Chmod(filepath.Join(dir, "token"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadState(dir); !errors.Is(err, keyfile.ErrPermissive) {
		t.Fatalf("permissive token: %v", err)
	}
	if err := os.Chmod(filepath.Join(dir, "token"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "positions.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadState(dir); err == nil {
		t.Fatal("corrupt state accepted")
	}
}

func TestLockExcludesSecondOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	first, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := Lock(dir); err == nil {
		second.Close()
		t.Fatal("second owner acquired state")
	}
}

func TestStateRejectsUnsafePaths(t *testing.T) {
	for _, kind := range []string{"symlink", "permissive"} {
		t.Run(kind, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "state")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "positions.json")
			if kind == "symlink" {
				if err := os.Symlink(filepath.Join(dir, "other"), path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := SaveState(dir, State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{}}); err == nil {
				t.Fatal("unsafe state accepted")
			}
		})
	}
}
