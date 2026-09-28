package kyyard

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/logstore"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// Replace and Unpair couple durable pairing changes with service lifecycle changes.
// Lock order is Service.mu, Pairing.mu, database. No network runs under either lock.
func (s *Service) Replace(ctx context.Context, c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep the pairing lock through adoption; Save always creates a new generation.
	s.Pairing.mu.Lock()
	defer s.Pairing.mu.Unlock()
	c.Generation = newGeneration()
	if err := s.Pairing.saveLocked(ctx, c); err != nil {
		return err
	}
	s.gen++
	s.resetLocked()
	s.paired, s.cfg = true, c
	return nil
}
func (s *Service) Unpair(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Pairing.Delete(ctx); err != nil {
		return err
	}
	s.gen++
	s.resetLocked()
	return nil
}

// RunCollection owns its timer independently of inventory and health polling.
func (s *Service) RunCollection(ctx context.Context, every time.Duration, done chan<- struct{}) {
	defer close(done)
	s.CollectNow(ctx)
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.CollectNow(ctx)
		}
	}
}

// cursorKey uses only the opaque generation and escaped stream identity, never URL/token.
func cursorKey(c Config, kind, endpoint, container string) string {
	return c.Generation + ":" + kind + ":" + url.QueryEscape(endpoint) + ":" + url.QueryEscape(container)
}

var errGenerationChanged = errors.New("kyyard: pairing changed")

func (s *Service) importBatch(ctx context.Context, gen uint64, cfg Config, batch store.LogBatch, previous, next store.LogCursor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return errGenerationChanged
	}
	s.Pairing.mu.Lock()
	defer s.Pairing.mu.Unlock()
	current, ok, err := s.Pairing.loadLocked(ctx)
	if err != nil {
		return err
	}
	if !ok || current.Generation != cfg.Generation {
		return errGenerationChanged
	}
	return s.Store.Logs().AppendImported(ctx, batch, previous, next, s.MaxLogBytes)
}

func (s *Service) collectionResult(gen uint64, audit bool, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return
	}
	dest := &s.logStatus
	if audit {
		dest = &s.auditStatus
	}
	dest.Error = reason
	if reason == "" {
		now := s.now()
		dest.LastSuccess = &now
	}
}

// CollectNow fetches each resolved linked container once, even when health is paused.
// Each target gets its own attributed copy, committed with a single shared cursor.
func (s *Service) CollectNow(ctx context.Context) {
	s.collectMu.Lock()
	defer s.collectMu.Unlock()
	gen := s.currentGen()
	cfg, ok, err := s.Pairing.Load(ctx)
	if err != nil {
		s.collectionResult(gen, false, "unreadable")
		s.collectionResult(gen, true, "unreadable")
		return
	}
	if !ok {
		return
	}
	c := &Client{HTTP: s.HTTP, LogHTTP: s.LogHTTP, Config: cfg}
	s.collectLogs(ctx, gen, c)
	s.collectAudit(ctx, gen, c)
}

type linkedContainer struct {
	facts   ContainerFacts
	targets []string
}

func (s *Service) collectLogs(ctx context.Context, gen uint64, c *Client) {
	targets, err := s.Store.Targets().ListTargets(ctx)
	if err != nil {
		s.collectionResult(gen, false, "storage")
		return
	}
	s.mu.Lock()
	groups := map[string]*linkedContainer{}
	reason := ""
	for _, target := range targets {
		f, ok := s.facts[target.Container]
		if !ok {
			if target.Container != "" {
				reason = "links_unresolved"
			}
			continue
		}
		key := cursorKey(c.Config, "logs", f.EndpointID, f.ContainerID)
		if groups[key] == nil {
			groups[key] = &linkedContainer{facts: f}
		}
		groups[key].targets = append(groups[key].targets, target.ID)
	}
	s.mu.Unlock()
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if ctx.Err() != nil {
			return
		}
		group := groups[key]
		previous, err := s.Store.Logs().Cursor(ctx, key)
		if err != nil {
			reason = "storage"
			continue
		}
		page, err := c.Logs(ctx, group.facts.EndpointID, group.facts.ContainerID, previous)
		if err != nil {
			reason = Reason(err)
			continue
		}
		now := s.now()
		next := previous
		last, _ := time.Parse(time.RFC3339Nano, previous)
		batch := store.LogBatch{}
		for _, record := range page.Lines {
			// Docker since is inclusive. Keep all equal-timestamp lines, including duplicates.
			if record.Time.After(last) {
				last = record.Time
				next = last.UTC().Format(time.RFC3339Nano)
			}
			for _, target := range group.targets {
				line, activity := logstore.Parse(record.Line, "", "kyyard", target, record.Time, now, record.Truncated)
				batch.Logs = append(batch.Logs, line)
				if activity != nil {
					batch.Activity = append(batch.Activity, *activity)
				}
			}
		}
		if page.Notice != "" {
			for _, target := range group.targets {
				// Notices are collector data, never parsed as application audit records.
				batch.Logs = append(batch.Logs, store.LogLine{Time: now, ReceivedAt: now, Source: "kyyard", TargetID: target, Level: "warn", Event: "collector_notice", Message: logstore.Bounded(page.Notice), Raw: logstore.Bounded(page.Notice)})
			}
		}
		if err = s.importBatch(ctx, gen, c.Config, batch, store.LogCursor{Key: key, Value: previous}, store.LogCursor{Key: key, Value: next}); err != nil {
			if errors.Is(err, errGenerationChanged) {
				return
			}
			reason = "storage"
		}
	}
	s.collectionResult(gen, false, reason)
}

func (s *Service) collectAudit(ctx context.Context, gen uint64, c *Client) {
	key := cursorKey(c.Config, "audit", c.Config.OrganizationID, "")
	previous, err := s.Store.Logs().Cursor(ctx, key)
	if err != nil {
		s.collectionResult(gen, true, "storage")
		return
	}
	after := int64(0)
	if previous != "" {
		after, err = strconv.ParseInt(previous, 10, 64)
		if err != nil || after < 0 {
			s.collectionResult(gen, true, "cursor_invalid")
			return
		}
	}
	for pageNo := 0; pageNo < 5; pageNo++ {
		if ctx.Err() != nil {
			return
		}
		page, err := c.AuditAfter(ctx, after)
		if err != nil {
			s.collectionResult(gen, true, Reason(err))
			return
		}
		now := s.now()
		batch := store.LogBatch{}
		for _, event := range page.Items {
			// Use the shared parser's field bounds/NUL normalization for imported audit fields.
			// Feed mapping is explicit; remote hash-chain fields are neither required nor verified.
			activity := store.Activity{Time: event.CreatedAt, ReceivedAt: now, App: "kyyard", Actor: logstore.Bounded(event.UserID), Action: logstore.Bounded(event.Action), Target: logstore.Bounded(event.Resource), Outcome: logstore.Bounded(event.Result), IP: logstore.Bounded(event.IPAddress), ExternalKey: c.Config.Generation + ":" + url.PathEscape(c.Config.OrganizationID) + ":" + strconv.FormatInt(event.ID, 10)}
			batch.Activity = append(batch.Activity, activity)
		}
		next := strconv.FormatInt(page.NextAfterID, 10)
		if err = s.importBatch(ctx, gen, c.Config, batch, store.LogCursor{Key: key, Value: previous}, store.LogCursor{Key: key, Value: next}); err != nil {
			if !errors.Is(err, errGenerationChanged) {
				s.collectionResult(gen, true, "storage")
			}
			return
		}
		// Each committed page is progress even when a subsequent request fails.
		s.collectionResult(gen, true, "")
		previous, after = next, page.NextAfterID
		if len(page.Items) < 200 {
			return
		}
	}
}
