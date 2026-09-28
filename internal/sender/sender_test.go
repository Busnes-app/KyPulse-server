package sender

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/keyfile"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/ingest"
)

type postFunc func([]byte) (*egress.Response, error)

func (f postFunc) Post(_ context.Context, _, _ string, body []byte, _ map[string]string) (*egress.Response, error) {
	return f(body)
}

func prepareSender(t *testing.T, s *Sender) {
	t.Helper()
	if err := SaveState(s.StateDir, s.State); err != nil {
		t.Fatal(err)
	}
	if err := keyfile.Store(filepath.Join(s.StateDir, "token"), s.Token, keyfile.Hex); err != nil {
		t.Fatal(err)
	}
}

func TestEncodedBatchCapAndCheckpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	line := strings.Repeat("\x00", ingest.MaxLineBytes)
	encoded, _ := json.Marshal(ingest.Record{Line: line})
	if len(encoded) <= ingest.MaxLineBytes {
		t.Fatal("fixture must expand")
	}
	var calls, received int
	s := Sender{HTTP: postFunc(func(body []byte) (*egress.Response, error) {
		calls++
		if len(body) > ingest.MaxRequestBytes {
			t.Fatalf("oversized batch: %d", len(body))
		}
		received += strings.Count(string(body), "\n")
		return &egress.Response{StatusCode: 204}, nil
	}), StateDir: dir, Token: make([]byte, 32), State: State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{}}}
	prepareSender(t, &s)
	items := make(chan Item, 500)
	for i := 1; i <= 500; i++ {
		items <- Item{Record: ingest.Record{Line: line}, Position: Position{Kind: "file", Input: "a", Offset: int64(i)}}
	}
	close(items)
	if err := s.Run(context.Background(), items); err != nil {
		t.Fatal(err)
	}
	got, _, err := LoadState(dir)
	if err != nil || calls < 2 || received != 500 || got.Positions["file:a"].Offset != 500 {
		t.Fatalf("calls=%d received=%d state=%+v err=%v", calls, received, got, err)
	}
}

func TestRetrySameFrozenBatchAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := filepath.Join(t.TempDir(), "state")
	var first []byte
	calls := 0
	s := Sender{HTTP: postFunc(func(body []byte) (*egress.Response, error) {
		calls++
		if calls == 1 {
			first = append([]byte(nil), body...)
			return nil, errors.New("dropped response")
		}
		if string(body) != string(first) {
			t.Fatal("retry changed frozen batch")
		}
		cancel()
		return &egress.Response{StatusCode: 204}, nil
	}), StateDir: dir, Token: make([]byte, 32), State: State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{}}, wait: func(context.Context, time.Duration) error { return nil }}
	prepareSender(t, &s)
	items := make(chan Item, 1)
	items <- Item{Record: ingest.Record{Line: "a"}, Position: Position{Kind: "stdin", Input: "stdin", Ordinal: 1}}
	close(items)
	if err := s.Run(ctx, items); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected Run error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRetryAfterBounds(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{{"600", 5 * time.Minute}, {now.Add(4 * time.Second).Format(http.TimeFormat), 4 * time.Second}, {"bad", 0}} {
		if got := retryAfter(tc.value, now); got != tc.want {
			t.Fatalf("%q: %s", tc.value, got)
		}
	}
}

func TestFlushAfterTwoSeconds(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	tick := make(chan time.Time, 1)
	called := make(chan struct{}, 1)
	s := Sender{HTTP: postFunc(func([]byte) (*egress.Response, error) {
		called <- struct{}{}
		return &egress.Response{StatusCode: 204}, nil
	}), StateDir: dir, Token: make([]byte, 32), State: State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{}}, newTimer: func(d time.Duration) <-chan time.Time {
		if d != 2*time.Second {
			t.Errorf("flush delay %s", d)
		}
		return tick
	}}
	prepareSender(t, &s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan Item)
	finished := make(chan error, 1)
	go func() { finished <- s.Run(ctx, input) }()
	input <- Item{Record: ingest.Record{Line: "line"}, Position: Position{Kind: "stdin", Input: "stdin", Ordinal: 1}}
	tick <- time.Now()
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("timer did not flush")
	}
	close(input)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestFiveHundredLinesFlushWithoutTimer(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	called := make(chan struct{}, 1)
	s := Sender{HTTP: postFunc(func(body []byte) (*egress.Response, error) {
		if strings.Count(string(body), "\n") != 500 {
			t.Errorf("batch lines=%d", strings.Count(string(body), "\n"))
		}
		called <- struct{}{}
		return &egress.Response{StatusCode: 204}, nil
	}), StateDir: dir, Token: make([]byte, 32), State: State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{}}, newTimer: func(time.Duration) <-chan time.Time { return make(chan time.Time) }}
	prepareSender(t, &s)
	input := make(chan Item)
	finished := make(chan error, 1)
	go func() { finished <- s.Run(context.Background(), input) }()
	for i := 1; i <= 500; i++ {
		input <- Item{Record: ingest.Record{Line: "x"}, Position: Position{Kind: "stdin", Input: "stdin", Ordinal: i}}
	}
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("500 lines did not flush")
	}
	close(input)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestOverflowMarkerAndAcknowledgedDrop(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	block := make(chan struct{})
	blockMarker := make(chan struct{})
	first := make(chan struct{})
	markerStarted := make(chan struct{})
	var mu sync.Mutex
	var markers int
	calls := 0
	s := Sender{HTTP: postFunc(func(body []byte) (*egress.Response, error) {
		mu.Lock()
		calls++
		current := calls
		if strings.HasPrefix(string(body), `{"line":"dropped `) {
			markers++
		}
		mu.Unlock()
		if current == 1 {
			close(first)
			<-block
		} else if current == 2 {
			close(markerStarted)
			<-blockMarker
		}
		return &egress.Response{StatusCode: 204}, nil
	}), StateDir: dir, Token: make([]byte, 32), State: State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{}}}
	prepareSender(t, &s)
	input := make(chan Item)
	finished := make(chan error, 1)
	go func() { finished <- s.Run(context.Background(), input) }()
	sent := make(chan struct{})
	go func() {
		for i := 1; i <= 1800; i++ {
			input <- Item{Record: ingest.Record{Line: strings.Repeat("x", ingest.MaxLineBytes)}, Position: Position{Kind: "file", Input: "a", Offset: int64(i)}}
		}
		close(sent)
	}()
	<-first
	<-sent
	close(block)
	<-markerStarted
	for i := 1801; i <= 3600; i++ {
		input <- Item{Record: ingest.Record{Line: strings.Repeat("x", ingest.MaxLineBytes)}, Position: Position{Kind: "file", Input: "a", Offset: int64(i)}}
	}
	close(input)
	close(blockMarker)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	seen := markers
	mu.Unlock()
	if seen < 2 {
		t.Fatalf("later overflow was lost, markers=%d", seen)
	}
	if s.State.Positions["file:a"].Offset != 3600 {
		t.Fatalf("position=%+v", s.State.Positions)
	}
}

func TestFailedCheckpointReplays(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{"file:a": {Kind: "file", Input: "a", Offset: 1}}}
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := Sender{HTTP: postFunc(func([]byte) (*egress.Response, error) { return &egress.Response{StatusCode: 204}, nil }), StateDir: dir, Token: make([]byte, 32), State: state, saveState: func(d string, st State) error {
		calls++
		if calls == 1 {
			return errors.New("disk full")
		}
		return SaveState(d, st)
	}}
	if err := keyfile.Store(filepath.Join(dir, "token"), s.Token, keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	input := make(chan Item, 1)
	input <- Item{Record: ingest.Record{Line: "second"}, Position: Position{Kind: "file", Input: "a", Offset: 2}}
	close(input)
	if err := s.Run(context.Background(), input); err == nil {
		t.Fatal("checkpoint failure ignored")
	}
	got, err := readState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Positions["file:a"].Offset != 1 {
		t.Fatalf("unsent checkpoint: %+v", got.Positions)
	}
}

func TestRunRefusesStaleCallerState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state := State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{"file:a": {Kind: "file", Input: "a", Offset: 10}}}
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	token := make([]byte, 32)
	if err := keyfile.Store(filepath.Join(dir, "token"), token, keyfile.Hex); err != nil {
		t.Fatal(err)
	}
	stale := State{URL: state.URL, SourceID: state.SourceID, Positions: map[string]Position{"file:a": {Kind: "file", Input: "a", Offset: 11}}}
	s := Sender{HTTP: postFunc(func([]byte) (*egress.Response, error) { t.Fatal("posted with stale state"); return nil, nil }), StateDir: dir, Token: token, State: stale}
	input := make(chan Item)
	close(input)
	if err := s.Run(context.Background(), input); err == nil {
		t.Fatal("accepted stale state")
	}
	got, err := readState(dir)
	if err != nil || got.Positions["file:a"].Offset != 10 {
		t.Fatalf("state=%+v err=%v", got, err)
	}
}

func TestHTTPRetryAndPermanentRefusal(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		first int
		retry bool
		after string
		want  time.Duration
	}{{"rate_seconds", 429, true, "600", 5 * time.Minute}, {"rate_date", 429, true, now.Add(30 * time.Second).Format(http.TimeFormat), 30 * time.Second}, {"server", 503, true, "", 0}, {"bad_request", 400, false, "", 0}, {"auth", 401, false, "", 0}, {"forbidden", 403, false, "", 0}, {"oversize", 413, false, "", 0}, {"media", 415, false, "", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var waited time.Duration
			s := Sender{HTTP: postFunc(func([]byte) (*egress.Response, error) {
				calls++
				if calls == 1 {
					return &egress.Response{StatusCode: tc.first, Header: http.Header{"Retry-After": []string{tc.after}}}, nil
				}
				return &egress.Response{StatusCode: 204}, nil
			}), Token: make([]byte, 32), State: State{URL: "https://example.com", SourceID: "one"}, now: func() time.Time { return now }, wait: func(_ context.Context, d time.Duration) error { waited = d; return nil }}
			err := s.deliver(context.Background(), []byte("{}\n"))
			if tc.retry {
				if err != nil || calls != 2 || waited == 0 {
					t.Fatalf("calls=%d waited=%s err=%v", calls, waited, err)
				}
				if tc.want == 5*time.Minute && waited != tc.want {
					t.Fatalf("wait=%s", waited)
				}
				if tc.want == 30*time.Second && waited != tc.want {
					t.Fatalf("wait=%s", waited)
				}
			} else if err == nil || calls != 1 || !strings.Contains(err.Error(), "fix sender configuration") {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestCancellationStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	s := Sender{HTTP: postFunc(func([]byte) (*egress.Response, error) { calls++; return &egress.Response{StatusCode: 503}, nil }), Token: make([]byte, 32), State: State{URL: "https://example.com"}, wait: func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() }}
	if err := s.deliver(ctx, []byte("{}\n")); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestDockerStreamsShareQueueButCheckpointIndependently(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	calls := 0
	failed := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	s := Sender{HTTP: postFunc(func(body []byte) (*egress.Response, error) {
		calls++
		if calls == 2 {
			return &egress.Response{StatusCode: 503}, nil
		}
		return &egress.Response{StatusCode: 204}, nil
	}), StateDir: dir, Token: make([]byte, 32), State: State{URL: "https://example.com", SourceID: "one", Positions: map[string]Position{}}, wait: func(ctx context.Context, _ time.Duration) error {
		close(failed)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	prepareSender(t, &s)
	items := make(chan Item, 501)
	quiet := Position{Kind: "docker", Input: "quiet", Stream: "stderr", Timestamp: "2026-09-27T12:00:00Z", Ordinal: 1}
	items <- Item{Record: ingest.Record{Line: "quiet"}, Position: quiet}
	for i := 1; i <= 500; i++ {
		items <- Item{Record: ingest.Record{Line: "busy"}, Position: Position{Kind: "docker", Input: "busy", Stream: "stdout", Timestamp: "2026-09-27T12:00:00Z", Ordinal: i}}
	}
	close(items)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, items) }()
	select {
	case <-failed:
	case <-time.After(3 * time.Second):
		t.Fatal("failed batch was not held")
	}
	held, _, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if held.Positions[PositionKey(quiet)] != quiet || held.Positions["docker:busy:stdout"].Ordinal != 499 || len(held.Positions) != 2 {
		t.Fatalf("unacknowledged busy position advanced: %+v", held.Positions)
	}
	release <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	saved, _, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if calls < 3 || saved.Positions[PositionKey(quiet)] != quiet || saved.Positions["docker:busy:stdout"].Ordinal != 500 || len(saved.Positions) != 2 {
		t.Fatalf("calls=%d positions=%+v", calls, saved.Positions)
	}
}
