package kyyard

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
)

const (
	PullEvery  = 60 * time.Second
	StaleAfter = 3 * time.Minute
	// historyWindow is how far back restart counts are kept, for "restarts in the last hour".
	historyWindow = time.Hour
)

var (
	evPullFailed = logging.DeclareEvent("kyyard_pull_failed", "KyYard could not be read", slog.LevelWarn)
	evPulled     = logging.DeclareEvent("kyyard_pulled", "KyYard inventory refreshed", slog.LevelDebug)
	fEndpoints   = logging.DeclareInt("endpoints")
	fContainers  = logging.DeclareInt("containers")
)

// pulledStates are the KyYard endpoint states that can hold an inventory; pending, expired
// and revoked endpoints have none.
var pulledStates = map[string]bool{"approved": true, "active": true, "offline": true}

type restartPoint struct {
	at    time.Time
	count int64
}

// Service keeps the latest KyYard snapshot in memory and refreshes it on a schedule.
type Service struct {
	Pairing *Pairing
	HTTP    HTTP
	Logger  *logging.Logger
	Now     func() time.Time

	// pullMu single-flights PullNow: the handler's first pull right after a pairing and the
	// loop's own tick must not run concurrently and race each other's commit.
	pullMu sync.Mutex

	wakeOnce sync.Once
	wake     chan struct{} // buffered 1; Kick nudges Run to pull now instead of on its ticker

	mu        sync.Mutex
	gen       uint64 // bumped by Clear; a commit from a pull started before the bump is discarded
	paired    bool
	cfg       Config
	fetchedAt *time.Time // last successful pull
	lastErr   string     // reason of the last failed pull, "" after a success
	facts     map[string]ContainerFacts
	history   map[string][]restartPoint
}

// currentGen reads the generation under the lock.
func (s *Service) currentGen() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// wakeCh lazily builds the wake channel, so the zero value of Service works.
func (s *Service) wakeCh() chan struct{} {
	s.wakeOnce.Do(func() { s.wake = make(chan struct{}, 1) })
	return s.wake
}

// Kick nudges Run to pull now instead of waiting for its ticker. Non-blocking: a wake already
// pending (Run hasn't gotten to it yet) makes this a no-op, since one extra pull covers both.
func (s *Service) Kick() {
	select {
	case s.wakeCh() <- struct{}{}:
	default:
	}
}

// Open adopts the stored pairing so Status reports it before the first pull lands.
func (s *Service) Open(ctx context.Context) error {
	cfg, ok, err := s.Pairing.Load(ctx)
	if err != nil {
		return err
	}
	if ok {
		s.Adopt(cfg)
	}
	return nil
}

// Run pulls every `every` until ctx ends, then closes done. The first pull is immediate.
// Kick wakes it early, for a pairing that wants its first real pull without blocking the
// request that just landed it: PullNow single-flights on pullMu, so a slow loop pull already
// running is not interrupted, only followed by the kicked one.
func (s *Service) Run(ctx context.Context, every time.Duration, done chan<- struct{}) {
	defer close(done)
	_ = s.PullNow(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.PullNow(ctx)
		case <-s.wakeCh():
			_ = s.PullNow(ctx)
		}
	}
}

// PullNow reads the pairing, then endpoints, inventory and samples. A failure keeps the last
// snapshot and records its reason. Unpaired clears the snapshot too, but only if nothing has
// adopted a pairing since this call started (clearIfCurrent).
//
// pullMu single-flights the whole call: the handler's first pull right after a successful
// pairing and the loop's next tick must not run concurrently, or one's commit could clobber
// the other's. The network calls still run unlocked with respect to s.mu; an unpair (Clear)
// racing an in-flight pull must not have its clear overwritten by that pull's stale commit.
// gen is captured under s.mu before the pairing is even loaded -- not after the load succeeds
// -- so an unpair landing between the load and the read is caught too, and re-checked before
// every locked write; a mismatch discards the commit silently. The same check guards the
// unpaired branch below: Load finding nothing is stale information the instant a concurrent
// Clear+Adopt (a re-pair) has landed a newer generation, and clearing unconditionally there
// would un-adopt a pairing that arrived after this Load ran.
func (s *Service) PullNow(ctx context.Context) error {
	s.pullMu.Lock()
	defer s.pullMu.Unlock()

	gen := s.currentGen()
	cfg, ok, err := s.Pairing.Load(ctx)
	if err != nil {
		s.fail(gen, "unreadable")
		s.Logger.Log(ctx, evPullFailed, logging.ReasonCode("unreadable"))
		return err
	}
	if !ok {
		s.clearIfCurrent(gen)
		return nil
	}
	c := &Client{HTTP: s.HTTP, Config: cfg}
	eps, err := c.Endpoints(ctx)
	if err != nil {
		s.fail(gen, Reason(err))
		s.Logger.Log(ctx, evPullFailed, logging.ReasonCode(Reason(err)))
		return err
	}
	now := s.now()
	facts := map[string]ContainerFacts{}
	points := map[string]restartPoint{}
	for _, ep := range eps {
		if ep.Runtime != "docker" || !pulledStates[ep.State] {
			continue
		}
		inv, err := c.Inventory(ctx, ep.ID)
		var samples []Sample
		if err == nil {
			samples, err = c.Samples(ctx, ep.ID)
		}
		if Reason(err) == "status_404" {
			continue // no inventory yet (approved, never reported); no facts, no history
		}
		if err != nil {
			s.fail(gen, Reason(err))
			s.Logger.Log(ctx, evPullFailed, logging.ReasonCode(Reason(err)))
			return err
		}
		byContainer := map[string]Sample{}
		for _, smp := range samples {
			if prev, ok := byContainer[smp.ContainerID]; !ok || smp.ObservedAt.After(prev.ObservedAt) {
				byContainer[smp.ContainerID] = smp
			}
		}
		for _, ct := range inv.Containers {
			health, exit := ParseStatus(ct.Status)
			f := ContainerFacts{Link: LinkFor(ep.ID, ct.Name), EndpointID: ep.ID, EndpointName: ep.Name, ContainerID: ct.ID, Name: ct.Name, Image: ct.Image, State: ct.State, Status: ct.Status, Health: health, ExitCode: exit, ObservedAt: inv.ObservedAt, EndpointOffline: ep.State == "offline"}
			if smp, ok := byContainer[ct.ID]; ok {
				at := smp.ObservedAt
				f.HasSample, f.SampleAt = true, &at
				f.MemoryBytes, f.MemoryLimit, f.RestartCount = smp.MemoryBytes, smp.MemoryLimit, smp.RestartCount
				points[f.Link] = restartPoint{at: now, count: smp.RestartCount}
			}
			facts[f.Link] = f
		}
	}
	s.mu.Lock()
	if s.gen != gen {
		s.mu.Unlock()
		return nil
	}
	s.mergeHistory(facts, points, now)
	s.paired, s.cfg, s.facts, s.fetchedAt, s.lastErr = true, cfg, facts, &now, ""
	s.mu.Unlock()
	s.Logger.Log(ctx, evPulled, fEndpoints(int64(len(eps))), fContainers(int64(len(facts))))
	return nil
}

// fail records a pull failure, unless gen shows an unpair landed since the pull started. The
// pairing and the last snapshot stay as they are.
func (s *Service) fail(gen uint64, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	s.lastErr = reason
}

// mergeHistory appends this pull's restart points, trims to historyWindow, drops links no
// longer in facts, and fills each fact's RestartsLastHour from the trimmed history. Caller
// holds the lock.
func (s *Service) mergeHistory(facts map[string]ContainerFacts, points map[string]restartPoint, now time.Time) {
	if s.history == nil {
		s.history = map[string][]restartPoint{}
	}
	for link, pt := range points {
		s.history[link] = append(s.history[link], pt)
	}
	for link, pts := range s.history {
		if _, ok := facts[link]; !ok {
			delete(s.history, link)
			continue
		}
		cut := 0
		for cut < len(pts) && now.Sub(pts[cut].at) > historyWindow {
			cut++
		}
		s.history[link] = pts[cut:]
	}
	for link, f := range facts {
		pts := s.history[link]
		if len(pts) == 0 {
			continue
		}
		if d := pts[len(pts)-1].count - pts[0].count; d > 0 {
			f.RestartsLastHour = d
			facts[link] = f
		}
	}
}

// Clear forgets the snapshot and the pairing; called after an unpair. Bumps gen so a pull
// already in flight cannot resurrect what it committed after this.
func (s *Service) Clear() {
	s.mu.Lock()
	s.gen++
	s.resetLocked()
	s.mu.Unlock()
}

// clearIfCurrent is Clear, but only when gen still matches the current generation. PullNow's
// unpaired branch uses it: Load reporting no pairing is stale the instant a concurrent
// Clear+Adopt (a re-pair) has already landed a newer generation, and clearing unconditionally
// there would un-adopt a pairing that arrived after that Load ran.
func (s *Service) clearIfCurrent(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	s.gen++
	s.resetLocked()
}

// resetLocked zeroes the snapshot and pairing fields; gen is the caller's responsibility.
// Caller holds s.mu.
func (s *Service) resetLocked() {
	s.paired, s.cfg, s.facts, s.history, s.fetchedAt, s.lastErr = false, Config{}, nil, nil, nil, ""
}

// Adopt marks cfg paired in memory with no snapshot yet: Status reports paired:true,
// stale:true and no fetched_at until the first pull -- normally kicked right after -- lands.
// It does not touch gen, so callers must run it last, in this order: Save cfg to disk, then
// Clear (invalidates any pull still in flight under the previous pairing), then Adopt. Save
// first so a failed save leaves the previous pairing's in-memory state untouched rather than
// reporting unpaired with a perfectly good pairing still on disk; Clear before Adopt so a
// concurrent pull under the old pairing cannot commit over what Adopt is about to set.
func (s *Service) Adopt(cfg Config) {
	s.mu.Lock()
	s.paired, s.cfg, s.facts, s.fetchedAt, s.lastErr = true, cfg, nil, nil, ""
	s.mu.Unlock()
}

// stale: no successful pull within StaleAfter, or the token was refused.
func (s *Service) stale(now time.Time) bool {
	return s.paired && (s.lastErr == "unauthorized" || s.fetchedAt == nil || now.Sub(*s.fetchedAt) > StaleAfter)
}

func (s *Service) Status(now time.Time) StatusView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.paired {
		return StatusView{}
	}
	return StatusView{Paired: true, URL: s.cfg.URL, Organization: s.cfg.OrganizationName, FetchedAt: s.fetchedAt, Stale: s.stale(now), Error: s.lastErr}
}

// Facts answers for a target's container link; false when the latest snapshot has no such
// container (removed, renamed, or never seen).
func (s *Service) Facts(link string, now time.Time) (ContainerFacts, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.facts[link]
	if !ok {
		return ContainerFacts{}, false
	}
	f.Stale = s.stale(now) || f.EndpointOffline
	if pts := s.history[link]; len(pts) > 0 {
		f.HistoryMinutes = min(60, int(now.Sub(pts[0].at)/time.Minute))
	}
	return f, true
}

func (s *Service) Suggestions() []Suggestion {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Suggestion, 0, len(s.facts))
	for _, f := range s.facts {
		out = append(out, Suggestion{Link: f.Link, EndpointName: f.EndpointName, Name: f.Name, Image: f.Image, State: f.State})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Link < out[j].Link })
	return out
}
