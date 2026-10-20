package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Busnes-app/ky-primitives/syncauth"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const directoryMaxBody = 64 << 10

// handleInstallerDirectory accepts KyIdentity suite-directory deliveries signed with
// ky-primitives syncauth v1 under the installer's directory secret.
func (s *Server) handleInstallerDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	issuer, secret := s.config.SSO.KySignOnIssuer, s.config.SSO.KySignOnHMACSecret
	if issuer == "" || secret == "" {
		s.writeError(w, http.StatusNotFound, "Directory is not configured")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, directoryMaxBody+1))
	if err != nil || len(raw) > directoryMaxBody {
		s.writeError(w, http.StatusBadRequest, "Invalid directory body")
		return
	}
	event, err := syncauth.Verify([]byte(secret), syncauth.FromRequest(r), raw, syncauth.Options{})
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "Invalid directory signature")
		return
	}
	if event.Type == "user.mfa_reset" {
		// kyPulse holds no KyIdentity MFA state; acknowledged so it does not stay pending.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var res struct {
		ID     string `json:"id"`
		Active *bool  `json:"active"`
		Meta   struct {
			Version string `json:"version"`
		} `json:"meta"`
		Roles []struct {
			Value string `json:"value"`
		} `json:"roles"`
	}
	if json.Unmarshal(raw, &res) != nil || res.ID == "" || len(res.ID) > 255 {
		s.writeError(w, http.StatusBadRequest, "Invalid directory identity")
		return
	}
	// meta.version is the sender's revision as a weak ETag, W/"<n>".
	v := strings.TrimSuffix(strings.TrimPrefix(res.Meta.Version, `W/"`), `"`)
	revision, err := strconv.ParseUint(v, 10, 64)
	if err != nil || res.Meta.Version != `W/"`+v+`"` || strconv.FormatUint(revision, 10) != v ||
		(event.Type == "user.created" || event.Type == "user.updated") && res.Active == nil {
		s.writeError(w, http.StatusBadRequest, "Invalid directory revision or assignment")
		return
	}
	admin := false
	for _, role := range res.Roles {
		admin = admin || role.Value == "kypulse.admin" || role.Value == store.RoleAdmin
	}
	sum := sha256.Sum256(append([]byte(event.Type+"\x00"), raw...))
	err = s.store.ApplyInstallerDirectory(r.Context(), store.DirectoryUpdate{
		Issuer: issuer, Subject: res.ID, Event: event.Type, Digest: hex.EncodeToString(sum[:]),
		Revision: revision, Assigned: res.Active != nil && *res.Active, Admin: admin,
	})
	switch {
	case errors.Is(err, store.ErrDirectoryRefused):
		s.writeError(w, http.StatusUnprocessableEntity, "Directory revision or identity refused")
	case err != nil:
		s.writeError(w, http.StatusServiceUnavailable, "Directory unavailable")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
