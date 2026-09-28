package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestImportedAtomicCursorCAS(t *testing.T) {
	s := newLogTestStore(t)
	ctx := context.Background()
	logs := s.Logs()
	prior, next := LogCursor{Key: "generation:logs:ep:ct"}, LogCursor{Key: "generation:logs:ep:ct", Value: "timestamp"}
	batch := LogBatch{Logs: []LogLine{testLine(time.Now(), "imported")}}
	bad := LogBatch{Logs: []LogLine{testLine(time.Now(), "rollback")}, Activity: []Activity{{Time: time.Now(), ReceivedAt: time.Now(), TargetID: "missing-target", Action: "bad"}}}
	if err := logs.AppendImported(ctx, bad, prior, next, 1<<20); err == nil {
		t.Fatal("expected foreign key failure")
	}
	if cur, err := logs.Cursor(ctx, prior.Key); err != nil || cur != "" {
		t.Fatalf("cursor after failed insert: %q %v", cur, err)
	}
	rows, _ := logs.List(ctx, LogFilter{})
	if len(rows) != 0 || usage(t, s) != 0 {
		t.Fatal("failed insert retained rows/usage")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- logs.AppendImported(ctx, LogBatch{Logs: []LogLine{testLine(time.Now(), "imported")}}, prior, next, 1<<20)
		}()
	}
	wg.Wait()
	close(results)
	success, stale := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrCursorConflict) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("success=%d stale=%d", success, stale)
	}
	rows, _ = logs.List(ctx, LogFilter{})
	if len(rows) != 1 {
		t.Fatalf("CAS appended %d rows", len(rows))
	}
	// A fresh store facade after a lost acknowledgment sees the committed cursor.
	if cur, err := s.Logs().Cursor(ctx, prior.Key); err != nil || cur != next.Value {
		t.Fatalf("restart cursor=%q %v", cur, err)
	}
	if err := logs.AppendImported(ctx, batch, next, LogCursor{Key: next.Key, Value: "next"}, 0); err != nil {
		t.Fatal(err)
	}
	rows, _ = logs.List(ctx, LogFilter{})
	if len(rows) != 0 || usage(t, s) != 0 {
		t.Fatal("import did not atomically prune")
	}
}
