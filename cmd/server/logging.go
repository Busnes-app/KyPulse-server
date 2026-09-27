package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"

	"github.com/Busnes-app/ky-primitives/logging"
)

// processLogger is set by newLogger so fatal can reach it without a Logger passed down
// every call stack that might need to exit.
var processLogger *logging.Logger

var (
	fatalEvent = logging.DeclareEvent("fatal", "process exiting on a fatal error", slog.LevelError)
	fatalText  = logging.DeclareString("detail")
)

// newLogger builds the process logger and routes the stdlib log package and the default slog
// logger through it, so every line on stderr is one sanitised JSON record. The scaffold's
// log.Printf call sites keep working; monitoring code written from here on uses declared
// events on the returned logger.
func newLogger(out io.Writer) (*logging.Logger, error) {
	cfg, err := logging.FromEnv() // KY_LOG_LEVEL, shared across the suite
	if err != nil {
		return nil, err
	}
	cfg.App = "kypulse"
	cfg.Out = out
	lg, err := logging.New(cfg)
	if err != nil {
		return nil, err
	}
	processLogger = lg
	slog.SetDefault(slog.New(lg.Handler()))
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(lg.Handler(), slog.LevelInfo).Writer())
	return lg, nil
}

// fatalf logs one JSON line at Error level, bypassing KY_LOG_LEVEL, then returns without
// exiting -- tests call this directly. fatal is fatalf followed by os.Exit(1). log.Fatalf
// routes through the Info-gated bridge above, so with KY_LOG_LEVEL=warn a log.Fatalf before
// exit is lost entirely (the bootstrap admin password, database open failures, ...);
// every fatal exit in this package must go through fatal instead.
//
// Error is the ceiling logging.FromEnv builds from KY_LOG_LEVEL, so this line is gated only
// if an operator sets a level above Error (e.g. "ERROR+4"), which the suite does not do.
func fatalf(format string, args ...any) {
	processLogger.Log(context.Background(), fatalEvent, fatalText(fmt.Sprintf(format, args...)))
}

func fatal(format string, args ...any) {
	fatalf(format, args...)
	os.Exit(1)
}
