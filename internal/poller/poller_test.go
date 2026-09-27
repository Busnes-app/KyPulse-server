package poller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

type fakeGetter struct {
	mu        sync.Mutex
	calls     map[string]int
	release   chan struct{} // when non-nil, Get blocks until closed
	ignoreCtx bool          // a Get that does not honour cancellation, like a stuck dial
}

func (g *fakeGetter) Get(ctx context.Context, url string) (*egress.Response, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = map[string]int{}
	}
	g.calls[url]++
	g.mu.Unlock()
	if g.release != nil {
		if g.ignoreCtx {
			<-g.release
		} else {
			select {
			case <-g.release:
			case <-ctx.Done():
			}
		}
	}
	if url == "http://bad.lan/healthz" {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &egress.Response{StatusCode: 200, Body: []byte(`{"status":"ok"}`)}, nil
}

func (g *fakeGetter) count(url string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[url]
}

func TestTickPollsEveryDueTargetAndObserves(t *testing.T) {
	g := &fakeGetter{}
	obs := make(chan Observation, 4)
	p := &Poller{Get: g, Workers: 2,
		Due: func(context.Context, time.Time) ([]Target, error) {
			return []Target{{ID: "a", URL: "http://good.lan/healthz"}, {ID: "b", URL: "http://bad.lan/healthz"}}, nil
		},
		Observe: func(_ context.Context, o Observation) { obs <- o },
	}
	p.Tick(context.Background(), time.Now())
	got := map[string]Result{}
	for range 2 {
		select {
		case o := <-obs:
			got[o.Target.ID] = o.Result
		case <-time.After(2 * time.Second):
			t.Fatal("observations did not arrive")
		}
	}
	if got["a"].State != OK || got["b"].State != Down || got["b"].Cause != "refused" {
		t.Fatalf("got %+v", got)
	}
}

func TestInFlightTargetIsNotPolledAgain(t *testing.T) {
	g := &fakeGetter{release: make(chan struct{})}
	p := &Poller{Get: g, Workers: 4,
		Due: func(context.Context, time.Time) ([]Target, error) {
			return []Target{{ID: "a", URL: "http://slow.lan/healthz"}}, nil
		},
		Observe: func(context.Context, Observation) {},
	}
	p.Tick(context.Background(), time.Now())
	p.Tick(context.Background(), time.Now())
	p.Tick(context.Background(), time.Now())
	time.Sleep(20 * time.Millisecond)
	if n := g.count("http://slow.lan/healthz"); n != 1 {
		t.Fatalf("slow target polled %d times while in flight, want 1", n)
	}
	close(g.release)
}

func TestFullPoolLeavesTargetsForTheNextTick(t *testing.T) {
	g := &fakeGetter{release: make(chan struct{})}
	obs := make(chan Observation, 8)
	var mu sync.Mutex
	due := []Target{{ID: "a", URL: "http://a.lan/healthz"}, {ID: "b", URL: "http://b.lan/healthz"}}
	p := &Poller{Get: g, Workers: 1,
		Due: func(context.Context, time.Time) ([]Target, error) {
			mu.Lock()
			defer mu.Unlock()
			return append([]Target(nil), due...), nil
		},
		Observe: func(_ context.Context, o Observation) { obs <- o },
	}
	p.Tick(context.Background(), time.Now())
	time.Sleep(20 * time.Millisecond)
	if g.count("http://a.lan/healthz") != 1 || g.count("http://b.lan/healthz") != 0 {
		t.Fatalf("one worker must seat exactly one target: a=%d b=%d", g.count("http://a.lan/healthz"), g.count("http://b.lan/healthz"))
	}
	close(g.release)
	<-obs
	mu.Lock()
	due = due[1:] // a has been polled; only b is due now
	mu.Unlock()
	// The slot is released just after Observe returns; tick until b is seated.
	deadline := time.Now().Add(2 * time.Second)
	for g.count("http://b.lan/healthz") == 0 && time.Now().Before(deadline) {
		p.Tick(context.Background(), time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case o := <-obs:
		if o.Target.ID != "b" {
			t.Fatalf("unexpected %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("next tick did not poll b")
	}
	if g.count("http://b.lan/healthz") != 1 || g.count("http://a.lan/healthz") != 1 {
		t.Fatalf("counts a=%d b=%d", g.count("http://a.lan/healthz"), g.count("http://b.lan/healthz"))
	}
}

func TestRunStopsWithContextAfterDrainingPolls(t *testing.T) {
	g := &fakeGetter{release: make(chan struct{}), ignoreCtx: true}
	observed := make(chan struct{}, 1)
	p := &Poller{Get: g, Due: func(context.Context, time.Time) ([]Target, error) {
		return []Target{{ID: "a", URL: "http://a.lan/healthz"}}, nil
	}, Observe: func(context.Context, Observation) { observed <- struct{}{} }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx, time.Millisecond); close(done) }()
	for g.count("http://a.lan/healthz") == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while a poll was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(g.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after the poll finished")
	}
	select {
	case <-observed:
	default:
		t.Fatal("the in-flight poll was not observed before Run returned")
	}
}
