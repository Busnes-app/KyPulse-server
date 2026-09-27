package poller

import (
	"context"
	"sync"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// Target is what the poller needs to know about a watched app.
type Target struct {
	ID   string
	Name string
	URL  string
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
	OnError func(err error) // a failed Due; may be nil

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
// store after Run returns. Ticks that fire while a Tick is blocked on a worker are dropped.
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

// Tick seats every due target, blocking for a free worker, and returns without waiting for
// the last polls to finish. A target already in flight is skipped, so a poll that outlives
// its interval cannot stack.
func (p *Poller) Tick(ctx context.Context, now time.Time) {
	p.init()
	due, err := p.Due(ctx, now)
	if err != nil {
		if p.OnError != nil {
			p.OnError(err)
		}
		return
	}
	for _, tg := range due {
		if _, busy := p.inflight.LoadOrStore(tg.ID, struct{}{}); busy {
			continue
		}
		select {
		case p.slots <- struct{}{}:
		case <-ctx.Done():
			p.inflight.Delete(tg.ID)
			return
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
