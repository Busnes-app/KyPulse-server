package backup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/backup"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// Q6 A: the most recent scheduled failure is kept for the Backup page; any success clears it.
func TestRecordRunResult(t *testing.T) {
	ctx := context.Background()
	_, st := sqliteInstance(t)
	s := st.Settings()
	if err := backup.RecordRunResult(ctx, s, true, "failure", "disk full"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting(ctx, backup.LastRunErrorKey); v != "disk full" {
		t.Fatalf("scheduled failure not kept: %q", v)
	}
	if err := backup.RecordRunResult(ctx, s, false, "failure", "manual"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting(ctx, backup.LastRunErrorKey); v != "disk full" {
		t.Fatalf("a manual failure replaced the scheduled one: %q", v)
	}
	if err := backup.RecordRunResult(ctx, s, false, "success", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSetting(ctx, backup.LastRunErrorKey); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("success did not clear it: %v", err)
	}
	if err := backup.RecordRunResult(ctx, s, true, "success", ""); err != nil {
		t.Fatal("clearing twice must not fail:", err)
	}
}
