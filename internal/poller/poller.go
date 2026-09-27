package poller

import (
	"context"
	"sync"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// Target is what the poller needs to know about a watched app.
type Target struct {
	ID       string
	Name     string
	URL      string
	Interval time.Duration
}

// Observation is one completed poll, handed to Observe.
type Observation struct {
	Target  Target
	Result  Result
	At      time.Time
	Latency time.Duration
}

// Getter is the slice of egress.Client the poller uses; a fake stands in for tests.
type Getter interface {
	Get(ctx context.Context, url string) (*egress.Response, error)
}

// Poller runs polls on a bounded worker pool, so one slow app never delays the others.
// Due and Observe are the store's: the poller owns no persistence.
type Poller struct {
	Get     Getter
	Workers int
	Due     func(ctx context.Context, now time.Time) ([]Target, error)
	Observe func(ctx context.Context, o Observation)

	once     sync.Once
	slots    chan struct{}
	inflight sync.Map // target ID -> struct{}
	wg       sync.WaitGroup
}

func (p *Poller) init() {
	p.once.Do(func() {
		if p.Workers <= 0 {
			p.Workers = 4
		}
		p.slots = make(chan struct{}, p.Workers)
	})
}

// Run ticks until ctx ends, then waits for the polls in flight, so the caller can close the
// store after Run returns. A tick dispatches every due target it can seat; the rest wait for
// the next tick rather than queueing, so a burst of slow apps degrades to late polls, never
// to unbounded goroutines.
func (p *Poller) Run(ctx context.Context, tick time.Duration) {
	p.init()
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.wg.Wait()
			return
		case now := <-t.C:
			p.Tick(ctx, now)
		}
	}
}

// Tick dispatches one round and returns without waiting for the polls to finish. A target
// already in flight is skipped, so a poll that outlives its interval cannot stack.
func (p *Poller) Tick(ctx context.Context, now time.Time) {
	p.init()
	due, err := p.Due(ctx, now)
	if err != nil {
		return
	}
	for _, tg := range due {
		if _, busy := p.inflight.LoadOrStore(tg.ID, struct{}{}); busy {
			continue
		}
		select {
		case p.slots <- struct{}{}:
		default:
			p.inflight.Delete(tg.ID)
			return // every worker is busy; the rest keep their place until the next tick
		}
		p.wg.Add(1)
		go p.poll(ctx, tg)
	}
}

func (p *Poller) poll(ctx context.Context, tg Target) {
	defer func() { <-p.slots; p.inflight.Delete(tg.ID); p.wg.Done() }()
	start := time.Now()
	resp, err := p.Get.Get(ctx, tg.URL)
	p.Observe(ctx, Observation{Target: tg, Result: Normalize(resp, err), At: start, Latency: time.Since(start)})
}
