package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// targetNameRe: starts with a letter or digit, then up to 63 more letters, digits, spaces,
// underscores, dots or hyphens.
var targetNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$`)

type targetInput struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	IntervalSec int    `json:"interval_sec"`
	Enabled     *bool  `json:"enabled"` // nil on create means true
	Container   string `json:"container"`
}

// validateTargetInput checks an admin's target write. Health targets may always be plain
// http; KYPULSE_ALERT_ALLOW_HTTP gates only the outbound webhook.
func validateTargetInput(in targetInput) error {
	if !targetNameRe.MatchString(in.Name) {
		return errors.New("name must be 1-64 characters, starting with a letter or digit")
	}
	if err := egress.ValidateURL(in.URL, true); err != nil {
		return err
	}
	if in.IntervalSec != 0 && (in.IntervalSec < 10 || in.IntervalSec > 3600) {
		return errors.New("interval_sec must be between 10 and 3600 seconds")
	}
	if len(in.Container) > 128 {
		return errors.New("container must be 128 characters or fewer")
	}
	return nil
}

// targetView renders a target the way the API answers it: the stored columns (which already
// carry silenced_until and until_fixed as the source of truth) plus basic, decoded from the
// last poll result.
func targetView(t *store.Target) map[string]any {
	b, _ := json.Marshal(t)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	var res poller.Result
	if t.LastResult != "" {
		_ = json.Unmarshal([]byte(t.LastResult), &res)
	}
	out["basic"] = res.Basic
	return out
}

// writeTargetError maps the errors a target write can produce to their HTTP status.
func (s *Server) writeTargetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrAlreadyExists):
		s.writeError(w, http.StatusConflict, "A target with that name exists")
	case errors.Is(err, store.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "Target not found")
	case errors.Is(err, egress.ErrRefusedAddress), errors.Is(err, egress.ErrScheme):
		s.writeError(w, http.StatusBadRequest, err.Error())
	default:
		s.writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.Targets().ListTargets(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list targets")
		return
	}
	views := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		views = append(views, targetView(t))
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"targets": views})
}

func (s *Server) handleGetTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.store.Targets().GetTarget(r.Context(), id)
	if err != nil {
		s.writeTargetError(w, err)
		return
	}
	var lastResult any
	if t.LastResult != "" {
		var res poller.Result
		if json.Unmarshal([]byte(t.LastResult), &res) == nil {
			lastResult = res
		}
	}
	events, _, err := s.store.Targets().ListEvents(r.Context(), id, 0, 20)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list events")
		return
	}
	var facts any
	if t.Container != "" {
		if f, ok := s.kyyard.Facts(t.Container, time.Now()); ok {
			facts = f
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"target":      targetView(t),
		"last_result": lastResult,
		"events":      events,
		"kyyard":      facts,
	})
}

func (s *Server) handleCreateTarget(w http.ResponseWriter, r *http.Request) {
	var in targetInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	if err := validateTargetInput(in); err != nil {
		s.writeTargetError(w, err)
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	t := &store.Target{
		ID:          "tgt_" + crypto.RandomHex(8),
		Name:        in.Name,
		URL:         in.URL,
		IntervalSec: in.IntervalSec,
		Enabled:     enabled,
		Container:   in.Container,
	}
	if err := s.store.Targets().CreateTarget(r.Context(), t); err != nil {
		s.writeTargetError(w, err)
		return
	}
	actor := s.actorID(r)
	s.audit(r.Context(), actor, r, "admin.target_create", t.ID,
		fmt.Sprintf("name=%s url=%s", auditValue(t.Name), auditValue(t.URL)))
	s.writeJSON(w, http.StatusCreated, map[string]any{"target": targetView(t)})
}

func (s *Server) handleUpdateTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in targetInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	existing, err := s.store.Targets().GetTarget(r.Context(), id)
	if err != nil {
		s.writeTargetError(w, err)
		return
	}
	if in.IntervalSec == 0 {
		in.IntervalSec = existing.IntervalSec // omitted: keep the stored interval
	}
	if err := validateTargetInput(in); err != nil {
		s.writeTargetError(w, err)
		return
	}
	enabled := existing.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	existing.Name, existing.URL, existing.IntervalSec, existing.Enabled, existing.Container =
		in.Name, in.URL, in.IntervalSec, enabled, in.Container
	if err := s.store.Targets().UpdateTarget(r.Context(), existing); err != nil {
		s.writeTargetError(w, err)
		return
	}
	actor := s.actorID(r)
	s.audit(r.Context(), actor, r, "admin.target_update", id,
		fmt.Sprintf("name=%s url=%s", auditValue(existing.Name), auditValue(existing.URL)))
	s.writeJSON(w, http.StatusOK, map[string]any{"target": targetView(existing)})
}

func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.Targets().DeleteTarget(r.Context(), id); err != nil {
		s.writeTargetError(w, err)
		return
	}
	actor := s.actorID(r)
	s.audit(r.Context(), actor, r, "admin.target_delete", id, "")
	w.WriteHeader(http.StatusNoContent)
}

type silenceRequest struct {
	For string `json:"for"`
}

func (s *Server) handleSilence(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req silenceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	var d time.Duration
	var untilFixed bool
	switch req.For {
	case "1h":
		d = time.Hour
	case "8h":
		d = 8 * time.Hour
	case "until_fixed":
		untilFixed = true
	case "off":
	default:
		s.writeError(w, http.StatusBadRequest, `for must be "1h", "8h", "until_fixed" or "off"`)
		return
	}
	if err := s.monitor.Silence(r.Context(), id, d, untilFixed); err != nil {
		s.writeTargetError(w, err)
		return
	}
	t, err := s.store.Targets().GetTarget(r.Context(), id)
	if err != nil {
		s.writeTargetError(w, err)
		return
	}
	actor := s.actorID(r)
	s.audit(r.Context(), actor, r, "admin.target_silence", id, "for="+auditValue(req.For))
	s.writeJSON(w, http.StatusOK, map[string]any{"silenced_until": t.SilencedUntil, "until_fixed": t.UntilFixed})
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	target := q.Get("target")
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	limit := 50
	if raw := q.Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	events, total, err := s.store.Targets().ListEvents(r.Context(), target, offset, limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list alerts")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"events": events, "total": total})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	targets, err := s.store.Targets().ListTargets(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list targets")
		return
	}
	counts := map[string]int{}
	type problem struct {
		ID    string    `json:"id"`
		Name  string    `json:"name"`
		State string    `json:"state"`
		Since time.Time `json:"since"`
		Cause string    `json:"cause,omitempty"`
	}
	var problems []problem
	var checkedAt *time.Time
	for _, t := range targets {
		if !t.Enabled {
			counts["paused"]++ // a paused target is never a problem
			continue
		}
		counts[t.State]++
		if t.State == "down" || t.State == "degraded" {
			problems = append(problems, problem{ID: t.ID, Name: t.Name, State: t.State, Since: t.StateSince, Cause: t.Cause})
		}
		if t.LastPolledAt != nil && (checkedAt == nil || t.LastPolledAt.After(*checkedAt)) {
			checkedAt = t.LastPolledAt
		}
	}
	sort.Slice(problems, func(i, j int) bool {
		if problems[i].State != problems[j].State {
			return problems[i].State == "down"
		}
		return problems[i].Name < problems[j].Name
	})

	_, configured, err := s.monitor.Webhooks.Load(r.Context())
	if err != nil {
		// A webhook row exists but cannot be opened (e.g. rotated key): still "configured",
		// since delivery is broken, not absent.
		configured = true
	}
	webhook := map[string]any{"configured": configured, "last": nil}
	if last, ok, err := s.monitor.Webhooks.Status(r.Context()); err == nil && ok {
		webhook["last"] = last
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"checked_at": checkedAt,
		"total":      len(targets),
		"ok":         counts["ok"],
		"degraded":   counts["degraded"],
		"down":       counts["down"],
		"pending":    counts["pending"],
		"paused":     counts["paused"],
		"problems":   problems,
		"webhook":    webhook,
		"kyyard":     s.kyyard.Status(time.Now()),
	})
}

func (s *Server) handleGetWebhook(w http.ResponseWriter, r *http.Request) {
	cfg, ok, err := s.monitor.Webhooks.Load(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to load the webhook")
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	out := map[string]any{"configured": true, "preset": cfg.Preset, "url": cfg.URL, "has_token": cfg.Token != "", "last": nil}
	if last, ok, err := s.monitor.Webhooks.Status(r.Context()); err == nil && ok {
		out["last"] = last
	}
	s.writeJSON(w, http.StatusOK, out)
}

type webhookRequest struct {
	Preset     string `json:"preset"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	ClearToken bool   `json:"clear_token"`
}

// sameReceiver reports whether a stored token may ride along to the new config: same preset
// and same URL scheme and host, so an edit cannot hand the token to a different receiver
// or silently downgrade it to plaintext HTTP.
func sameReceiver(old notify.Config, preset, rawURL string) bool {
	a, errA := url.Parse(old.URL)
	b, errB := url.Parse(rawURL)
	return errA == nil && errB == nil && old.Preset == notify.Preset(preset) && a.Scheme == b.Scheme && a.Host == b.Host
}

func (s *Server) handleSetWebhook(w http.ResponseWriter, r *http.Request) {
	var req webhookRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	token := req.Token
	if req.ClearToken {
		token = ""
	} else if token == "" {
		if existing, ok, err := s.monitor.Webhooks.Load(r.Context()); err == nil && ok && sameReceiver(existing, req.Preset, req.URL) {
			token = existing.Token
		}
	}
	cfg := notify.Config{Preset: notify.Preset(req.Preset), URL: req.URL, Token: token}
	if err := notify.Validate(cfg, s.config.Alerts.AllowHTTP); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.monitor.Webhooks.Save(r.Context(), cfg); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to save the webhook")
		return
	}
	// The URL itself stays out of the audit: for Discord and ntfy it is the credential.
	u, _ := url.Parse(cfg.URL)
	actor := s.actorID(r)
	s.audit(r.Context(), actor, r, "admin.webhook_set", "",
		fmt.Sprintf("preset=%s scheme=%s host=%s allow_http=%v", auditValue(req.Preset), auditValue(u.Scheme), auditValue(u.Host), s.config.Alerts.AllowHTTP))
	s.writeJSON(w, http.StatusOK, map[string]any{"configured": true, "preset": cfg.Preset, "url": cfg.URL, "has_token": cfg.Token != ""})
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	if err := s.monitor.Webhooks.Delete(r.Context()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to delete the webhook")
		return
	}
	actor := s.actorID(r)
	s.audit(r.Context(), actor, r, "admin.webhook_delete", "", "")
	w.WriteHeader(http.StatusNoContent)
}

// testSendBudget bounds a manual test send: one attempt through the egress client's own 5 s
// timeout, on a context detached from the request so a slow receiver still gets a recorded,
// reported outcome inside the listener's 15 s WriteTimeout.
const testSendBudget = 10 * time.Second

func (s *Server) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), testSendBudget)
	defer cancel()
	actor := s.actorID(r)
	err := s.monitor.SendTest(ctx)
	details := "outcome=success"
	if err != nil {
		details = "outcome=failure reason=" + notify.Reason(err)
	}
	s.audit(ctx, actor, r, "admin.webhook_test", "", details)
	switch {
	case errors.Is(err, monitor.ErrNoWebhook):
		s.writeError(w, http.StatusPreconditionFailed, "No webhook is configured")
	case err != nil:
		s.writeJSON(w, http.StatusBadGateway, map[string]string{"error": notify.Reason(err)})
	default:
		s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
