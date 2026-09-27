package poller

import (
	"context"
	"errors"
	"fmt"
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

// dueTargets returns n due targets named t0..t<n-1>.
func dueTargets(n int) func(context.Context, time.Time) ([]Target, error) {
	return func(context.Context, time.Time) ([]Target, error) {
		out := make([]Target, n)
		for i := range out {
			out[i] = Target{ID: fmt.Sprint("t", i), URL: fmt.Sprintf("http://t%d.lan/healthz", i)}
		}
		return out, nil
	}
}

func TestTickSeatsEveryDueTargetWithOneWorker(t *testing.T) {
	g := &fakeGetter{}
	obs := make(chan Observation, 2)
	p := &Poller{Get: g, Workers: 1, Due: dueTargets(2), Observe: func(_ context.Context, o Observation) { obs <- o }}
	p.Tick(context.Background(), time.Now())
	seen := map[string]bool{}
	for range 2 {
		select {
		case o := <-obs:
			seen[o.Target.ID] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("one tick seated only %v", seen)
		}
	}
}

func TestThirtyTargetsFourWorkersOneTick(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	p := &Poller{Workers: 4, Due: dueTargets(30),
		Get: getterFunc(func(context.Context, string) (*egress.Response, error) {
			time.Sleep(10 * time.Millisecond)
			return &egress.Response{StatusCode: 200}, nil
		}),
		Observe: func(_ context.Context, o Observation) { mu.Lock(); seen[o.Target.ID] = true; mu.Unlock() },
	}
	p.Tick(context.Background(), time.Now())
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(seen)
		mu.Unlock()
		if n == 30 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("observed %d of 30 targets after one tick", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTickReportsADueError(t *testing.T) {
	var got error
	p := &Poller{Get: &fakeGetter{}, Observe: func(context.Context, Observation) {},
		Due:     func(context.Context, time.Time) ([]Target, error) { return nil, errors.New("db gone") },
		OnError: func(err error) { got = err },
	}
	p.Tick(context.Background(), time.Now())
	if got == nil {
		t.Fatal("a Due error was not reported")
	}
	p.OnError = nil
	p.Tick(context.Background(), time.Now()) // nil OnError must not panic
}

type getterFunc func(context.Context, string) (*egress.Response, error)

func (f getterFunc) Get(ctx context.Context, url string) (*egress.Response, error) {
	return f(ctx, url)
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
