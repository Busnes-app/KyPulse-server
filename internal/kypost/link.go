package kypost

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Busnes-app/ky-primitives/suitedir"
)

// Timing of the KyIdentity lookup; variables so tests can shorten them.
var (
	Refresh  = 15 * time.Minute // a working pairing is fetched again this often
	Retry    = 30 * time.Second // first retry after a failure, doubling up to Refresh
	MinGap   = time.Minute      // a kick (refused signature) waits this long after a lookup
	lookupTO = 10 * time.Second
)

// Why there is no pairing (Status).
const (
	ReasonPending      = "pending"
	ReasonNoKyIdentity = "no_kyidentity" // KyPulse does not sign in through KyIdentity
	ReasonNoPeer       = "no_peer"
	ReasonUnauthorized = "unauthorized"
	ReasonUnreachable  = "unreachable"
	ReasonInvalidPeer  = "invalid_peer"
)

// Peer is the paired KyPost: its https origin and the pair secret, in memory only.
type Peer struct {
	URL    string
	Secret []byte
}

// Link is the KyPost pairing, refreshed from KyIdentity while the server runs.
type Link struct {
	Dir  *suitedir.Client // nil: KyPulse does not sign in through KyIdentity
	Post Poster
	Now  func() time.Time
	// Lookup replaces Dir.Peer in tests.
	Lookup func(ctx context.Context) (suitedir.Peer, error)

	peer   atomic.Pointer[Peer]
	mu     sync.Mutex
	reason string
	last   time.Time
	kick   chan struct{}
	once   sync.Once
}

func (l *Link) init() {
	l.once.Do(func() {
		l.kick = make(chan struct{}, 1)
		l.reason = ReasonPending
		if l.Dir == nil && l.Lookup == nil {
			l.reason = ReasonNoKyIdentity
		}
	})
}

// Available reports whether the channel can work at all: KyPulse signs in through KyIdentity.
func (l *Link) Available() bool { return l.Dir != nil || l.Lookup != nil }

// Status is whether KyPost is paired, and why not.
func (l *Link) Status() (paired bool, reason string) {
	l.init()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.peer.Load() != nil, l.reason
}

// Set pairs p directly; nil unpairs. Tests and Run use it.
func (l *Link) Set(p *Peer) { l.peer.Store(p) }

// Run looks KyPost up at once, every Refresh, and soon after a refused signature, until ctx
// ends. A failure is retried after Retry, doubling to Refresh. KyIdentity being unreachable
// keeps a working pairing; no_peer, refused credentials or a bad answer unpair at once.
func (l *Link) Run(ctx context.Context) {
	l.init()
	if !l.Available() {
		return
	}
	backoff := Retry
	for {
		wait := Refresh
		if !l.resolve(ctx) {
			wait, backoff = backoff, min(backoff*2, Refresh)
		} else {
			backoff = Retry
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		case <-l.kick:
			l.mu.Lock()
			gap := MinGap - time.Since(l.last)
			l.mu.Unlock()
			if gap > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(gap):
				}
			}
		}
	}
}

func (l *Link) resolve(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, lookupTO)
	defer cancel()
	lookup := l.Lookup
	if lookup == nil {
		lookup = func(ctx context.Context) (suitedir.Peer, error) { return l.Dir.Peer(ctx, "kypost") }
	}
	p, err := lookup(ctx)
	reason := ""
	switch {
	case errors.Is(err, suitedir.ErrNoPeer):
		reason = ReasonNoPeer
	case errors.Is(err, suitedir.ErrUnauthorized):
		reason = ReasonUnauthorized
	case err != nil:
		reason = ReasonUnreachable
	case !originOnly(p.URL):
		reason = ReasonInvalidPeer
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.last = time.Now()
	if reason == ReasonUnreachable && l.peer.Load() != nil {
		return false // a KyIdentity outage alone keeps a working pairing
	}
	l.reason = reason
	if reason != "" {
		l.peer.Store(nil)
		return false
	}
	l.peer.Store(&Peer{URL: p.URL, Secret: p.Secret})
	return true
}

// originOnly is an https://host[:port] URL with nothing else.
func originOnly(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && (u.Path == "" || u.Path == "/") &&
		u.RawQuery == "" && u.Fragment == ""
}

// Send delivers one alert under id (stable across retries). A refused signature asks for a
// fresh pairing soon, since the secret may have rotated, and is worth a retry.
func (l *Link) Send(ctx context.Context, id string, req SendRequest) error {
	l.init()
	p := l.peer.Load()
	if p == nil {
		return ErrNotPaired
	}
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	err := send(ctx, l.Post, trimSlash(p.URL), p.Secret, now(), id, req)
	var r Refused
	if errors.As(err, &r) && (r.Code == "bad_signature" || r.Code == "status_401") {
		select {
		case l.kick <- struct{}{}:
		default:
		}
		return errors.Join(ErrRetry, err)
	}
	return err
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
