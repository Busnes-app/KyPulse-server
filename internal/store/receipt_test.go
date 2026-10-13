package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSequencedReceiptAtomicReplay(t *testing.T) {
	s := newLogTestStore(t)
	ctx := context.Background()
	if err := s.Sources().CreateCode(ctx, "code", "", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	source, err := s.Sources().Claim(ctx, "code", "token", "sender")
	if err != nil {
		t.Fatal(err)
	}
	r := LogReceipt{Sequence: 1, Digest: strings.Repeat("ab", 32)}
	now := time.Now().UTC()
	// Conflict after the receipt update must roll back the entire transaction.
	seed := Activity{Time: now, ReceivedAt: now, Action: "seed", ExternalKey: "conflict"}
	if err := s.Logs().Append(ctx, "", LogBatch{Activity: []Activity{seed}}, 1<<20); err != nil {
		t.Fatal(err)
	}
	before := usage(t, s)
	bad := LogBatch{Logs: []LogLine{testLine(now, "rollback")}, Activity: []Activity{seed}}
	if err := s.Logs().AppendSequenced(ctx, source.ID, bad, r, 1<<20); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("rollback: %v", err)
	}
	if usage(t, s) != before {
		t.Fatal("failed receipt changed usage")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.Logs().AppendSequenced(ctx, source.ID, LogBatch{Logs: []LogLine{testLine(now, "one")}, Activity: []Activity{{Time: now, ReceivedAt: now, Action: "accepted"}}}, r, 1<<20)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.Logs().List(ctx, LogFilter{})
	if err != nil || len(rows) != 1 || rows[0].SourceID != source.ID || rows[0].Source != "sender" {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	activity, err := s.Logs().ListActivity(ctx, ActivityFilter{})
	if err != nil || len(activity) != 2 {
		t.Fatalf("activity: %+v %v", activity, err)
	}
	before = usage(t, s)
	for _, conflict := range []LogReceipt{{1, strings.Repeat("cd", 32)}, {3, r.Digest}, {0, r.Digest}, {2, "invalid"}} {
		if err := s.Logs().AppendSequenced(ctx, source.ID, LogBatch{}, conflict, 0); !errors.Is(err, ErrLogSequence) {
			t.Fatalf("conflict %+v: %v", conflict, err)
		}
	}
	if err := s.Logs().Append(ctx, source.ID, LogBatch{}, 0); !errors.Is(err, ErrLogSequence) {
		t.Fatalf("legacy fallback: %v", err)
	}
	if usage(t, s) != before {
		t.Fatal("refusal pruned or changed usage")
	}
	// Retention can remove payloads without removing the durable acknowledgment.
	if err := s.Logs().Prune(ctx, now, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Logs().AppendSequenced(ctx, source.ID, LogBatch{Logs: []LogLine{testLine(now, "one")}}, r, 1<<20); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.Logs().List(ctx, LogFilter{}); err != nil || len(rows) != 0 {
		t.Fatalf("replayed pruned batch: %+v %v", rows, err)
	}
	if err := s.Logs().AppendSequenced(ctx, source.ID, LogBatch{}, LogReceipt{2, r.Digest}, 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := s.Logs().AppendSequenced(ctx, source.ID, LogBatch{}, r, 1<<20); !errors.Is(err, ErrLogSequence) {
		t.Fatalf("stale receipt: %v", err)
	}
	if err := s.Sources().Revoke(ctx, source.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Logs().AppendSequenced(ctx, source.ID, LogBatch{}, LogReceipt{2, r.Digest}, 1<<20); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked replay: %v", err)
	}
}
