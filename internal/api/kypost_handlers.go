package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/kypost"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
)

// mailOwners are the accounts that may own the sending mailbox: active and signed in through
// KyIdentity, so KyPost can resolve them by subject and issuer.
func (s *Server) mailOwners(r *http.Request) ([]map[string]string, error) {
	users, _, err := s.store.Users().ListUsers(r.Context(), 0, 500, "")
	if err != nil {
		return nil, err
	}
	out := []map[string]string{}
	for _, u := range users {
		if monitor.Eligible(u) {
			name := u.DisplayName
			if name == "" {
				name = u.Username
			}
			out = append(out, map[string]string{"id": u.ID, "name": name})
		}
	}
	return out, nil
}

func (s *Server) handleGetKyPostMail(w http.ResponseWriter, r *http.Request) {
	m := s.monitor.Mail
	if m == nil || !m.Link.Available() {
		s.writeJSON(w, http.StatusOK, map[string]any{"available": false, "reason": kypost.ReasonNoKyIdentity})
		return
	}
	paired, reason := m.Link.Status()
	owners, err := s.mailOwners(r)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list mailbox owners")
		return
	}
	out := map[string]any{"available": true, "paired": paired, "reason": reason, "owners": owners, "configured": false}
	cfg, ok, err := m.Load(r.Context())
	switch {
	case err != nil:
		out["configured"], out["unreadable"] = true, true
	case ok:
		out["configured"], out["owner_id"], out["recipients"] = true, cfg.OwnerID, cfg.Recipients
	}
	if last, ok := m.Status(r.Context()); ok {
		out["last"] = last
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSetKyPostMail(w http.ResponseWriter, r *http.Request) {
	m := s.monitor.Mail
	if m == nil || !m.Link.Available() {
		s.writeError(w, http.StatusConflict, "KyPulse does not sign in through KyIdentity")
		return
	}
	var in monitor.MailConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	if err := in.Validate(); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if u, err := s.store.Users().GetUserByID(r.Context(), in.OwnerID); err != nil || !monitor.Eligible(u) {
		s.writeError(w, http.StatusBadRequest, "The mailbox owner must be an active account that signs in through KyIdentity")
		return
	}
	if err := m.Save(r.Context(), in); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to save the channel")
		return
	}
	// Addresses are personal data: the audit names the owner's ID and a count only.
	s.audit(r.Context(), s.actorID(r), r, "admin.kypost_mail_set", in.OwnerID, fmt.Sprintf("recipients=%d", len(in.Recipients)))
	s.handleGetKyPostMail(w, r)
}

func (s *Server) handleDeleteKyPostMail(w http.ResponseWriter, r *http.Request) {
	if m := s.monitor.Mail; m != nil {
		if err := m.Delete(r.Context()); err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to delete the channel")
			return
		}
	}
	s.audit(r.Context(), s.actorID(r), r, "admin.kypost_mail_delete", "", "")
	w.WriteHeader(http.StatusNoContent)
}

// handleTestKyPostMail makes one attempt, so the answer arrives inside the request. A refusal
// is 502 with KyPost's code, never an address.
func (s *Server) handleTestKyPostMail(w http.ResponseWriter, r *http.Request) {
	m := s.monitor.Mail
	cfg, ok, err := m.Load(r.Context())
	if m == nil || err != nil || !ok {
		s.writeError(w, http.StatusPreconditionFailed, "Email through KyPost is not set up")
		return
	}
	msg := notify.Message{App: "KyPulse", State: "ok", Previous: "ok", Time: time.Now(), URL: s.config.Server.AppURL, Test: true}
	err = m.Send(r.Context(), cfg, "kypulse-test-"+crypto.RandomHex(16), msg, false)
	reason := kypost.Reason(err)
	_ = m.SetStatus(r.Context(), monitor.DeliveryStatus{At: time.Now(), OK: err == nil, Error: reason})
	details := "outcome=sent"
	if err != nil {
		details = "reason=" + reason
	}
	s.audit(r.Context(), s.actorID(r), r, "admin.kypost_mail_test", cfg.OwnerID, details)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, map[string]string{"error": reason})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
