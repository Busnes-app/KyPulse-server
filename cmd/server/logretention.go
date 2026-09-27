package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

var (
	logRetentionFailed    = logging.DeclareEvent("log_retention_failed", "log retention cleanup failed", slog.LevelError)
	logRetentionReason    = logging.DeclareString("reason")
	logRetentionAbandoned = logging.DeclareEvent("log_retention_abandoned", "log retention still running at shutdown deadline", slog.LevelError)
)

func logRetentionLoop(ctx context.Context, st store.Store, maxBytes int64, lg *logging.Logger, done chan<- struct{}) {
	defer close(done)
	cleanup := func() {
		if err := st.Logs().Prune(ctx, time.Now().UTC(), maxBytes); err != nil && ctx.Err() == nil {
			lg.Log(ctx, logRetentionFailed, logRetentionReason("prune"))
		}
		if err := st.Sources().ExpireCodes(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			lg.Log(ctx, logRetentionFailed, logRetentionReason("expire_codes"))
		}
	}
	cleanup()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
