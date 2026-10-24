package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/totp"
	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const mfaTestPassword = "SuperSecretPass123!"

// mfaPost sends a JSON POST as the holder of session, echoing the CSRF cookie unless noCSRF.
func mfaPost(t *testing.T, srv *api.Server, session *http.Cookie, path, body string, noCSRF bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if session != nil {
		req.AddCookie(session)
		if !noCSRF {
			req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "test-csrf"})
			req.Header.Set(auth.HeaderCSRF, "test-csrf")
		}
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie: %d %s", w.Code, w.Body.String())
	return nil
}

func getUser(t *testing.T, st store.Store, username string) *store.User {
	t.Helper()
	u, err := st.Users().GetUserByUsername(context.Background(), username)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func totpCode(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	code, err := totp.Code(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// startEnrol starts enrolment and returns the secret the server handed out.
func startEnrol(t *testing.T, srv *api.Server, session *http.Cookie) string {
	t.Helper()
	w := mfaPost(t, srv, session, "/api/auth/mfa/enroll", "", false)
	if w.Code != http.StatusOK {
		t.Fatalf("enroll: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Secret     string `json:"secret"`
		OTPAuthURI string `json:"otpauth_uri"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Secret == "" || !strings.HasPrefix(resp.OTPAuthURI, "otpauth://totp/") || !strings.Contains(resp.OTPAuthURI, "secret="+resp.Secret) {
		t.Fatalf("enroll response: %+v", resp)
	}
	return resp.Secret
}

// enrol runs enrolment to the end with a code for now, the first of the two codes a test
// may use (one step of skew either way keeps now and now+30s valid across a step boundary).
func enrol(t *testing.T, srv *api.Server, session *http.Cookie) (secret string, recovery []string) {
	t.Helper()
	secret = startEnrol(t, srv, session)
	w := mfaPost(t, srv, session, "/api/auth/mfa/enroll/confirm", `{"password":"`+mfaTestPassword+`","code":"`+totpCode(t, secret, time.Now())+`"}`, false)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Enabled  bool     `json:"totp_enabled"`
		Recovery []string `json:"recovery_codes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Enabled || len(resp.Recovery) != 10 {
		t.Fatalf("confirm response: %s", w.Body.String())
	}
	return secret, resp.Recovery
}

// The bug this fixes: a local account had no way to turn TOTP on. Enrolment must need a
// valid code, and once on, sign-in must stop at the second factor and accept the secret
// enrolment stored (the sign-in path opens what enrolment sealed).
func TestMFAEnrolmentNeedsAValidCodeThenGuardsSignIn(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	session := loginAs(t, srv, st, "erin", "viewer")
	other := sessionCookie(t, loginFrom(t, srv, "192.0.2.20:1000", "erin", mfaTestPassword))

	secret := startEnrol(t, srv, session)
	u := getUser(t, st, "erin")
	if u.TOTPEnabled || u.TOTPSecretEnc == "" || strings.Contains(u.TOTPSecretEnc, secret) {
		t.Fatalf("pending enrolment: enabled=%v sealed=%q", u.TOTPEnabled, u.TOTPSecretEnc)
	}
	// A pending enrolment is not a second factor yet.
	if w := loginFrom(t, srv, "192.0.2.21:1000", "erin", mfaTestPassword); !hasSession(w) {
		t.Fatalf("pending enrolment blocked sign-in: %d %s", w.Code, w.Body.String())
	}

	// A stolen session alone cannot enrol: the right code still needs the password.
	withoutPassword := `{"code":"` + totpCode(t, secret, time.Now()) + `"}`
	wrongPassword := `{"password":"WrongPassword123!","code":"` + totpCode(t, secret, time.Now()) + `"}`
	for _, bad := range []string{`{"password":"` + mfaTestPassword + `","code":"not-a-code"}`, `{}`, withoutPassword, wrongPassword} {
		if w := mfaPost(t, srv, session, "/api/auth/mfa/enroll/confirm", bad, false); w.Code != http.StatusUnauthorized {
			t.Fatalf("confirm %s: got %d, want 401", bad, w.Code)
		}
	}
	if getUser(t, st, "erin").TOTPEnabled {
		t.Fatal("a wrong code enrolled TOTP")
	}

	w := mfaPost(t, srv, session, "/api/auth/mfa/enroll/confirm", `{"password":"`+mfaTestPassword+`","code":"`+totpCode(t, secret, time.Now())+`"}`, false)
	if w.Code != http.StatusOK {
		t.Fatalf("confirm: %d %s", w.Code, w.Body.String())
	}
	if !getUser(t, st, "erin").TOTPEnabled {
		t.Fatal("valid confirm did not enable TOTP")
	}
	// Enabling keeps this session and ends every other one.
	if got := do(t, srv, "GET", "/api/status", session).Code; got != http.StatusOK {
		t.Fatalf("enrolling session revoked: %d", got)
	}
	if got := do(t, srv, "GET", "/api/status", other).Code; got != http.StatusUnauthorized {
		t.Fatalf("other session survived enrolment: %d", got)
	}

	login := loginFrom(t, srv, "192.0.2.22:1000", "erin", mfaTestPassword)
	var challenge struct {
		Required bool   `json:"mfa_required"`
		Token    string `json:"mfa_token"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &challenge)
	if login.Code != http.StatusOK || !challenge.Required || challenge.Token == "" || hasSession(login) {
		t.Fatalf("enrolled login: %d %s session=%v", login.Code, login.Body.String(), hasSession(login))
	}
	body, _ := json.Marshal(map[string]string{"mfa_token": challenge.Token, "code": totpCode(t, secret, time.Now().Add(30*time.Second))})
	req := httptest.NewRequest("POST", "/api/auth/mfa/totp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mw := httptest.NewRecorder()
	srv.ServeHTTP(mw, req)
	if mw.Code != http.StatusOK || !hasSession(mw) {
		t.Fatalf("MFA sign-in with the enrolled secret: %d %s", mw.Code, mw.Body.String())
	}
}

// Re-enrolment must not be a way around the factor: while TOTP is on, enroll is refused and
// the stored secret stays; at the store, a pending write cannot replace an enabled secret.
func TestMFAReEnrolmentWhileEnabledIsRefused(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	session := loginAs(t, srv, st, "fay", "viewer")
	enrol(t, srv, session)
	before := getUser(t, st, "fay")

	if w := mfaPost(t, srv, session, "/api/auth/mfa/enroll", "", false); w.Code != http.StatusConflict {
		t.Fatalf("re-enroll while enabled: got %d, want 409", w.Code)
	}
	if w := mfaPost(t, srv, session, "/api/auth/mfa/enroll/confirm", `{"code":"123456"}`, false); w.Code != http.StatusConflict {
		t.Fatalf("confirm while enabled: got %d, want 409", w.Code)
	}
	if err := st.Users().SetTOTPSecret(context.Background(), before.ID, before.TOTPSecretEnc, "replacement"); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("SetTOTPSecret over an enabled secret: %v", err)
	}
	after := getUser(t, st, "fay")
	if !after.TOTPEnabled || after.TOTPSecretEnc != before.TOTPSecretEnc || after.RecoveryCodesHash != before.RecoveryCodesHash {
		t.Fatal("re-enrolment changed the enabled factor")
	}
}

func TestMFADisableNeedsPasswordAndCurrentCode(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	session := loginAs(t, srv, st, "gus", "viewer")
	secret, _ := enrol(t, srv, session)
	// Four refusals and the success stay inside the 5-per-5-minutes account window.
	next := totpCode(t, secret, time.Now().Add(30*time.Second))

	refused := []struct{ name, body string }{
		{"wrong password", `{"password":"WrongPassword123!","code":"` + next + `"}`},
		{"no code", `{"password":"` + mfaTestPassword + `"}`},
		{"wrong code", `{"password":"` + mfaTestPassword + `","code":"not-a-code"}`},
		{"spent code", `{"password":"` + mfaTestPassword + `","code":"` + totpCode(t, secret, time.Now().Add(-30*time.Second)) + `"}`},
	}
	for _, tc := range refused {
		if w := mfaPost(t, srv, session, "/api/auth/mfa/disable", tc.body, false); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: got %d, want 401", tc.name, w.Code)
		}
		if !getUser(t, st, "gus").TOTPEnabled {
			t.Fatalf("%s disabled TOTP", tc.name)
		}
	}

	// A refused attempt with the wrong password did not spend the code.
	if w := mfaPost(t, srv, session, "/api/auth/mfa/disable", `{"password":"`+mfaTestPassword+`","code":"`+next+`"}`, false); w.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", w.Code, w.Body.String())
	}
	u := getUser(t, st, "gus")
	if u.TOTPEnabled || u.TOTPSecretEnc != "" || u.RecoveryCodesHash != "[]" {
		t.Fatalf("disable left state: enabled=%v secret=%q codes=%q", u.TOTPEnabled, u.TOTPSecretEnc, u.RecoveryCodesHash)
	}
	if w := loginFrom(t, srv, "192.0.2.30:1000", "gus", mfaTestPassword); !hasSession(w) {
		t.Fatalf("login after disable: %d %s", w.Code, w.Body.String())
	}
}

func TestMFADisableAcceptsARecoveryCodeOnce(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	session := loginAs(t, srv, st, "hal", "viewer")
	_, recovery := enrol(t, srv, session)
	before := getUser(t, st, "hal")
	// The password is checked before the code, so a wrong password spends nothing.
	if w := mfaPost(t, srv, session, "/api/auth/mfa/disable", `{"password":"nope","code":"`+recovery[0]+`"}`, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", w.Code)
	}
	if getUser(t, st, "hal").RecoveryCodesHash != before.RecoveryCodesHash {
		t.Fatal("a refused disable spent a recovery code")
	}
	if w := mfaPost(t, srv, session, "/api/auth/mfa/disable", `{"password":"`+mfaTestPassword+`","code":"`+recovery[0]+`"}`, false); w.Code != http.StatusOK {
		t.Fatalf("disable with recovery code: %d %s", w.Code, w.Body.String())
	}
	if w := mfaPost(t, srv, session, "/api/auth/mfa/disable", `{"password":"`+mfaTestPassword+`","code":"`+recovery[1]+`"}`, false); w.Code != http.StatusConflict {
		t.Fatalf("disable while off: got %d, want 409", w.Code)
	}
}

// The sign-in MFA routes are CSRF-exempt; the management routes beside them must not be.
func TestMFAManagementRequiresCSRF(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	session := loginAs(t, srv, st, "ivy", "viewer")
	for _, path := range []string{"/api/auth/mfa/enroll", "/api/auth/mfa/enroll/confirm", "/api/auth/mfa/disable"} {
		if w := mfaPost(t, srv, session, path, `{"password":"`+mfaTestPassword+`","code":"123456"}`, true); w.Code != http.StatusForbidden {
			t.Fatalf("%s without CSRF: got %d, want 403", path, w.Code)
		}
	}
	if u := getUser(t, st, "ivy"); u.TOTPSecretEnc != "" || u.TOTPEnabled {
		t.Fatal("a request without CSRF changed TOTP state")
	}

	// Enabled, a forged disable is refused too.
	enrol(t, srv, session)
	if w := mfaPost(t, srv, session, "/api/auth/mfa/disable", `{"password":"`+mfaTestPassword+`","code":"123456"}`, true); w.Code != http.StatusForbidden {
		t.Fatalf("disable without CSRF: got %d, want 403", w.Code)
	}
	if !getUser(t, st, "ivy").TOTPEnabled {
		t.Fatal("a forged disable turned TOTP off")
	}
}

// Accounts that sign in through an identity provider have no local password for TOTP to
// guard and none to turn it off with.
func TestMFAManagementRefusesSSOAccounts(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	ctx := context.Background()
	if err := st.Users().CreateUser(ctx, &store.User{
		ID: "usr_sso", Username: "sso", Role: store.RoleViewer, Status: "active", SSOProvider: "kysignon", SSOSubject: "sub-1",
	}); err != nil {
		t.Fatal(err)
	}
	raw := crypto.RandomHex(32)
	if err := st.Sessions().CreateSession(ctx, &store.Session{
		TokenHash: crypto.SHA256Hex([]byte(raw)), UserID: "usr_sso", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, store.Credentials{}); err != nil {
		t.Fatal(err)
	}
	session := &http.Cookie{Name: auth.SessionCookieName, Value: raw}
	for _, path := range []string{"/api/auth/mfa/enroll", "/api/auth/mfa/enroll/confirm", "/api/auth/mfa/disable"} {
		if w := mfaPost(t, srv, session, path, `{"password":"x","code":"123456"}`, false); w.Code != http.StatusConflict {
			t.Fatalf("SSO %s: got %d, want 409", path, w.Code)
		}
	}
	if err := st.Users().SetTOTPSecret(ctx, "usr_sso", "", "sealed"); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("store SetTOTPSecret on an SSO account: %v", err)
	}
	if u := getUser(t, st, "sso"); u.TOTPSecretEnc != "" || u.TOTPEnabled {
		t.Fatal("SSO account got TOTP state")
	}
}

// A confirm whose code was checked against one pending secret must not enable another.
func TestMFAConfirmAfterRestartedEnrolmentUsesTheNewSecret(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	session := loginAs(t, srv, st, "jo", "viewer")
	first := startEnrol(t, srv, session)
	second := startEnrol(t, srv, session)
	if first == second {
		t.Fatal("restart reused the secret")
	}
	if w := mfaPost(t, srv, session, "/api/auth/mfa/enroll/confirm", `{"password":"`+mfaTestPassword+`","code":"`+totpCode(t, first, time.Now())+`"}`, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("code for the replaced secret: got %d, want 401", w.Code)
	}
	if getUser(t, st, "jo").TOTPEnabled {
		t.Fatal("replaced secret enrolled")
	}
}
