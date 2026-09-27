package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/sender"
)

var fatalEvent = logging.DeclareEvent("sender_fatal", "sender exiting after an error", slog.LevelError)

func main() {
	cfg, err := logging.FromEnv()
	if err != nil {
		lg, _ := logging.New(logging.Config{App: "kypulse-send", Out: os.Stderr})
		lg.Log(context.Background(), fatalEvent, logging.Err(err))
		os.Exit(1)
	}
	cfg.App = "kypulse-send"
	cfg.Out = os.Stderr
	lg, err := logging.New(cfg)
	if err != nil {
		os.Exit(1)
	}
	if err := run(os.Args[1:]); err != nil {
		lg.Log(context.Background(), fatalEvent, logging.Err(err))
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: kypulse-send pair|file|docker|stdin")
	}
	if args[0] != "pair" {
		return fmt.Errorf("sender: %s input is not available in this build", args[0])
	}
	dir, err := sender.DefaultStateDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("pair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.String("url", "", "kyPulse HTTPS origin")
	code := flags.String("code", "", "six-digit pairing code")
	name := flags.String("name", "", "source name")
	stateDir := flags.String("state-dir", dir, "sender state directory")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("sender: unexpected pair arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := sender.Pair(ctx, egress.New(egress.Options{Timeout: 10 * time.Second}), *stateDir, *base, *code, *name)
	if err != nil {
		return err
	}
	fmt.Printf("paired source %s\n", state.SourceID)
	return nil
}
