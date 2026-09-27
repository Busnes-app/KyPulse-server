package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/keyfile"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/ingest"
	"github.com/Busnes-app/kypulse-server/internal/sender"
)

type fakeHTTP struct{ rows []ingest.Record }

func (f *fakeHTTP) Post(_ context.Context, _, _ string, body []byte, _ map[string]string) (*egress.Response, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var row ingest.Record
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, err
		}
		f.rows = append(f.rows, row)
	}
	return &egress.Response{StatusCode: 204}, nil
}

type rejectHTTP struct{}

func (rejectHTTP) Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error) {
	return &egress.Response{StatusCode: 400}, nil
}

type failedStdin struct{}

func (failedStdin) Read([]byte) (int, error) { return 0, errors.New("source broke") }
func (failedStdin) Close() error             { return nil }

func TestCLIReaderErrorSurfaces(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := sender.State{URL: "https://example.com", SourceID: "test", Positions: map[string]sender.Position{}}
	if err := sender.SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(dir, "token"), make([]byte, 32), keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	if err := runWith([]string{"stdin", "--state-dir", dir}, failedStdin{}, &fakeHTTP{}); err == nil || !strings.Contains(err.Error(), "source broke") {
		t.Fatalf("reader error: %v", err)
	}
}

func TestCLIStdinFinalPartialToIngest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := sender.State{URL: "https://example.com", SourceID: "test", Positions: map[string]sender.Position{}}
	if err := sender.SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(dir, "token"), make([]byte, 32), keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	r, w := io.Pipe()
	go func() { _, _ = io.WriteString(w, "first\nfinal partial"); _ = w.Close() }()
	fake := &fakeHTTP{}
	if err := runWith([]string{"stdin", "--state-dir", dir}, r, fake); err != nil {
		t.Fatal(err)
	}
	if len(fake.rows) != 2 || fake.rows[0].Line != "first" || fake.rows[1].Line != "final partial" {
		t.Fatalf("rows: %+v", fake.rows)
	}
	saved, _, err := sender.LoadState(dir)
	if err != nil || saved.Positions["stdin:stdin"].Offset != 19 {
		t.Fatalf("checkpoint: %+v %v", saved, err)
	}
}

func TestCLITerminalFailureJoinsStdin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := sender.State{URL: "https://example.com", SourceID: "test", Positions: map[string]sender.Position{}}
	if err := sender.SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(dir, "token"), make([]byte, 32), keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	r, w := io.Pipe()
	defer w.Close()
	done := make(chan error, 1)
	go func() { done <- runWith([]string{"stdin", "--state-dir", dir}, r, rejectHTTP{}) }()
	_, _ = io.WriteString(w, "line\n")
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "HTTP 400") {
			t.Fatalf("delivery: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("sender failed to stop reader")
	}
}
