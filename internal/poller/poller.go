package poller

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// Target is what the poller needs to know about a watched app.
type Target struct {
	ID   string
	Name string
	URL  string // http(s)://… health URL, or tls://host:port for a TLS-only target
	// SNI and AllowLocal apply to a TLS-only target: the server name to send (the host when
	// empty) and the administrator's permission to dial loopback or link-local.
	SNI        string
	AllowLocal bool
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
	Handshake(ctx context.Context, hostport, serverName string, allowLocal bool) (*tls.ConnectionState, error)
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
	var res Result
	if hostport, ok := strings.CutPrefix(tg.URL, TLSScheme); ok {
		res = p.handshake(ctx, hostport, tg)
	} else {
		resp, err := p.Get.Get(ctx, tg.URL)
		res = Normalize(resp, err)
		if u, perr := url.Parse(tg.URL); perr == nil && u.Scheme == "https" {
			var state *tls.ConnectionState
			if resp != nil {
				state = resp.TLS
			}
			res.Cert = CertOf(state, err, u.Hostname())
		}
	}
	p.Observe(ctx, Observation{Target: tg, Result: res, At: start, Latency: time.Since(start)})
}

// TLSScheme prefixes a TLS-only target's host:port.
const TLSScheme = "tls://"

// handshake probes a TLS-only target. Its health is reachability: ok once the server
// presented a certificate, whatever is wrong with it; the certificate track judges that.
func (p *Poller) handshake(ctx context.Context, hostport string, tg Target) Result {
	state, err := p.Get.Handshake(ctx, hostport, tg.SNI, tg.AllowLocal)
	name := tg.SNI
	if name == "" {
		name, _, _ = net.SplitHostPort(hostport)
	}
	if c := CertOf(state, err, name); c != nil {
		return Result{State: OK, Basic: true, Cert: c}
	}
	if err == nil {
		err = errors.New("tls: no certificate")
	}
	return Result{State: Down, Cause: egress.Cause(err)}
}
