package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/keyfile"
	"github.com/Busnes-app/ky-primitives/logging"
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
	if err := runWith([]string{"stdin", "--state-dir", dir}, failedStdin{}, &fakeHTTP{}, nil); err == nil || !strings.Contains(err.Error(), "source broke") {
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
	if err := runWith([]string{"stdin", "--state-dir", dir}, r, fake, nil); err != nil {
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
	go func() { done <- runWith([]string{"stdin", "--state-dir", dir}, r, rejectHTTP{}, nil) }()
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
	err := runWith([]string{"docker", "--state-dir", dir, "--container", strings.Join(names, ","), "--socket", "/missing/docker.sock"}, nil, &fakeHTTP{}, nil)
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
		done <- runWith([]string{"docker", "--state-dir", dir, "--socket", socket, "--container", "one,two"}, nil, fake, nil)
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

type shutdownHTTP struct {
	status   int
	calls    int
	started  chan struct{}
	canceled chan struct{}
}

func (h *shutdownHTTP) Post(ctx context.Context, _, _ string, _ []byte, _ map[string]string) (*egress.Response, error) {
	h.calls++
	if h.calls == 1 {
		close(h.started)
	}
	if h.status == 0 {
		<-ctx.Done()
		close(h.canceled)
		return nil, ctx.Err()
	}
	return &egress.Response{StatusCode: h.status, Header: http.Header{"Retry-After": []string{"300"}}}, nil
}

func cliState(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	state := sender.State{URL: "https://example.com", SourceID: "test", Positions: map[string]sender.Position{"stdin:stdin": {Kind: "stdin", Input: "stdin", Offset: 4}}}
	if err := sender.SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(dir, "token"), bytes.Repeat([]byte{0xab}, 32), keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCLISignalDuringOutage(t *testing.T) {
	for _, status := range []int{503, 429, 0} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			dir := cliState(t)
			before, err := os.ReadFile(filepath.Join(dir, "positions.json"))
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			logger, err := logging.New(logging.Config{App: "kypulse-send", Out: &logs})
			if err != nil {
				t.Fatal(err)
			}
			h := &shutdownHTTP{status: status, started: make(chan struct{}), canceled: make(chan struct{})}
			input := io.ReadCloser(io.NopCloser(strings.NewReader("unsent\n")))
			if status == 429 {
				r, w := io.Pipe()
				defer w.Close()
				input = r
				go func() { _, _ = io.WriteString(w, strings.Repeat("unsent\n", 500)) }()
			}
			done := make(chan error, 1)
			go func() {
				done <- runWith([]string{"stdin", "--state-dir", dir}, input, h, logger)
			}()
			select {
			case <-h.started:
			case <-time.After(3 * time.Second):
				t.Fatal("no POST")
			}
			started := time.Now()
			if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("signal shutdown: %v", err)
				}
			case <-time.After(7 * time.Second):
				t.Fatal("SIGTERM did not stop delivery during outage")
			}
			if status != 0 {
				found := false
				for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
					var event map[string]any
					if err := json.Unmarshal([]byte(line), &event); err != nil {
						t.Fatalf("invalid JSON event %q: %v", line, err)
					}
					if event["event"] == "sender_delivery_retry" {
						found = true
					}
				}
				if !found || strings.Contains(logs.String(), strings.Repeat("ab", 32)) || strings.Contains(logs.String(), "Bearer") {
					t.Fatalf("missing retry or leaked credentials: %s", logs.String())
				}
			}
			if elapsed := time.Since(started); elapsed < 4*time.Second {
				t.Fatalf("drain cut short: %v", elapsed)
			}
			if status == 0 {
				select {
				case <-h.canceled:
				default:
					t.Fatal("HTTP was not joined before return")
				}
			}
			after, err := os.ReadFile(filepath.Join(dir, "positions.json"))
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("unacknowledged checkpoint changed: %s %v", after, err)
			}
		})
	}
}

func TestCLIReaderErrorDuringOutage(t *testing.T) {
	dir := cliState(t)
	h := &shutdownHTTP{status: 503, started: make(chan struct{})}
	done := make(chan error, 1)
	input := io.NopCloser(io.MultiReader(strings.NewReader("unsent\n"), failedStdin{}))
	go func() { done <- runWith([]string{"stdin", "--state-dir", dir}, input, h, nil) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "source broke") {
			t.Fatalf("reader error lost: %v", err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("reader error hidden by outage")
	}
	state, _, err := sender.LoadState(dir)
	if err != nil || state.Positions["stdin:stdin"].Offset != 4 {
		t.Fatalf("checkpoint: %+v %v", state, err)
	}
}

type signalHTTP struct{ fakeHTTP }

func (h *signalHTTP) Post(ctx context.Context, url, contentType string, body []byte, headers map[string]string) (*egress.Response, error) {
	resp, err := h.fakeHTTP.Post(ctx, url, contentType, body, headers)
	if len(h.rows) == 1 {
		return resp, syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}
	return resp, err
}

func TestCLISignalFlushesFilePartial(t *testing.T) {
	dir := cliState(t)
	path := filepath.Join(t.TempDir(), "input.log")
	if err := os.WriteFile(path, []byte("first\nfinal partial"), 0600); err != nil {
		t.Fatal(err)
	}
	h := &signalHTTP{}
	done := make(chan error, 1)
	go func() { done <- runWith([]string{"file", "--state-dir", dir, "--path", path}, nil, h, nil) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("partial file did not drain promptly")
	}
	if len(h.rows) != 2 || h.rows[1].Line != "final partial" {
		t.Fatalf("rows: %+v", h.rows)
	}
	state, _, err := sender.LoadState(dir)
	if err != nil || state.Positions["file:"+path].Offset != 19 {
		t.Fatalf("checkpoint: %+v %v", state, err)
	}
}
