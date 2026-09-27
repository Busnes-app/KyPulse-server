package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
)

// pairBudget bounds a claim and its sealing; the first pull is kicked off the loop, not run
// inline, so it does not need to fit in this budget.
const pairBudget = 20 * time.Second

func (s *Server) handleKyYardStatus(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.kyyard.Status(time.Now()))
}

func (s *Server) handleKyYardContainers(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.kyyard.Suggestions())
}

type pairRequest struct {
	URL         string `json:"url"`
	PairingCode string `json:"pairing_code"`
}

// handleKyYardPair claims the code and seals the token. The URL is validated by the egress
// guard (loopback, link-local and metadata addresses refused; http only by opt-in) before any
// request, and a dial-time refusal (a name that resolves to one of the same addresses) is
// reported the same way, 400; the audit row names host and organization, never the code or
// the token. Pairing while already paired replaces the sealed row here (audited
// `replaced=true`); the previous token stays valid in KyYard until an operator revokes it
// there.
//
// The response does not wait for a pull: PullNow single-flights on the service's own mutex,
// so calling it inline here could queue behind a slow loop pull (one HTTP round trip per
// endpoint) and stall past the listener's write timeout. Instead Clear invalidates any pull
// already in flight under the old pairing, Save commits the new one to disk, Adopt marks it
// paired in memory with no snapshot yet, and Kick wakes the loop to pull it without the
// request waiting. The 200 body therefore reads paired:true, stale:true, no fetched_at --
// "first pull pending" -- until that pull lands.
func (s *Server) handleKyYardPair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	base := trimURL(req.URL)
	// KyYard is a separate service kyPulse reads over the network, never itself: unlike a
	// health target (which may legitimately watch this instance), "localhost" here is refused
	// by name, ahead of the DNS lookup the egress dialer would otherwise need to catch it.
	if u, err := url.Parse(base); err == nil && strings.EqualFold(u.Hostname(), "localhost") {
		s.writeError(w, http.StatusBadRequest, "KyYard URL: "+egress.ErrRefusedAddress.Error())
		return
	}
	if err := egress.ValidateURL(base, s.config.KyYard.AllowHTTP); err != nil {
		s.writeError(w, http.StatusBadRequest, "KyYard URL: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), pairBudget)
	defer cancel()
	actor := s.actorID(r)
	cfg, err := kyyard.Claim(ctx, s.kyyard.HTTP, base, req.PairingCode)
	u, _ := url.Parse(base)
	if err != nil {
		s.audit(ctx, actor, r, "admin.kyyard_pair", "", fmt.Sprintf("outcome=failure host=%s reason=%s", auditValue(u.Host), kyyard.Reason(err)))
		switch {
		case errors.Is(err, kyyard.ErrPairingRefused):
			s.writeError(w, http.StatusForbidden, "KyYard refused the pairing code")
		case errors.Is(err, kyyard.ErrRateLimited):
			s.writeError(w, http.StatusTooManyRequests, "KyYard is rate-limiting pairing attempts; wait a minute")
		case errors.Is(err, egress.ErrRefusedAddress):
			// The pre-flight check let the URL through (a name, not a literal IP), but it
			// resolved to a loopback, link-local or reserved address at dial time.
			s.writeError(w, http.StatusBadRequest, "KyYard URL: "+err.Error())
		default:
			s.writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not reach KyYard", "reason": kyyard.Reason(err)})
		}
		return
	}
	// A pairing that already exists is replaced, not refused: the sealed row is write-once
	// only in the sense that it always holds one KyYard, and the operator who claimed a new
	// code from the KyYard side clearly means to move to it. The old token is not revoked
	// here -- that happens in KyYard -- so it is audited, not silently dropped.
	_, existed, loadErr := s.kyyard.Pairing.Load(ctx)
	replaced := existed || loadErr != nil
	s.kyyard.Clear() // invalidate any pull still in flight under the old pairing first
	if err := s.kyyard.Pairing.Save(ctx, cfg); err != nil {
		s.audit(ctx, actor, r, "admin.kyyard_pair", "", fmt.Sprintf("outcome=failure host=%s org=%s reason=save", auditValue(u.Host), auditValue(cfg.OrganizationID)))
		s.writeError(w, http.StatusInternalServerError, "KyYard already issued a token for this pairing; it could not be saved here. Revoke it in KyYard (Members -> Service tokens) before retrying")
		return
	}
	s.kyyard.Adopt(cfg)
	s.kyyard.Kick()
	s.audit(ctx, actor, r, "admin.kyyard_pair", "", fmt.Sprintf("outcome=success host=%s org=%s allow_http=%v replaced=%v", auditValue(u.Host), auditValue(cfg.OrganizationID), s.config.KyYard.AllowHTTP, replaced))
	s.writeJSON(w, http.StatusOK, s.kyyard.Status(time.Now()))
}

// handleKyYardUnpair deletes the URL and the sealed token here. Revoking the token in KyYard
// is a KyYard administrator's separate step; the UI names it.
func (s *Server) handleKyYardUnpair(w http.ResponseWriter, r *http.Request) {
	if err := s.kyyard.Pairing.Delete(r.Context()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to remove the pairing")
		return
	}
	s.kyyard.Clear()
	s.audit(r.Context(), s.actorID(r), r, "admin.kyyard_unpair", "", "")
	w.WriteHeader(http.StatusNoContent)
}

func trimURL(raw string) string {
	base := strings.TrimSpace(raw)
	for strings.HasSuffix(base, "/") {
		base = strings.TrimSuffix(base, "/")
	}
	return base
}
