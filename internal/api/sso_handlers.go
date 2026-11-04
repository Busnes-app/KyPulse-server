package api

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/sso"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"golang.org/x/oauth2"
)

var evSSOFailed = logging.DeclareEvent("sso_failed", "SSO sign-in failed", slog.LevelWarn)

// ssoOn is the one rule for KySignOn sign-in: enabled and an issuer set. The login page
// offers the button on the same rule, and both sign-in routes refuse without it.
func (s *Server) ssoOn() bool {
	return s.config.SSO.Enabled && s.config.SSO.KySignOnIssuer != ""
}

func (s *Server) handleKySignOnLogin(w http.ResponseWriter, r *http.Request) {
	if !s.ssoOn() {
		s.writeError(w, http.StatusNotFound, "SSO is not enabled")
		return
	}
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
	if !s.ssoOn() {
		s.writeError(w, http.StatusNotFound, "SSO is not enabled")
		return
	}
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
		if err := s.store.Users().CreateUser(r.Context(), user); errors.Is(err, store.ErrAlreadyExists) {
			// Another account, local or federated, already holds this username.
			s.writeError(w, http.StatusConflict, "An account with this username already exists here; ask an administrator")
			return
		} else if err != nil {
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
	default:
		// KyIdentity is the authority for an SSO user's role, except an installer-owned
		// identity's, which the installer directory governs.
		if !s.applySSORole(w, r, user, sso.RoleFor(claims.Role)) {
			return
		}
	}

	_, _, err = s.sessions.IssueSSOSession(r.Context(), w, r, user, claims.SessionID, claims.IssuedAt)
	if errors.Is(err, store.ErrSSOSessionEnded) {
		s.writeError(w, http.StatusUnauthorized, "SSO session already ended")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Session creation failed")
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

// applySSORole moves a non-installer-owned user to role and ends every session they hold,
// before the new one is issued. It reports false after writing an error.
func (s *Server) applySSORole(w http.ResponseWriter, r *http.Request, user *store.User, role string) bool {
	if user.Role == role {
		return true
	}
	_, err := s.store.Settings().GetSetting(r.Context(), store.InstallerUserKey(s.config.SSO.KySignOnIssuer, user.SSOSubject))
	if err == nil {
		return true
	}
	if !errors.Is(err, store.ErrNotFound) {
		s.writeError(w, http.StatusInternalServerError, "Authentication error")
		return false
	}
	from := user.Role
	user.Role = role
	if err := s.store.Users().UpdateUser(r.Context(), user); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Authentication error")
		return false
	}
	if err := s.store.Sessions().DeleteUserSessions(r.Context(), user.ID); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Authentication error")
		return false
	}
	s.audit(r.Context(), user.ID, r, "auth.sso_role_changed", "user", "from="+from+" to="+role)
	return true
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

// ssoLogoutFence outlives the 5-minute login callback window, so a logout that lands
// while the browser is still finishing that login also ends it.
const ssoLogoutFence = 10 * time.Minute

// handleKySignOnBackchannelLogout is the OpenID Connect Back-Channel Logout receiver: KySignOn
// POSTs a signed logout_token when a login ends, and every local session of it ends here.
func (s *Server) handleKySignOnBackchannelLogout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	claims, err := s.kysignon.VerifyLogoutToken(r.Context(), r.PostFormValue("logout_token"))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	fenceUntil := time.Now().Add(ssoLogoutFence)
	if claims.SessionID != "" {
		err = s.store.Sessions().EndSSOSession(r.Context(), claims.SessionID, fenceUntil)
	} else {
		// iat has whole-second precision: round up so a login from that same second ends too.
		err = s.store.Sessions().EndSSOSubject(r.Context(), "kysignon", claims.Subject, claims.IssuedAt.Add(time.Second), fenceUntil)
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	s.audit(r.Context(), "", r, "auth.sso_backchannel_logout", "session", "")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(s.saml.GenerateMetadata()))
}
