package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Busnes-app/ky-primitives/password"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// mfaSessionUser returns the signed-in local account a TOTP management route acts on. Accounts
// that do not sign in with a local password are refused with 409: MFA guards only that sign-in,
// and turning it off needs the password. It has answered the request when ok is false.
func (s *Server) mfaSessionUser(w http.ResponseWriter, r *http.Request) (*store.User, *store.Session, bool) {
	user, sess, err := s.sessions.AuthenticateRequest(r)
	if err != nil {
		s.writeError(w, http.StatusUnauthorized, "Authentication required")
		return nil, nil, false
	}
	if user.SSOProvider != "local" || user.PasswordHash == "" {
		s.writeError(w, http.StatusConflict, "Two-factor authentication is managed by your identity provider")
		return nil, nil, false
	}
	return user, sess, true
}

// handleMFAEnroll starts (or restarts) enrolment: a fresh secret is sealed and stored pending,
// with totp_enabled still false, and returned once so an authenticator app can add it.
func (s *Server) handleMFAEnroll(w http.ResponseWriter, r *http.Request) {
	user, _, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	if user.TOTPEnabled {
		s.writeError(w, http.StatusConflict, "Two-factor authentication is already on; turn it off first")
		return
	}
	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to start enrolment")
		return
	}
	// Sealed exactly the way handleMFATOTP opens it.
	sealed, err := crypto.EncryptAESGCM([]byte(secret), s.config.Security.EncryptionKey)
	if err == nil {
		// Compare-and-swap on the secret read above and totp_enabled = false: a concurrent
		// confirm that enabled TOTP in between makes this fail rather than replace it.
		err = s.store.Users().SetTOTPSecret(r.Context(), user.ID, user.TOTPSecretEnc, sealed)
	}
	if errors.Is(err, store.ErrAlreadyExists) {
		s.writeError(w, http.StatusConflict, "Account changed; try again")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to start enrolment")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{
		"secret":      secret,
		"otpauth_uri": auth.GenerateTOTPURL(s.config.Server.AppName, user.Username, secret),
	})
}

// handleMFAEnrollConfirm turns the pending secret on once the user proves their app has it.
func (s *Server) handleMFAEnrollConfirm(w http.ResponseWriter, r *http.Request) {
	const action = "auth.mfa_enroll"
	user, sess, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	fail := func(reason string, status int, message string) {
		s.auditRefusal(r.Context(), user.ID, r, action, "outcome=failure reason="+reason)
		s.writeError(w, status, message)
	}
	if !s.allowAccount("mfa-enroll-user:"+user.ID, 5, 5*time.Minute) {
		fail("account_rate_limit", http.StatusTooManyRequests, "Too many attempts")
		return
	}
	if !isJSON(r) {
		s.writeError(w, http.StatusUnsupportedMediaType, "Expected application/json")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if user.TOTPEnabled || user.TOTPSecretEnc == "" {
		s.writeError(w, http.StatusConflict, "No enrolment is pending")
		return
	}
	secret, err := crypto.DecryptAESGCM(user.TOTPSecretEnc, s.config.Security.EncryptionKey)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to decrypt MFA secret")
		return
	}
	counter, ok := auth.ValidateTOTP(string(secret), req.Code)
	if !ok {
		fail("code", http.StatusUnauthorized, "Invalid TOTP verification code")
		return
	}
	if err := s.store.Users().SpendTOTPCounter(r.Context(), user.ID, counter); err != nil {
		fail("replay", http.StatusUnauthorized, "TOTP code already used")
		return
	}
	codes, hashes, err := auth.GenerateRecoveryCodes(10)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to generate recovery codes")
		return
	}
	// Enables only the secret the code was checked against; a restarted enrolment fails it.
	err = s.store.Users().EnableTOTP(r.Context(), user.ID, user.TOTPSecretEnc, hashes, sess.TokenHash, s.requestIP(r))
	if errors.Is(err, store.ErrAlreadyExists) {
		s.writeError(w, http.StatusConflict, "Enrolment changed; start again")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to enable two-factor authentication")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"totp_enabled": true, "recovery_codes": codes})
}

// handleMFADisable turns TOTP off. It needs the password and a current TOTP or unused
// recovery code, so a stolen session alone cannot remove the second factor.
func (s *Server) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	const action = "auth.mfa_disable"
	user, sess, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	fail := func(reason string, status int, message string) {
		s.auditRefusal(r.Context(), user.ID, r, action, "outcome=failure reason="+reason)
		s.writeError(w, status, message)
	}
	if !s.allowAccount("mfa-disable-user:"+user.ID, 5, 5*time.Minute) {
		fail("account_rate_limit", http.StatusTooManyRequests, "Too many attempts")
		return
	}
	if !isJSON(r) {
		s.writeError(w, http.StatusUnsupportedMediaType, "Expected application/json")
		return
	}
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Password) > 1024 {
		s.writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !user.TOTPEnabled {
		s.writeError(w, http.StatusConflict, "Two-factor authentication is off")
		return
	}
	if ok, err := password.Verify(req.Password, user.PasswordHash); err != nil || !ok {
		fail("password", http.StatusUnauthorized, "Password or code is incorrect")
		return
	}
	if !s.spendSecondFactor(r, user, req.Code) {
		fail("code", http.StatusUnauthorized, "Password or code is incorrect")
		return
	}
	err := s.store.Users().DisableTOTP(r.Context(), user.ID, sess.TokenHash, s.requestIP(r))
	if errors.Is(err, store.ErrAlreadyExists) {
		s.writeError(w, http.StatusConflict, "Two-factor authentication is off")
		return
	}
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to disable two-factor authentication")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]bool{"totp_enabled": false})
}

// spendSecondFactor accepts a current TOTP code or an unused recovery code and spends it, so
// neither can be used again.
func (s *Server) spendSecondFactor(r *http.Request, user *store.User, code string) bool {
	if secret, err := crypto.DecryptAESGCM(user.TOTPSecretEnc, s.config.Security.EncryptionKey); err == nil {
		if counter, ok := auth.ValidateTOTP(string(secret), code); ok {
			return s.store.Users().SpendTOTPCounter(r.Context(), user.ID, counter) == nil
		}
	}
	next, ok := auth.RedeemRecoveryCode(code, user.RecoveryCodesHash)
	return ok && s.store.Users().UpdateRecoveryCodes(r.Context(), user.ID, user.RecoveryCodesHash, next) == nil
}
