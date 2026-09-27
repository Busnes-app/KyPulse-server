package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
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
	return runWith(args, os.Stdin, nil)
}

func runWith(args []string, stdin io.ReadCloser, http sender.HTTP) error {
	if len(args) == 0 {
		return errors.New("usage: kypulse-send pair|file|docker|stdin")
	}
	if args[0] == "docker" {
		return fmt.Errorf("sender: %s input is not available in this build", args[0])
	}
	if args[0] != "pair" && args[0] != "file" && args[0] != "stdin" {
		return fmt.Errorf("sender: unknown command %s", args[0])
	}
	dir, err := sender.DefaultStateDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	base := flags.String("url", "", "kyPulse HTTPS origin")
	code := flags.String("code", "", "six-digit pairing code")
	name := flags.String("name", "", "source name")
	stateDir := flags.String("state-dir", dir, "sender state directory")
	path := flags.String("path", "", "file to follow")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("sender: unexpected arguments")
	}
	if args[0] != "pair" {
		if args[0] == "file" && *path == "" {
			return errors.New("sender: --path is required")
		}
		if args[0] == "stdin" && *path != "" {
			return errors.New("sender: --path is only valid for file")
		}
		if *base != "" || *code != "" || *name != "" {
			return errors.New("sender: pairing flags are only valid for pair")
		}
		if args[0] == "file" {
			*path, err = filepath.Abs(*path)
			if err != nil {
				return err
			}
		}
		return runInput(args[0], *path, *stateDir, stdin, http)
	}
	if *path != "" {
		return errors.New("sender: --path is only valid for file")
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

func runInput(kind, path, dir string, stdin io.ReadCloser, http sender.HTTP) error {
	state, token, err := sender.LoadState(dir)
	if err != nil {
		return err
	}
	if http == nil {
		http = egress.New(egress.Options{Timeout: 10 * time.Second})
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	readerCtx, cancelReader := context.WithCancel(signalCtx)
	defer cancelReader()
	deliveryCtx, cancelDelivery := context.WithCancel(context.Background())
	defer cancelDelivery()
	items := make(chan sender.Item, 128)
	readerDone := make(chan error, 1)
	var closeOnce sync.Once
	closeStdin := func() {
		if kind == "stdin" {
			closeOnce.Do(func() { _ = stdin.Close() })
		}
	}
	if kind == "stdin" {
		go func() { <-readerCtx.Done(); closeStdin() }()
	}
	go func() {
		defer close(items)
		if kind == "stdin" {
			readerDone <- sender.ReadStdin(readerCtx, stdin, items)
		} else {
			readerDone <- sender.ReadFile(readerCtx, path, state.Positions[sender.PositionKey(sender.Position{Kind: "file", Input: path})], items)
		}
	}()
	s := sender.Sender{HTTP: http, StateDir: dir, Token: token, State: state}
	deliveryErr := s.Run(deliveryCtx, items)
	if deliveryErr != nil {
		cancelReader()
		closeStdin()
		<-readerDone
		return deliveryErr
	}
	readerErr := <-readerDone
	cancelReader()
	closeStdin()
	if readerErr != nil && signalCtx.Err() == nil {
		return readerErr
	}
	return nil
}
