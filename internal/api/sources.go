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
		TargetID string `json:"target_id"`
	}
	if err := oneJSON(w, r, &req); err != nil {
		s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", "", "outcome=refused reason=bad_json")
		s.writeError(w, http.StatusBadRequest, "Invalid request")
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
		err = s.store.Sources().CreateCode(r.Context(), sourceHash(code), req.TargetID, expires)
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
		s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", req.TargetID, "outcome=success")
		s.writeJSON(w, http.StatusOK, map[string]any{"code": code, "expires_at": expires})
		return
	}
	s.audit(r.Context(), s.actorID(r), r, "admin.log_pairing", "", "outcome=refused reason=collision")
	s.writeError(w, http.StatusServiceUnavailable, "Could not create pairing code")
}

func (s *Server) handleClaimLogSource(w http.ResponseWriter, r *http.Request) {
	if !s.allowClaim(s.requestIP(r)) {
		w.Header().Set("Retry-After", "60")
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=refused reason=rate_limit")
		s.writeError(w, http.StatusTooManyRequests, "Too many attempts")
		return
	}
	var req struct {
		Code string `json:"pairing_code"`
		Name string `json:"name"`
	}
	if err := oneJSON(w, r, &req); err != nil {
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=refused reason=bad_json")
		s.writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if len(req.Code) != 6 || !sixDigits(req.Code) {
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=failure reason=invalid_code")
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
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=refused reason=bad_name")
		s.writeError(w, http.StatusBadRequest, "Invalid source name")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=failure reason=invalid_code")
		s.writeError(w, http.StatusForbidden, "Invalid or expired pairing code")
		return
	}
	if err != nil {
		s.audit(r.Context(), "", r, "log_source.claim", "", "outcome=failure reason=store")
		s.writeError(w, http.StatusInternalServerError, "Failed to claim source")
		return
	}
	s.audit(r.Context(), "", r, "log_source.claim", source.ID, "outcome=success")
	s.writeJSON(w, http.StatusOK, map[string]any{"token": token, "source": map[string]string{"id": source.ID, "name": source.Name, "target_id": source.TargetID}})
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
