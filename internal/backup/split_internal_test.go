package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDBMembersSplitAndJoin(t *testing.T) {
	defer func(v int64) { memberLimit = v }(memberLimit)
	memberLimit = 4
	raw := []byte("0123456789")
	parts := dbMembers(raw)
	if len(parts) != 3 || parts[0].Path != "data/kypulse.db.part-000" || parts[2].Path != "data/kypulse.db.part-002" {
		t.Fatalf("parts: %+v", parts)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0700)
	for _, p := range parts {
		os.WriteFile(filepath.Join(dir, p.Path), p.Data, 0600)
	}
	if joined, err := JoinDatabase(dir, true); err != nil || !joined {
		t.Fatal(joined, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, dbPath))
	left, _ := filepath.Glob(filepath.Join(dir, "data", "*.part-*"))
	if !bytes.Equal(got, raw) || len(left) != 0 {
		t.Fatalf("joined %q, parts left %v", got, left)
	}
	if joined, err := JoinDatabase(dir, true); err != nil || joined {
		t.Fatal("a single-member capsule needs no join", joined, err)
	}
	// A gap in the parts is refused rather than joined into a corrupt database.
	gap := t.TempDir()
	os.MkdirAll(filepath.Join(gap, "data"), 0700)
	os.WriteFile(filepath.Join(gap, "data/kypulse.db.part-000"), []byte("a"), 0600)
	os.WriteFile(filepath.Join(gap, "data/kypulse.db.part-002"), []byte("c"), 0600)
	if _, err := JoinDatabase(gap, true); err == nil {
		t.Fatal("non-consecutive parts joined")
	}

	// A destination that is a glob pattern ([restore]) is still a literal directory.
	odd := filepath.Join(t.TempDir(), "[restore]")
	os.MkdirAll(filepath.Join(odd, "data"), 0700)
	for _, p := range parts {
		os.WriteFile(filepath.Join(odd, p.Path), p.Data, 0600)
	}
	if joined, err := JoinDatabase(odd, true); err != nil || !joined {
		t.Fatal("parts under a bracketed path not joined:", joined, err)
	}
}
