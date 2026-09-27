package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSourceClaimSingleUseAndAttribution(t *testing.T) {
	ctx := context.Background()
	st := newLogTestStore(t)
	now := time.Now().UTC()
	if err := st.Targets().CreateTarget(ctx, &Target{ID: "tgt_source", Name: "Source Target", URL: "https://example.com/", IntervalSec: 30}); err != nil {
		t.Fatal(err)
	}
	if err := st.Sources().CreateCode(ctx, "code-hash", "tgt_source", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := st.Sources().Claim(ctx, "code-hash", fmt.Sprintf("hash-%d", i), "host")
			if err == nil {
				winners.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("claims won = %d", winners.Load())
	}
	sources, err := st.Sources().List(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatalf("sources=%v err=%v", sources, err)
	}
	if sources[0].TargetID != "tgt_source" {
		t.Fatalf("target=%q", sources[0].TargetID)
	}
	var tokenHash string
	if err := st.db.QueryRowContext(ctx, st.rebind("SELECT token_hash FROM log_sources WHERE id=?"), sources[0].ID).Scan(&tokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Sources().Authenticate(ctx, tokenHash); err != nil {
		t.Fatalf("active auth=%v", err)
	}
	line := testLine(now, "hello")
	line.Source = "forged"
	line.TargetID = "forged"
	if err := st.Logs().Append(ctx, sources[0].ID, LogBatch{Logs: []LogLine{line}}, 100000); err != nil {
		t.Fatal(err)
	}
	logs, err := st.Logs().List(ctx, LogFilter{})
	if err != nil || len(logs) != 1 || logs[0].Source != "host" || logs[0].TargetID != "tgt_source" {
		t.Fatalf("logs=%v err=%v", logs, err)
	}
	if err := st.Sources().Revoke(ctx, sources[0].ID, now); err != nil {
		t.Fatal(err)
	}
	if err := st.Logs().Append(ctx, sources[0].ID, LogBatch{Logs: []LogLine{line}}, 100000); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked append=%v", err)
	}
	if _, err := st.Sources().Authenticate(ctx, tokenHash); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked auth=%v", err)
	}
}

func TestSourceCodeExpiryAndTarget(t *testing.T) {
	ctx := context.Background()
	st := newLogTestStore(t)
	now := time.Now().UTC()
	if err := st.Sources().CreateCode(ctx, "missing-target", "absent", now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("target=%v", err)
	}
	if err := st.Sources().CreateCode(ctx, "expired", "", now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Sources().Claim(ctx, "expired", "token", "host"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deadline=%v", err)
	}
	if err := st.Sources().CreateCode(ctx, "duplicate-name-1", "", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.Sources().CreateCode(ctx, "duplicate-name-2", "", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for i, code := range []string{"duplicate-name-1", "duplicate-name-2"} {
		if _, err := st.Sources().Claim(ctx, code, fmt.Sprintf("token-%d", i), "host"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Sources().Authenticate(ctx, "token-0"); err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if err := st.Sources().CreateCode(ctx, "rollback-code", "", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Sources().Claim(ctx, "rollback-code", "token-0", "host"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate token=%v", err)
	}
	if _, err := st.Sources().Claim(ctx, "rollback-code", "token-2", "host"); err != nil {
		t.Fatalf("code consumed after failed insert: %v", err)
	}
}

func TestSourceClaimExpiresWhileWaitingForUsageLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st := newLogTestStore(t)
	expires := time.Now().UTC().Add(200 * time.Millisecond)
	if err := st.Sources().CreateCode(ctx, "blocked-code", "", expires); err != nil {
		t.Fatal(err)
	}
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.logs.lock(ctx, tx); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := st.Sources().Claim(ctx, "blocked-code", "blocked-token", "host")
		done <- err
	}()
	<-started
	time.Sleep(time.Until(expires) + 50*time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("claim finished before lock release: %v", err)
	default:
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim after expiry=%v", err)
	}
}
