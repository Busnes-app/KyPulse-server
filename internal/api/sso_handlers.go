package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/sso"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"golang.org/x/oauth2"
)

var evSSOFailed = logging.DeclareEvent("sso_failed", "SSO sign-in failed", slog.LevelWarn)

func (s *Server) handleKySignOnLogin(w http.ResponseWriter, r *http.Request) {
	state := crypto.RandomHex(16)
	nonce := crypto.RandomHex(16)
	verifier := oauth2.GenerateVerifier()

	redirectURI := s.config.Server.AppURL + ssoCallbackPath
	authURL, err := s.kysignon.BuildAuthURL(r.Context(), redirectURI, state, verifier, nonce)
	if err != nil {
		s.lg.Log(r.Context(), evSSOFailed, logging.Err(err))
		s.writeError(w, http.StatusBadRequest, "SSO is unavailable")
		return
	}

	// Verifier and nonce live in short-lived cookies only the callback receives.
	s.setSSOCookie(w, "ky_pkce_"+state, verifier, 300)
	s.setSSOCookie(w, "ky_nonce_"+state, nonce, 300)

	http.Redirect(w, r, authURL, http.StatusFound)
}

const ssoCallbackPath = "/api/sso/kysignon/callback"

func (s *Server) setSSOCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     ssoCallbackPath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.config.Security.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) handleKySignOnCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	// One attempt per login: the verifier and nonce are spent whatever happens next.
	s.setSSOCookie(w, "ky_pkce_"+state, "", -1)
	s.setSSOCookie(w, "ky_nonce_"+state, "", -1)

	cookie, err := r.Cookie("ky_pkce_" + state)
	if err != nil || cookie.Value == "" {
		s.writeError(w, http.StatusBadRequest, "Invalid or expired SSO state")
		return
	}
	verifier := cookie.Value
	nonceCookie, err := r.Cookie("ky_nonce_" + state)
	if err != nil || nonceCookie.Value == "" {
		s.writeError(w, http.StatusBadRequest, "Invalid or expired SSO nonce")
		return
	}

	redirectURI := s.config.Server.AppURL + ssoCallbackPath
	claims, err := s.kysignon.ExchangeCode(r.Context(), code, verifier, redirectURI, nonceCookie.Value)
	if err != nil {
		// The cause may quote the identity provider's response body: log it, never return it.
		s.lg.Log(r.Context(), evSSOFailed, logging.Err(err))
		s.writeError(w, http.StatusUnauthorized, "SSO sign-in failed")
		return
	}

	// Upsert user
	user, err := s.store.Users().GetUserBySSO(r.Context(), "kysignon", claims.Subject)
	switch {
	case errors.Is(err, store.ErrNotFound) && s.config.SSO.AutoProvision:
		user = &store.User{
			ID:          fmt.Sprintf("usr_%s", crypto.RandomHex(12)),
			Username:    claims.PreferredUsername,
			Email:       claims.Email,
			DisplayName: claims.Name,
			Role:        sso.RoleFor(claims.Role),
			Status:      "active",
			SSOProvider: "kysignon",
			SSOSubject:  claims.Subject,
		}
		if err := s.store.Users().CreateUser(r.Context(), user); err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to provision SSO user")
			return
		}
	case errors.Is(err, store.ErrNotFound):
		s.writeError(w, http.StatusForbidden, "User account not provisioned")
		return
	case err != nil:
		s.writeError(w, http.StatusInternalServerError, "Authentication error")
		return
	case user.Status != "active":
		s.writeError(w, http.StatusForbidden, "Account is disabled or inactive")
		return
	}

	_, _, err = s.sessions.IssueSession(r.Context(), w, r, user)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Session creation failed")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleKySignOnSyncWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	sig := r.Header.Get("X-KySignOn-Signature")
	if strings.HasPrefix(sig, "v1=") {
		// KyIdentity's suite webhook (syncauth v1) may be pointed at this legacy path.
		s.handleInstallerDirectory(w, r)
		return
	}
	if sig == "" {
		sig = r.Header.Get("X-Signature-SHA256")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	if err := s.kysignon.HandleSyncWebhook(r.Context(), body, sig); err != nil {
		s.writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]bool{"synced": true})
}

func (s *Server) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(s.saml.GenerateMetadata()))
}
