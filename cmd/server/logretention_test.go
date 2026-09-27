package main

import (
	"context"
	"database/sql"
	"io"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func TestLogRetentionLoopPrunesImmediatelyAndStops(t *testing.T) {
	ctx := context.Background()
	cfg := testdb.Config(t)
	st, err := store.Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	driver := "sqlite"
	if cfg.Driver == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour)
	if err := st.Logs().Append(ctx, "", store.LogBatch{Logs: []store.LogLine{{Time: now.Add(30 * 24 * time.Hour), ReceivedAt: old, Raw: "expired"}}}, 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := st.Sources().CreateCode(ctx, "expired-code", "", old); err != nil {
		t.Fatal(err)
	}
	var codes int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM log_pairing_codes").Scan(&codes); err != nil || codes != 1 {
		t.Fatalf("before loop codes=%d err=%v", codes, err)
	}
	lg, err := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go logRetentionLoop(loopCtx, st, 1<<20, lg, done)
	deadline := time.After(3 * time.Second)
	for {
		logs, err := st.Logs().List(ctx, store.LogFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM log_pairing_codes").Scan(&codes); err != nil {
			t.Fatal(err)
		}
		if len(logs) == 0 && codes == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("startup prune did not run")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("retention loop did not stop")
	}
}
