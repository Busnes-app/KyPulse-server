package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Busnes-app/ky-primitives/password"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

type LoginRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	CaptchaToken string `json:"captcha_token,omitempty"`
}

// loginFailed answers every rejected credential, locked accounts included.
const loginFailed = "Invalid credentials or too many attempts"

func (s *Server) handlePoWChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	challenge, err := auth.GeneratePoWChallenge(s.config.Captcha.DifficultyPoW, s.config.Security.SessionSecret)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to generate PoW challenge")
		return
	}

	s.writeJSON(w, http.StatusOK, challenge)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if !isJSON(r) {
		s.writeError(w, http.StatusUnsupportedMediaType, "Expected application/json")
		return
	}
	fail := func(userID, reason string, status int, message string) {
		s.auditRefusal(r.Context(), userID, r, "auth.login", "outcome=failure reason="+reason)
		s.writeError(w, status, message)
	}
	if !s.allowAttempt("login:"+s.limitIP(r), 20, time.Minute) {
		fail("", "rate_limit", http.StatusTooManyRequests, "Too many login attempts")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		s.writeError(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	// PoW is the only CAPTCHA implemented; only an explicit "none" turns it off, so a value
	// naming an unimplemented provider fails closed instead of disabling the check.
	if s.config.Captcha.Provider != "none" {
		if req.CaptchaToken == "" || !s.pow.Verify(req.CaptchaToken, s.config.Security.SessionSecret) {
			fail("", "captcha", http.StatusForbidden, "Security check failed. Please complete the CAPTCHA puzzle.")
			return
		}
	}

	// Unknown, wrong, locked and disabled-with-wrong-password all cost one verification and
	// get the same answer, so none of them reveals whether the account exists.
	user, err := s.store.Users().GetUserByUsername(r.Context(), req.Username)
	if errors.Is(err, store.ErrNotFound) {
		password.DummyVerify()
		fail("", "unknown_user", http.StatusUnauthorized, loginFailed)
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Authentication error")
		return
	}
	// Per account and client (IPv6 by /64): one address cannot keep guessing past the per-IP
	// window, and nobody elsewhere can lock the real owner out. Only wrong passwords spend it,
	// so signing in often never locks an account.
	accountKey := "login-user:" + user.ID + "@" + s.limitIP(r)
	if !s.loginOpen(accountKey, 10) {
		password.DummyVerify()
		fail(user.ID, "account_rate_limit", http.StatusUnauthorized, loginFailed)
		return
	}
	ok, err := password.Verify(req.Password, user.PasswordHash)
	if err != nil || !ok {
		s.spendLogin(accountKey, 15*time.Minute)
		fail(user.ID, "password", http.StatusUnauthorized, loginFailed)
		return
	}
	if user.Status != "active" {
		fail(user.ID, "inactive", http.StatusForbidden, "Account is disabled or inactive")
		return
	}

	// A correct password signs in. Second factors are KyIdentity's; a stored local TOTP
	// state is kept but no longer asked for.
	_, _, err = s.sessions.IssueSession(r.Context(), w, r, user)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to issue session")
		return
	}

	s.audit(r.Context(), user.ID, r, "auth.login", "session", "outcome=success")

	s.writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user":          user,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if err := s.sessions.RevokeSession(r.Context(), w, r); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Sign-out failed; the session is still active")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"logged_out": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _, err := s.sessions.AuthenticateRequest(r)
	if err != nil {
		s.writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user":          user,
	})
}
