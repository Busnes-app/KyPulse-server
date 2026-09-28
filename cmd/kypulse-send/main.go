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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/sender"
)

const shutdownGrace = 5 * time.Second

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
	if err := runWith(os.Args[1:], os.Stdin, nil, lg); err != nil {
		lg.Log(context.Background(), fatalEvent, logging.Err(err))
		os.Exit(1)
	}
}

func runWith(args []string, stdin io.ReadCloser, http sender.HTTP, logger *logging.Logger) error {
	if len(args) == 0 {
		return errors.New("usage: kypulse-send pair|file|docker|stdin")
	}
	if args[0] != "pair" && args[0] != "file" && args[0] != "stdin" && args[0] != "docker" {
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
	containers := flags.String("container", "", "comma-separated Docker containers")
	socket := flags.String("socket", "/var/run/docker.sock", "Docker Unix socket")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("sender: unexpected arguments")
	}
	if args[0] != "pair" {
		if args[0] == "docker" && *containers == "" {
			return errors.New("sender: --container is required")
		}
		if args[0] != "docker" && (*containers != "" || *socket != "/var/run/docker.sock") {
			return errors.New("sender: Docker flags are only valid for docker")
		}
		if args[0] == "docker" && *path != "" {
			return errors.New("sender: --path is only valid for file")
		}
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
		return runInput(args[0], *path, *containers, *socket, *stateDir, stdin, http, logger)
	}
	if *path != "" || *containers != "" || *socket != "/var/run/docker.sock" {
		return errors.New("sender: input flags are only valid for inputs")
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

func runInput(kind, path, containers, socket, dir string, stdin io.ReadCloser, http sender.HTTP, logger *logging.Logger) error {
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
	var ids []string
	if kind == "docker" {
		names := map[string]bool{}
		for _, name := range strings.Split(containers, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				return errors.New("sender: empty Docker container name")
			}
			names[name] = true
		}
		if len(names) > 32 {
			return errors.New("sender: too many Docker containers (max 32)")
		}
		seenNames := map[string]bool{}
		seenIDs := map[string]bool{}
		for _, name := range strings.Split(containers, ",") {
			name = strings.TrimSpace(name)
			if seenNames[name] {
				continue
			}
			id, err := sender.ResolveDocker(readerCtx, socket, name)
			if err != nil {
				return err
			}
			if !seenIDs[id] {
				ids = append(ids, id)
				seenIDs[id] = true
			}
			seenNames[name] = true
		}
	}
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
		switch kind {
		case "stdin":
			readerDone <- sender.ReadStdin(readerCtx, stdin, items)
		case "file":
			readerDone <- sender.ReadFile(readerCtx, path, state.Positions[sender.PositionKey(sender.Position{Kind: "file", Input: path})], items)
		case "docker":
			results := make(chan error, len(ids))
			for _, id := range ids {
				go func() { results <- sender.ReadDocker(readerCtx, socket, id, state.Positions, items) }()
			}
			var first error
			for range ids {
				if err := <-results; err != nil && first == nil {
					first = err
					cancelReader()
				}
			}
			readerDone <- first
		}
	}()
	s := sender.Sender{HTTP: http, StateDir: dir, Token: token, State: state, Logger: logger}
	deliveryDone := make(chan error, 1)
	go func() { deliveryDone <- s.Run(deliveryCtx, items) }()
	var readerErr error
	var drain <-chan time.Time
	var drainTimer *time.Timer
	defer func() {
		if drainTimer != nil {
			drainTimer.Stop()
		}
	}()
	// Only shutdown or a reader failure bounds delivery; clean EOF keeps retrying.
	startDrain := func() {
		if drainTimer == nil {
			drainTimer = time.NewTimer(shutdownGrace)
			drain = drainTimer.C
		}
	}
	signalDone := signalCtx.Done()
	for {
		select {
		case <-signalDone:
			signalDone = nil
			startDrain()
		case readerErr = <-readerDone:
			readerDone = nil
			if readerErr != nil {
				cancelReader()
				startDrain()
			}
		case <-drain:
			drain = nil
			cancelDelivery()
		case deliveryErr := <-deliveryDone:
			cancelReader()
			closeStdin()
			if readerDone != nil {
				joinedErr := <-readerDone
				if deliveryErr == nil {
					readerErr = joinedErr
				}
			}
			if readerErr != nil && signalCtx.Err() == nil {
				return readerErr
			}
			if signalCtx.Err() != nil && errors.Is(deliveryErr, context.Canceled) {
				return nil
			}
			return deliveryErr
		}
	}
}
