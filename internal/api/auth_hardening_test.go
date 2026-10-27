package api_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/password"
	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/coreos/go-oidc/v3/oidc"
)

func loginFrom(t *testing.T, srv *api.Server, addr, username, pass string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": username, "password": pass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = addr
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func createLocalUser(t *testing.T, st store.Store, id, username, status string) {
	t.Helper()
	hash, _ := password.Hash("SuperSecretPass123!")
	if err := st.Users().CreateUser(context.Background(), &store.User{
		ID: id, Username: username, PasswordHash: hash, Role: store.RoleViewer, Status: status, SSOProvider: "local",
	}); err != nil {
		t.Fatal(err)
	}
}

// Unknown accounts, wrong passwords and disabled accounts with a wrong password must be
// indistinguishable; only the correct password may learn an account is disabled.
func TestLoginDoesNotEnumerateAccounts(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	createLocalUser(t, st, "usr_live", "live", "active")
	createLocalUser(t, st, "usr_off", "off", "inactive")

	var bodies []string
	for i, c := range [][2]string{{"nobody", "SuperSecretPass123!"}, {"live", "WrongPassword123!"}, {"off", "WrongPassword123!"}} {
		w := loginFrom(t, srv, fmt.Sprintf("192.0.2.%d:1", i+1), c[0], c[1])
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d %s", c[0], w.Code, w.Body.String())
		}
		bodies = append(bodies, w.Body.String())
	}
	if bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("rejections differ: %q", bodies)
	}
	if w := loginFrom(t, srv, "192.0.2.9:1", "off", "SuperSecretPass123!"); w.Code != http.StatusForbidden {
		t.Fatalf("disabled with correct password: %d", w.Code)
	}

	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 50)
	reasons := map[string]string{}
	for _, row := range rows {
		if row.Action == "auth.login" {
			reasons[row.Details] = row.UserID + "@" + row.IPAddress
		}
	}
	for details, who := range map[string]string{
		"outcome=failure reason=unknown_user": "@192.0.2.1",
		"outcome=failure reason=password":     "usr_live@192.0.2.2",
		"outcome=failure reason=inactive":     "usr_off@192.0.2.9",
	} {
		if reasons[details] != who {
			t.Errorf("audit %q: got %q want %q (all %v)", details, reasons[details], who, reasons)
		}
	}
}

// A botnet must not outrun the per-IP window on one account, a full IP limiter must not
// reset the account window, and a locked account answers like any other rejection.
func TestLoginPerAccountWindow(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	createLocalUser(t, st, "usr_target", "target", "active")
	for i := 1; i <= 10; i++ {
		if w := loginFrom(t, srv, fmt.Sprintf("203.0.113.%d:1", i), "target", "WrongPassword123!"); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	for i := 0; i < api.AttemptsCapForTest+50; i++ {
		api.AllowAttemptForTest(srv, "filler:"+strconv.Itoa(i), 3, 5*time.Minute)
	}
	locked := loginFrom(t, srv, "198.51.100.50:1", "target", "SuperSecretPass123!")
	wrong := loginFrom(t, srv, "198.51.100.51:1", "nobody", "SuperSecretPass123!")
	if locked.Code != http.StatusUnauthorized || locked.Body.String() != wrong.Body.String() {
		t.Fatalf("locked account: %d %s, unknown: %s", locked.Code, locked.Body.String(), wrong.Body.String())
	}
	if w := loginFrom(t, srv, "198.51.100.52:1", "other", "x"); w.Code != http.StatusUnauthorized {
		t.Fatalf("other account affected: %d", w.Code)
	}
}

// The account window counts failures only: a user signing in often is never locked out.
func TestLoginAccountWindowIgnoresSuccess(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	createLocalUser(t, st, "usr_busy", "busy", "active")
	for i := 1; i <= 12; i++ {
		if w := loginFrom(t, srv, fmt.Sprintf("203.0.113.%d:1", i), "busy", "SuperSecretPass123!"); w.Code != http.StatusOK {
			t.Fatalf("sign-in %d: %d %s", i, w.Code, w.Body.String())
		}
	}
}

// One IPv6 host holds a whole /64; rotating addresses inside it must not buy fresh windows.
func TestLoginLimiterKeysIPv6ByPrefix(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	for i := 1; i <= 20; i++ {
		if w := loginFrom(t, srv, fmt.Sprintf("[2001:db8:aa:bb::%x]:1", i), "nobody", "x"); w.Code == http.StatusTooManyRequests {
			t.Fatalf("throttled at %d", i)
		}
	}
	if w := loginFrom(t, srv, "[2001:db8:aa:bb:1:2:3:4]:1", "nobody", "x"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("21st login from one /64: %d", w.Code)
	}
	if w := loginFrom(t, srv, "[2001:db8:aa:bc::1]:1", "nobody", "x"); w.Code == http.StatusTooManyRequests {
		t.Fatal("neighbouring /64 shares the window")
	}
}

// Login and MFA are CSRF-exempt, so a cross-site form posting text/plain must not reach them.
func TestCSRFExemptRoutesRequireJSON(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	createLocalUser(t, st, "usr_form", "form", "active")
	for _, path := range []string{"/api/auth/login", "/api/auth/mfa/totp", "/api/auth/mfa/recovery-code"} {
		for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x"} {
			req := httptest.NewRequest("POST", path, strings.NewReader(`{"username":"form","password":"SuperSecretPass123!"}`))
			if ct != "" {
				req.Header.Set("Content-Type", ct)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			if w.Code != http.StatusUnsupportedMediaType {
				t.Errorf("%s with %q: %d", path, ct, w.Code)
			}
		}
	}
	req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"form","password":"SuperSecretPass123!"}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("JSON with charset: %d %s", w.Code, w.Body.String())
	}
}

// A flood of refused logins from one client leaves a bounded trail.
func TestFailedLoginAuditIsBounded(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	for i := 0; i < 30; i++ {
		loginFrom(t, srv, "192.0.2.77:1", "nobody", "x")
	}
	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 100)
	got := map[string]int{}
	for _, row := range rows {
		got[row.Details]++
	}
	if len(rows) != 2 || got["outcome=failure reason=unknown_user"] != 1 || got["outcome=failure reason=rate_limit"] != 1 {
		t.Fatalf("30 refused logins wrote %d rows: %v", len(rows), got)
	}
}

// The ingest route throttles each client before the token lookup and never writes one audit
// row per refused request.
func TestIngestLimitsClientsBeforeAuthentication(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	last := 0
	for i := 0; i < 601; i++ {
		r := httptest.NewRequest("POST", "/api/ingest/logs", strings.NewReader("{}\n"))
		r.Header.Set("Authorization", "Bearer not-a-token")
		r.Header.Set("Content-Type", "application/x-ndjson")
		r.RemoteAddr = "192.0.2.55:1"
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		last = w.Code
		if i < 600 && w.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: %d", i+1, w.Code)
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("601st request: %d", last)
	}
	_, total, _ := st.Audit().ListAuditRecords(context.Background(), 0, 1)
	if total != 2 {
		t.Fatalf("601 refused ingest requests wrote %d audit rows, want 2", total)
	}
}

// fakeIdP is a minimal OpenID provider: discovery, JWKS and a token endpoint that either
// fails with a body or returns an RS256 ID token for sub (and sid when set).
type fakeIdP struct {
	*httptest.Server
	key  *rsa.PrivateKey
	sub  string
	sid  string
	fail string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{key: key}
	b64 := base64.RawURLEncoding.EncodeToString
	idp.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issuer := idp.URL
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
				"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig",
				"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes())}}})
		case "/token":
			if idp.fail != "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"` + idp.fail + `"}`))
				return
			}
			header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": "JWT"})
			claims, _ := json.Marshal(map[string]any{"iss": issuer, "aud": "pulse", "sub": idp.sub, "sid": idp.sid, "nonce": "n1",
				"preferred_username": idp.sub, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix()})
			input := b64(header) + "." + b64(claims)
			digest := sha256.Sum256([]byte(input))
			sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 60, "id_token": input + "." + b64(sig)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(idp.Close)
	return idp
}

func ssoCallback(srv *api.Server, client *http.Client) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/api/sso/kysignon/callback?code=c&state=s1", nil)
	req = req.WithContext(oidc.ClientContext(req.Context(), client))
	req.AddCookie(&http.Cookie{Name: "ky_pkce_s1", Value: "verifier-verifier-verifier-verifier-verifier"})
	req.AddCookie(&http.Cookie{Name: "ky_nonce_s1", Value: "n1"})
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func hasSession(w *httptest.ResponseRecorder) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.Value != "" {
			return true
		}
	}
	return false
}

type ssoLookupFails struct{ store.UserStore }

func (ssoLookupFails) GetUserBySSO(context.Context, string, string) (*store.User, error) {
	return nil, errors.New("database unavailable")
}

type ssoBrokenStore struct{ store.Store }

func (s ssoBrokenStore) Users() store.UserStore { return ssoLookupFails{s.Store.Users()} }

func TestKySignOnCallbackHardening(t *testing.T) {
	idp := newFakeIdP(t)
	t.Setenv("KYPULSE_KYSIGNON_ISSUER", idp.URL)
	t.Setenv("KYPULSE_KYSIGNON_CLIENT_ID", "pulse")
	t.Setenv("KYPULSE_SSO_AUTO_PROVISION", "true")
	srv, st, _ := setupTestServer(t)
	ctx := context.Background()

	// The IdP's error body stays in the server log, and the verifier and nonce are spent.
	idp.fail = "secret-idp-marker"
	w := ssoCallback(srv, idp.Client())
	if w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "secret-idp-marker") {
		t.Fatalf("exchange failure: %d %s", w.Code, w.Body.String())
	}
	expired := map[string]bool{}
	for _, c := range w.Result().Cookies() {
		if c.MaxAge < 0 && c.Path == "/api/sso/kysignon/callback" {
			expired[c.Name] = true
		}
	}
	if !expired["ky_pkce_s1"] || !expired["ky_nonce_s1"] {
		t.Fatalf("PKCE/nonce cookies not expired: %v", w.Result().Cookies())
	}
	idp.fail = ""

	// An existing inactive account gets no session.
	if err := st.Users().CreateUser(ctx, &store.User{ID: "usr_sso_off", Username: "sso-off", Role: store.RoleViewer, Status: "inactive", SSOProvider: "kysignon", SSOSubject: "sub-off"}); err != nil {
		t.Fatal(err)
	}
	idp.sub = "sub-off"
	if w := ssoCallback(srv, idp.Client()); w.Code != http.StatusForbidden || hasSession(w) {
		t.Fatalf("inactive SSO user: %d session=%v", w.Code, hasSession(w))
	}

	// A lookup failure is not "unknown": nothing is provisioned.
	api.SetStoreForTest(srv, ssoBrokenStore{st})
	idp.sub = "sub-new"
	if w := ssoCallback(srv, idp.Client()); w.Code != http.StatusInternalServerError || hasSession(w) {
		t.Fatalf("lookup failure: %d", w.Code)
	}
	if _, err := st.Users().GetUserBySSO(ctx, "kysignon", "sub-new"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("lookup failure provisioned a user: %v", err)
	}

	// Only a genuinely unknown subject is provisioned.
	api.SetStoreForTest(srv, st)
	if w := ssoCallback(srv, idp.Client()); w.Code != http.StatusFound || !hasSession(w) {
		t.Fatalf("auto-provision: %d %s", w.Code, w.Body.String())
	}
}
