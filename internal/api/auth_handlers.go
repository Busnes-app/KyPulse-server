package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Busnes-app/ky-primitives/password"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

type LoginRequest struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	CaptchaToken string `json:"captcha_token,omitempty"`
}

// loginFailed answers every rejected credential, locked accounts included.
const loginFailed = "Invalid credentials or too many attempts"

type MFARequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

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

	// Verify PoW CAPTCHA if enabled
	if s.config.Captcha.Provider == "pow" {
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
	// Per account, so a botnet spread across many IPs cannot outrun the per-IP window. Only
	// wrong passwords spend it, so signing in often never locks an account.
	accountKey := "login-user:" + user.ID
	if !s.accountOpen(accountKey, 10) {
		password.DummyVerify()
		fail(user.ID, "account_rate_limit", http.StatusUnauthorized, loginFailed)
		return
	}
	ok, err := password.Verify(req.Password, user.PasswordHash)
	if err != nil || !ok {
		s.allowAccount(accountKey, 10, 15*time.Minute)
		fail(user.ID, "password", http.StatusUnauthorized, loginFailed)
		return
	}
	if user.Status != "active" {
		fail(user.ID, "inactive", http.StatusForbidden, "Account is disabled or inactive")
		return
	}

	// Check if MFA is required
	if user.TOTPEnabled {
		rawChallenge := crypto.RandomHex(32)
		if err := s.store.Sessions().CreateMFAChallenge(r.Context(), &store.MFAChallenge{
			TokenHash: crypto.SHA256Hex([]byte(rawChallenge)),
			UserID:    user.ID,
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
		}, user.PasswordHash); err != nil {
			s.writeError(w, http.StatusInternalServerError, "Failed to start MFA verification")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{
			"mfa_required": true,
			"mfa_token":    rawChallenge,
			"methods":      []string{"totp", "recovery_code"},
		})
		return
	}

	// Issue active session
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

// mfaUser runs the checks both MFA routes share and returns the account the transaction
// belongs to. It has answered and audited the request when ok is false.
func (s *Server) mfaUser(w http.ResponseWriter, r *http.Request, action string) (user *store.User, req MFARequest, ok bool) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return nil, req, false
	}
	if !isJSON(r) {
		s.writeError(w, http.StatusUnsupportedMediaType, "Expected application/json")
		return nil, req, false
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return nil, req, false
	}
	fail := func(userID, reason string, status int, message string) {
		s.auditRefusal(r.Context(), userID, r, action, "outcome=failure reason="+reason)
		s.writeError(w, status, message)
	}
	// Key on the client alone. Nothing caller-supplied may reach the key: a token in the key
	// lets one client mint unbounded slots and starve every other window. Same shape as the
	// login limit, so a client occupies one MFA slot no matter how it spends it.
	if !s.allowAttempt("mfa:"+s.limitIP(r), 20, time.Minute) {
		fail("", "rate_limit", http.StatusTooManyRequests, "Too many MFA attempts")
		return nil, req, false
	}

	userID, verifiedHash, err := s.store.Sessions().ConsumeMFAChallenge(r.Context(), crypto.SHA256Hex([]byte(req.MFAToken)))
	if err != nil {
		fail("", "transaction", http.StatusUnauthorized, "Invalid or expired MFA transaction")
		return nil, req, false
	}
	// Per-account, so a botnet spread across many IPs cannot outrun the per-IP window. Each
	// guess already costs a fresh password login, since the challenge is consumed either way.
	if !s.allowAccount("mfa-user:"+userID, 5, 5*time.Minute) {
		fail(userID, "account_rate_limit", http.StatusTooManyRequests, "Too many MFA attempts for this account")
		return nil, req, false
	}
	user, err = s.store.Users().GetUserByID(r.Context(), userID)
	if err != nil {
		fail(userID, "unknown_user", http.StatusUnauthorized, "User not found")
		return nil, req, false
	}
	if user.Status != "active" || !user.TOTPEnabled || user.PasswordHash != verifiedHash {
		fail(userID, "ineligible", http.StatusForbidden, "Account is not eligible for MFA login")
		return nil, req, false
	}
	return user, req, true
}

func (s *Server) handleMFATOTP(w http.ResponseWriter, r *http.Request) {
	const action = "auth.mfa_totp"
	user, req, ok := s.mfaUser(w, r, action)
	if !ok {
		return
	}

	secretBytes, err := crypto.DecryptAESGCM(user.TOTPSecretEnc, s.config.Security.EncryptionKey)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to decrypt MFA secret")
		return
	}

	counter, ok := auth.ValidateTOTP(string(secretBytes), req.Code)
	if !ok {
		s.auditRefusal(r.Context(), user.ID, r, action, "outcome=failure reason=code")
		s.writeError(w, http.StatusUnauthorized, "Invalid TOTP verification code")
		return
	}
	if err := s.store.Users().SpendTOTPCounter(r.Context(), user.ID, counter); err != nil {
		s.auditRefusal(r.Context(), user.ID, r, action, "outcome=failure reason=replay")
		s.writeError(w, http.StatusUnauthorized, "TOTP code already used")
		return
	}

	_, _, err = s.sessions.IssueSession(r.Context(), w, r, user)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to issue session")
		return
	}

	s.audit(r.Context(), user.ID, r, action, "session", "outcome=success")

	s.writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user":          user,
	})
}

func (s *Server) handleMFARecovery(w http.ResponseWriter, r *http.Request) {
	const action = "auth.mfa_recovery_code"
	user, req, ok := s.mfaUser(w, r, action)
	if !ok {
		return
	}

	updatedHashes, ok := auth.RedeemRecoveryCode(req.Code, user.RecoveryCodesHash)
	if !ok {
		s.auditRefusal(r.Context(), user.ID, r, action, "outcome=failure reason=code")
		s.writeError(w, http.StatusUnauthorized, "Invalid or already used recovery code")
		return
	}

	if err := s.store.Users().UpdateRecoveryCodes(r.Context(), user.ID, user.RecoveryCodesHash, updatedHashes); err != nil {
		s.auditRefusal(r.Context(), user.ID, r, action, "outcome=failure reason=replay")
		s.writeError(w, http.StatusUnauthorized, "Recovery code was already used")
		return
	}

	_, _, err := s.sessions.IssueSession(r.Context(), w, r, user)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to issue session")
		return
	}

	s.audit(r.Context(), user.ID, r, action, "session", "outcome=success")

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
	_ = s.sessions.RevokeSession(r.Context(), w, r)
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
