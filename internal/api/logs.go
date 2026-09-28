package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Busnes-app/kypulse-server/internal/ingest"
	"github.com/Busnes-app/kypulse-server/internal/logstore"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func adminLogAuditAction(r *http.Request) string {
	if action := sourceAuditAction(r); action != "" {
		return action
	}
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/api/logs":
			return "admin.log_list"
		case "/api/activity":
			return "admin.activity_list"
		}
	}
	return ""
}

func (s *Server) allowIngest(sourceID string) bool {
	now := s.ingestNow()
	s.ingestMu.Lock()
	defer s.ingestMu.Unlock()
	// One bounded source entry remains stable while a source is active. Login/IP eviction
	// cannot shorten this authenticated quota window.
	for id, window := range s.ingestWindows {
		if !now.Before(window.reset) {
			delete(s.ingestWindows, id)
		}
	}
	w := s.ingestWindows[sourceID]
	if !now.Before(w.reset) {
		w = attemptWindow{reset: now.Add(time.Minute)}
	}
	w.count++
	s.ingestWindows[sourceID] = w
	return w.count <= 60
}

func (s *Server) handleIngestLogs(w http.ResponseWriter, r *http.Request) {
	fail := func(status int, reason, message string) {
		s.audit(r.Context(), "", r, "log.ingest", "", "outcome=refused reason="+reason)
		s.writeError(w, status, message)
	}
	authorization := r.Header.Get("Authorization")
	if len(authorization) > 135 || !strings.HasPrefix(authorization, "Bearer ") || len(authorization) <= 7 || strings.ContainsAny(authorization[7:], " \t\r\n") {
		fail(http.StatusUnauthorized, "authentication", "Invalid source token")
		return
	}
	source, err := s.store.Sources().Authenticate(r.Context(), sourceHash(authorization[7:]))
	if errors.Is(err, store.ErrNotFound) {
		fail(http.StatusUnauthorized, "authentication", "Invalid source token")
		return
	}
	if err != nil {
		fail(http.StatusInternalServerError, "store", "Failed to authenticate source")
		return
	}
	if !s.allowIngest(source.ID) {
		w.Header().Set("Retry-After", "60")
		fail(http.StatusTooManyRequests, "rate_limit", "Too many requests")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/x-ndjson" {
		fail(http.StatusUnsupportedMediaType, "content_type", "Expected NDJSON")
		return
	}
	if encoding := r.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		fail(http.StatusUnsupportedMediaType, "encoding", "Unsupported content encoding")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, ingest.MaxRequestBytes))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			fail(http.StatusRequestEntityTooLarge, "size", "Request too large")
		} else {
			fail(http.StatusBadRequest, "body", "Invalid request body")
		}
		return
	}
	rows, err := ingest.Decode(data)
	if err != nil {
		fail(http.StatusBadRequest, "batch", "Invalid log batch")
		return
	}
	now := s.ingestNow().UTC()
	batch := store.LogBatch{Logs: make([]store.LogLine, 0, len(rows))}
	for _, row := range rows {
		line, activity := logstore.Parse(row.Line, source.ID, source.Name, source.TargetID, row.Time, now, row.Truncated)
		batch.Logs = append(batch.Logs, line)
		if activity != nil {
			batch.Activity = append(batch.Activity, *activity)
		}
	}
	if err := s.store.Logs().Append(r.Context(), source.ID, batch, s.config.Logs.MaxBytes); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(http.StatusUnauthorized, "revoked", "Invalid source token")
		} else {
			fail(http.StatusInternalServerError, "store", "Failed to store logs")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type logQuery struct {
	targetID, app, level, text, actor, outcome string
	from, to                                   time.Time
	before                                     int64
	limit                                      int
}

func parseLogQuery(r *http.Request, activity bool) (logQuery, error) {
	var q logQuery
	allowed := map[string]bool{"target_id": true, "app": true, "from": true, "to": true, "before_id": true, "limit": true}
	if activity {
		allowed["actor"] = true
		allowed["outcome"] = true
	} else {
		allowed["level"] = true
		allowed["text"] = true
	}
	v, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, errors.New("invalid query encoding")
	}
	for key, values := range v {
		if !allowed[key] || len(values) != 1 || !utf8.ValidString(values[0]) || strings.ContainsRune(values[0], 0) {
			return q, errors.New("invalid filter")
		}
	}
	q.targetID, q.app, q.level, q.text, q.actor, q.outcome = v.Get("target_id"), v.Get("app"), v.Get("level"), v.Get("text"), v.Get("actor"), v.Get("outcome")
	for _, value := range []string{q.app, q.text, q.actor} {
		if len(value) > 256 {
			return q, errors.New("filter too long")
		}
	}
	q.limit = 100
	if raw := v.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 200 {
			return q, errors.New("invalid limit")
		}
		q.limit = n
	} else if _, ok := v["limit"]; ok {
		return q, errors.New("invalid limit")
	}
	if raw := v.Get("before_id"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return q, errors.New("invalid before_id")
		}
		q.before = n
	} else if _, ok := v["before_id"]; ok {
		return q, errors.New("invalid before_id")
	}
	for _, item := range []struct {
		key string
		dst *time.Time
	}{{"from", &q.from}, {"to", &q.to}} {
		if raw, ok := v[item.key]; ok {
			parsed, err := time.Parse(time.RFC3339Nano, raw[0])
			if err != nil {
				return q, errors.New("invalid time")
			}
			*item.dst = parsed
		}
	}
	if !q.from.IsZero() && !q.to.IsZero() && q.from.After(q.to) {
		return q, errors.New("inverted time range")
	}
	return q, nil
}

func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	q, err := parseLogQuery(r, false)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid filter")
		return
	}
	f := store.LogFilter{TargetID: q.targetID, App: q.app, Level: q.level, Text: q.text, From: q.from, To: q.to, BeforeID: q.before, Limit: q.limit}
	items, err := s.store.Logs().List(r.Context(), f)
	if err != nil {
		s.writeError(w, 500, "Failed to list logs")
		return
	}
	var next int64
	if len(items) == q.limit {
		f.BeforeID = items[len(items)-1].ID
		f.Limit = 1
		more, e := s.store.Logs().List(r.Context(), f)
		if e != nil {
			s.writeError(w, 500, "Failed to list logs")
			return
		}
		if len(more) > 0 {
			next = f.BeforeID
		}
	}
	for i := range items {
		x := &items[i]
		x.SourceID = logstore.Display(x.SourceID)
		x.Source = logstore.Display(x.Source)
		x.TargetID = logstore.Display(x.TargetID)
		x.App = logstore.Display(x.App)
		x.Level = logstore.Display(x.Level)
		x.Event = logstore.Display(x.Event)
		x.Message = logstore.Display(x.Message)
		x.Raw = logstore.Display(x.Raw)
		x.Time = x.Time.UTC()
		x.ReceivedAt = x.ReceivedAt.UTC()
	}
	s.writeJSON(w, 200, map[string]any{"items": items, "next_before_id": next})
}

func (s *Server) handleListActivity(w http.ResponseWriter, r *http.Request) {
	q, err := parseLogQuery(r, true)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid filter")
		return
	}
	f := store.ActivityFilter{TargetID: q.targetID, App: q.app, Actor: q.actor, Outcome: q.outcome, From: q.from, To: q.to, BeforeID: q.before, Limit: q.limit}
	items, err := s.store.Logs().ListActivity(r.Context(), f)
	if err != nil {
		s.writeError(w, 500, "Failed to list activity")
		return
	}
	appEvents, err := s.store.Logs().ListActivity(r.Context(), store.ActivityFilter{App: q.app, TargetID: q.targetID, Limit: 1})
	if err != nil {
		s.writeError(w, 500, "Failed to inspect retained activity")
		return
	}

	bursts, err := s.store.Logs().ActivityBursts(r.Context(), f, logstore.SignInBurstRule())
	if err != nil {
		s.writeError(w, 500, "Failed to summarize activity")
		return
	}
	for i := range bursts {
		b := &bursts[i]
		b.App = logstore.Display(b.App)
		b.Actor = logstore.Display(b.Actor)
		b.IP = logstore.Display(b.IP)
		b.From = b.From.UTC()
		b.To = b.To.UTC()
	}

	var next int64
	if len(items) == q.limit {
		f.BeforeID = items[len(items)-1].ID
		f.Limit = 1
		more, e := s.store.Logs().ListActivity(r.Context(), f)
		if e != nil {
			s.writeError(w, 500, "Failed to list activity")
			return
		}
		if len(more) > 0 {
			next = f.BeforeID
		}
	}
	for i := range items {
		x := &items[i]
		x.SourceID = logstore.Display(x.SourceID)
		x.TargetID = logstore.Display(x.TargetID)
		x.App = logstore.Display(x.App)
		x.Actor = logstore.Display(x.Actor)
		x.Action = logstore.Display(x.Action)
		x.Target = logstore.Display(x.Target)
		x.Outcome = logstore.Display(x.Outcome)
		x.IP = logstore.Display(x.IP)
		x.Time = x.Time.UTC()
		x.ReceivedAt = x.ReceivedAt.UTC()
	}
	s.writeJSON(w, 200, map[string]any{"items": items, "next_before_id": next, "bursts": bursts, "has_app_events": len(appEvents) > 0})
}
