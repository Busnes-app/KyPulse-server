package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestDockerContainerLimitBeforeSocketAccess(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := sender.State{URL: "https://example.com", SourceID: "test", Positions: map[string]sender.Position{}}
	if err := sender.SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(dir, "token"), make([]byte, 32), keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	var names []string
	for i := range 33 {
		names = append(names, fmt.Sprintf("container-%d", i))
	}
	err := runWith([]string{"docker", "--state-dir", dir, "--container", strings.Join(names, ","), "--socket", "/missing/docker.sock"}, nil, &fakeHTTP{})
	if err == nil || !strings.Contains(err.Error(), "max 32") {
		t.Fatalf("limit: %v", err)
	}
}

func TestDockerReadersCompleteIndependently(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := sender.State{URL: "https://example.com", SourceID: "test", Positions: map[string]sender.Position{}}
	if err := sender.SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(dir, "token"), make([]byte, 32), keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	id1 := fmt.Sprintf("%064x", 1)
	id2 := fmt.Sprintf("%064x", 2)
	firstDone := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var firstCalls, secondCalls atomic.Int32
	frame := func(line string) []byte {
		b := make([]byte, 8+len(line))
		b[0] = 1
		binary.BigEndian.PutUint32(b[4:8], uint32(len(line)))
		copy(b[8:], line)
		return b
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			fmt.Fprint(w, `{"ApiVersion":"1.44"}`)
		case strings.HasSuffix(r.URL.Path, "/json"):
			id := id2
			if strings.Contains(r.URL.Path, "one") || strings.Contains(r.URL.Path, id1) {
				id = id1
			}
			fmt.Fprintf(w, `{"Id":%q,"Config":{"Tty":false}}`, id)
		case strings.HasSuffix(r.URL.Path, "/logs") && strings.Contains(r.URL.Path, id1):
			firstCalls.Add(1)
			_, _ = w.Write(frame("2026-09-27T12:00:00Z first\n"))
			close(firstDone)
		case strings.HasSuffix(r.URL.Path, "/logs") && strings.Contains(r.URL.Path, id2):
			secondCalls.Add(1)
			_, _ = w.Write(frame("2026-09-27T12:00:00Z second\n"))
			w.(http.Flusher).Flush()
			close(secondStarted)
			<-release
		default:
			http.NotFound(w, r)
		}
	}))
	server.Listener = ln
	server.Start()
	defer server.Close()
	fake := &fakeHTTP{}
	done := make(chan error, 1)
	go func() {
		done <- runWith([]string{"docker", "--state-dir", dir, "--socket", socket, "--container", "one,two"}, nil, fake)
	}()
	for _, ch := range []<-chan struct{}{firstDone, secondStarted} {
		select {
		case <-ch:
		case <-time.After(3 * time.Second):
			t.Fatal("readers did not start")
		}
	}
	select {
	case err := <-done:
		t.Fatalf("one reader ended other: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("completed readers did not drain")
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 1 || len(fake.rows) != 2 {
		t.Fatalf("calls=%d/%d rows=%+v", firstCalls.Load(), secondCalls.Load(), fake.rows)
	}
}
