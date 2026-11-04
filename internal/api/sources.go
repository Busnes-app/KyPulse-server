package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/Busnes-app/ky-primitives/password"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func sourceHash(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

// sourceAuditAction names only registered source methods, so shared middleware can audit
// denials before a source handler runs without auditing unrelated routes.
func sourceAuditAction(r *http.Request) string {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/log-sources/pairing":
		return "admin.log_pairing"
	case r.Method == http.MethodPost && r.URL.Path == "/api/log-sources/claim":
		return "log_source.claim"
	case r.Method == http.MethodGet && r.URL.Path == "/api/log-sources":
		return "admin.log_source_list"
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/log-sources/"):
		return "admin.log_source_revoke"
	default:
		return ""
	}
}

// oneJSON permits only one JSON object and caps source route bodies independently of the API cap.
func oneJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	defer r.Body.Close()
	if !isJSON(r) {
		return errors.New("JSON content type required")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return errors.New("JSON object required")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("extra JSON value")
	}
	return nil
}

func (s *Server) handleCreateLogPairing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetID          string `json:"target_id"`
		OffboardingAlerts bool   `json:"offboarding_alerts"`
		Password          string `json:"password"`
	}
	if err := oneJSON(w, r, &req); err != nil {
		s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", "", "outcome=refused reason=bad_json")
		s.writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if !s.pairingStepUp(w, r, req.Password) {
		return
	}
	expires := time.Now().UTC().Add(15 * time.Minute)
	for range 8 {
		n, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", "", "outcome=failure reason=random")
			s.writeError(w, http.StatusInternalServerError, "Failed to create pairing code")
			return
		}
		code := fmt.Sprintf("%06d", n.Int64())
		if req.OffboardingAlerts {
			err = s.store.Sources().CreateOffboardingCode(r.Context(), sourceHash(code), req.TargetID, expires)
		} else {
			err = s.store.Sources().CreateCode(r.Context(), sourceHash(code), req.TargetID, expires)
		}
		if errors.Is(err, store.ErrAlreadyExists) {
			continue
		}
		if errors.Is(err, store.ErrNotFound) {
			s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", "", "outcome=refused reason=unknown_target")
			s.writeError(w, http.StatusBadRequest, "Unknown target")
			return
		}
		if err != nil {
			s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", "", "outcome=failure reason=store")
			s.writeError(w, http.StatusInternalServerError, "Failed to create pairing code")
			return
		}
		details := "outcome=success"
		if req.OffboardingAlerts {
			details += " offboarding_alerts=true"
		}
		s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", req.TargetID, details)
		s.writeJSON(w, http.StatusOK, map[string]any{"code": code, "expires_at": expires})
		return
	}
	s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", "", "outcome=refused reason=collision")
	s.writeError(w, http.StatusServiceUnavailable, "Could not create pairing code")
}

// stepUpWindow is how long a KyIdentity step-up sign-in counts as proof of presence.
const stepUpWindow = 10 * time.Minute

// pairingStepUp requires proof of presence before minting a credential that outlives the
// session: a local account's current password, or a KyIdentity step-up sign-in (max_age=0)
// within stepUpWindow. It reports false after answering 403 step_up_required.
func (s *Server) pairingStepUp(w http.ResponseWriter, r *http.Request, pw string) bool {
	user, sess, err := s.sessions.AuthenticateRequest(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "Authentication required")
		return false
	}
	refuse := func(reason, method, message string) bool {
		s.auditRefusal(r.Context(), user.ID, r, "admin.log_pairing", "outcome=refused reason="+reason)
		s.writeJSON(w, http.StatusForbidden, map[string]string{"error": message, "code": "step_up_required", "method": method})
		return false
	}
	if user.SSOProvider != "local" {
		if sess.StepUpAt == nil || time.Since(*sess.StepUpAt) > stepUpWindow {
			return refuse("step_up", "kyidentity", "Confirm it is you with KyIdentity before adding a log source")
		}
		return true
	}
	// Only wrong passwords spend the window, so a stolen session cannot guess on for long.
	key := stepUpKey(s, r, user.ID)
	if !s.loginOpen(key, 5) {
		return refuse("step_up_rate_limit", "password", "Too many wrong passwords; wait a few minutes")
	}
	if ok, err := password.Verify(pw, user.PasswordHash); pw == "" || err != nil || !ok {
		s.spendLogin(key, 15*time.Minute)
		return refuse("step_up", "password", "Enter your current password to add a log source")
	}
	return true
}

// stepUpKey is the wrong-password window shared by the pairing step-up and the password
// change: five guesses across both, then 15 minutes. It carries the client address, so a
// stolen session elsewhere cannot lock the owner out of changing their password.
func stepUpKey(s *Server, r *http.Request, userID string) string {
	return "step-up:" + userID + "@" + s.limitIP(r)
}

func (s *Server) handleClaimLogSource(w http.ResponseWriter, r *http.Request) {
	if !s.allowClaim(s.limitIP(r)) {
		w.Header().Set("Retry-After", "60")
		s.auditRefusal(r.Context(), "", r, "log_source.claim", "outcome=refused reason=rate_limit")
		s.writeError(w, http.StatusTooManyRequests, "Too many attempts")
		return
	}
	var req struct {
		Code string `json:"pairing_code"`
		Name string `json:"name"`
	}
	if err := oneJSON(w, r, &req); err != nil {
		s.auditRefusal(r.Context(), "", r, "log_source.claim", "outcome=refused reason=bad_json")
		s.writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if len(req.Code) != 6 || !sixDigits(req.Code) {
		s.auditRefusal(r.Context(), "", r, "log_source.claim", "outcome=failure reason=invalid_code")
		s.writeError(w, http.StatusForbidden, "Invalid or expired pairing code")
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=failure reason=random")
		s.writeError(w, http.StatusInternalServerError, "Failed to claim source")
		return
	}
	token := hex.EncodeToString(random[:])
	source, err := s.store.Sources().Claim(r.Context(), sourceHash(req.Code), sourceHash(token), req.Name)
	if errors.Is(err, store.ErrInvalidSourceName) {
		s.auditRefusal(r.Context(), "", r, "log_source.claim", "outcome=refused reason=bad_name")
		s.writeError(w, http.StatusBadRequest, "Invalid source name")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		s.auditRefusal(r.Context(), "", r, "log_source.claim", "outcome=failure reason=invalid_code")
		s.writeError(w, http.StatusForbidden, "Invalid or expired pairing code")
		return
	}
	if err != nil {
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=failure reason=store")
		s.writeError(w, http.StatusInternalServerError, "Failed to claim source")
		return
	}
	s.audit(r.Context(), "", r, "log_source.claim", source.ID, "outcome=success")
	s.writeJSON(w, http.StatusOK, map[string]any{"token": token, "source": map[string]any{"id": source.ID, "name": source.Name, "target_id": source.TargetID, "offboarding_alerts": source.OffboardingAlerts}})
}

func sixDigits(code string) bool {
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Server) handleListLogSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.store.Sources().List(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list sources")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}

func (s *Server) handleRevokeLogSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.Sources().Revoke(r.Context(), id, time.Now().UTC())
	if errors.Is(err, store.ErrNotFound) {
		s.audit(r.Context(), s.actorID(r), r, "admin.log_source_revoke", "", "outcome=refused reason=unknown_source")
		s.writeError(w, http.StatusNotFound, "Source not found")
		return
	}
	if err != nil {
		s.audit(r.Context(), s.actorID(r), r, "admin.log_source_revoke", "", "outcome=failure reason=store")
		s.writeError(w, http.StatusInternalServerError, "Failed to revoke source")
		return
	}
	s.audit(r.Context(), s.actorID(r), r, "admin.log_source_revoke", id, "outcome=success")
	s.writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
