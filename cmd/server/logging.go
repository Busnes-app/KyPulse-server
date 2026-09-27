package main

import (
	"io"
	"log"
	"log/slog"

	"github.com/Busnes-app/ky-primitives/logging"
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
	slog.SetDefault(slog.New(lg.Handler()))
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(lg.Handler(), slog.LevelInfo).Writer())
	return lg, nil
}
