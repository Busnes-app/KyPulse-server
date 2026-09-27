package sender

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const dockerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const dockerTime = "2026-09-27T12:00:00.123456789Z"

func fakeDocker(t *testing.T, logs func(http.ResponseWriter, *http.Request), tty ...bool) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "docker.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/version":
			fmt.Fprint(w, `{"ApiVersion":"1.44","MinAPIVersion":"1.24"}`)
		case strings.HasSuffix(r.URL.Path, "/json"):
			fmt.Fprintf(w, `{"Id":%q,"Config":{"Tty":%t}}`, dockerID, len(tty) > 0 && tty[0])
		case strings.HasSuffix(r.URL.Path, "/logs"):
			logs(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	server.Listener = ln
	server.Start()
	t.Cleanup(server.Close)
	return socket
}

func TestDockerTTYRawAndEmptyLine(t *testing.T) {
	socket := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(dockerTime + " first\n" + dockerTime + " \n"))
	}, true)
	out := make(chan Item, 2)
	if err := ReadDocker(context.Background(), socket, "a", nil, out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || (<-out).Record.Line != "first" || (<-out).Record.Line != "" {
		t.Fatal("TTY raw lines lost")
	}
}
func frame(stream byte, body string) []byte {
	b := make([]byte, 8+len(body))
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:8], uint32(len(body)))
	copy(b[8:], body)
	return b
}

func TestDockerReplaySameTimestampAndStreams(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	socket := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.RawQuery)
		mu.Unlock()
		for _, part := range [][]byte{frame(1, dockerTime+" same\n"), frame(2, dockerTime+" error\n"), frame(1, dockerTime+" same\n"), frame(1, dockerTime+" same\n")} {
			for _, b := range part {
				_, _ = w.Write([]byte{b})
				w.(http.Flusher).Flush()
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out := make(chan Item, 10)
	if err := ReadDocker(ctx, socket, "my-container", nil, out); err != nil {
		t.Fatal(err)
	}
	first := <-out
	second := <-out
	third := <-out
	fourth := <-out
	if first.Record.Line != "same" || first.Record.Time.IsZero() || first.Position.Ordinal != 1 || second.Position.Stream != "stderr" || third.Position.Ordinal != 2 || fourth.Position.Ordinal != 3 {
		t.Fatalf("rows: %+v %+v %+v %+v", first, second, third, fourth)
	}
	resumed := make(chan Item, 10)
	start := map[string]Position{PositionKey(first.Position): first.Position, PositionKey(second.Position): second.Position}
	if err := ReadDocker(ctx, socket, "my-container", start, resumed); err != nil {
		t.Fatal(err)
	}
	if len(resumed) != 2 || (<-resumed).Position.Ordinal != 2 || (<-resumed).Position.Ordinal != 3 {
		t.Fatalf("resume: %d", len(resumed))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 2 || !strings.Contains(queries[0], "since=0") {
		t.Fatalf("queries: %v", queries)
	}
	if !strings.Contains(queries[1], fmt.Sprintf("since=%d", first.Record.Time.Unix())) || !strings.Contains(queries[1], "follow=1") || !strings.Contains(queries[1], "timestamps=1") {
		t.Fatalf("queries: %v", queries)
	}
}

func TestDockerOversizeAndMalformed(t *testing.T) {
	socket := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(frame(1, ""))
		big := make([]byte, 8)
		big[0] = 1
		binary.BigEndian.PutUint32(big[4:], maxDockerFrame+1)
		_, _ = w.Write(big)
		chunk := strings.Repeat("x", 4096)
		for remaining := maxDockerFrame + 1; remaining > 0; {
			n := remaining
			if n > len(chunk) {
				n = len(chunk)
			}
			_, _ = w.Write([]byte(chunk[:n]))
			remaining -= n
		}
		_, _ = w.Write(frame(2, dockerTime+" after\n"))
	})
	out := make(chan Item, 4)
	if err := ReadDocker(context.Background(), socket, "a", nil, out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || !strings.Contains((<-out).Record.Line, "gap") || (<-out).Record.Line != "after" {
		t.Fatal("oversize drain failed")
	}
	bad := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte{3, 0, 0, 0, 0, 0, 0, 0}) })
	if err := ReadDocker(context.Background(), bad, "a", nil, make(chan Item, 1)); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed: %v", err)
	}
}

func TestDockerRefusalAndCancellation(t *testing.T) {
	refused := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })
	if err := ReadDocker(context.Background(), refused, "a", nil, make(chan Item)); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("refusal: %v", err)
	}
	started := make(chan struct{})
	blocked := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ReadDocker(ctx, blocked, "a", nil, make(chan Item)) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("logs did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancellation ignored")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reader hung")
	}
}

func TestDockerReconnectUsesEmittedPosition(t *testing.T) {
	var mu sync.Mutex
	var calls int
	socket := fakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		_, _ = w.Write(frame(1, dockerTime+" same\n"+dockerTime+" same\n"))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out := make(chan Item, 8)
	done := make(chan error, 1)
	go func() { done <- FollowDocker(ctx, socket, "a", nil, out) }()
	for i := 0; i < 2; i++ {
		select {
		case <-out:
		case <-ctx.Done():
			t.Fatal("initial lines missing")
		}
	}
	select {
	case row := <-out:
		t.Fatalf("reconnected reader duplicated line: %+v", row)
	case <-time.After(1300 * time.Millisecond):
	}
	mu.Lock()
	seen := calls
	mu.Unlock()
	if seen < 2 {
		t.Fatalf("no reconnect: %d requests", seen)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconnect reader ignored cancellation")
	}
}
