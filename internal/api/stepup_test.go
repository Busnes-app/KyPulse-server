package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/coreos/go-oidc/v3/oidc"
)

func postPairing(srv *api.Server, session *http.Cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/log-sources/pairing", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(session)
	req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "test-csrf"})
	req.Header.Set(auth.HeaderCSRF, "test-csrf")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func stepUpCode(w *httptest.ResponseRecorder) (code, method string) {
	var body struct{ Code, Method string }
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Code, body.Method
}

// Q2 A: a local admin creates a pairing code only with the current password.
func TestPairingRequiresTheCurrentPassword(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")
	for _, body := range []string{`{}`, `{"password":"wrong-password-123"}`} {
		w := postPairing(srv, admin, body)
		if code, method := stepUpCode(w); w.Code != http.StatusForbidden || code != "step_up_required" || method != "password" {
			t.Fatalf("%s: got %d %s", body, w.Code, w.Body.String())
		}
	}
	if w := postPairing(srv, admin, `{"password":"SuperSecretPass123!"}`); w.Code != http.StatusOK {
		t.Fatalf("with the password: got %d %s", w.Code, w.Body.String())
	}
	// Wrong passwords run out; a stolen session cannot keep guessing.
	for range 5 {
		postPairing(srv, admin, `{"password":"wrong-password-123"}`)
	}
	if w := postPairing(srv, admin, `{"password":"SuperSecretPass123!"}`); w.Code != http.StatusForbidden {
		t.Fatalf("after five wrong passwords: got %d", w.Code)
	}
}

// stepUpLogin runs /api/sso/kysignon/login?step_up=1 and its callback for sub.
func stepUpLogin(t *testing.T, srv *api.Server, idp *fakeIdP, sub string, tamper func(*http.Cookie)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/sso/kysignon/login?step_up=1", nil)
	req = req.WithContext(oidc.ClientContext(req.Context(), idp.Client()))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	loc, err := url.Parse(w.Header().Get("Location"))
	if w.Code != http.StatusFound || err != nil {
		t.Fatalf("step-up login: %d", w.Code)
	}
	if q := loc.Query(); q.Get("max_age") != "0" || q.Get("prompt") != "login" {
		t.Fatalf("step-up does not ask KyIdentity to check the credentials: %s", loc)
	}
	state := loc.Query().Get("state")
	cb := httptest.NewRequest("GET", "/api/sso/kysignon/callback?code=c&state="+state, nil)
	cb = cb.WithContext(oidc.ClientContext(cb.Context(), idp.Client()))
	cb.AddCookie(&http.Cookie{Name: "ky_pkce_" + state, Value: "verifier-verifier-verifier-verifier-verifier"})
	cb.AddCookie(&http.Cookie{Name: "ky_nonce_" + state, Value: "n1"})
	for _, c := range w.Result().Cookies() {
		if c.Name == "ky_stepup_"+state {
			if tamper != nil {
				tamper(c)
			}
			cb.AddCookie(c)
		}
	}
	idp.sub, idp.sid = sub, "sid-"+sub
	out := httptest.NewRecorder()
	srv.ServeHTTP(out, cb)
	return out
}

// Q2 A with DECISIONS "SSO step-up": an SSO admin needs a fresh KyIdentity sign-in
// (max_age=0) dated by auth_time; a silent sign-in or an edited start time does not count.
func TestPairingRequiresAKyIdentityStepUp(t *testing.T) {
	idp := newFakeIdP(t)
	t.Setenv("KYPULSE_KYSIGNON_ISSUER", idp.URL)
	t.Setenv("KYPULSE_KYSIGNON_CLIENT_ID", "pulse")
	t.Setenv("KYPULSE_SSO_AUTO_PROVISION", "true")
	srv, _, _ := setupTestServer(t)
	idp.role = "admin"

	plain := sessionCookie(t, ssoLogin(srv, idp, "ann", "sid-plain"))
	w := postPairing(srv, plain, `{}`)
	if code, method := stepUpCode(w); w.Code != http.StatusForbidden || code != "step_up_required" || method != "kyidentity" {
		t.Fatalf("plain SSO session: got %d %s", w.Code, w.Body.String())
	}

	// KyIdentity reused an old sign-in: no fresh auth_time, no step-up.
	idp.auth = time.Now().Add(-time.Hour)
	if w := stepUpLogin(t, srv, idp, "ann", nil); w.Code != http.StatusUnauthorized || hasSession(w) {
		t.Fatalf("stale auth_time accepted: %d", w.Code)
	}
	// A start time the browser moved back is refused by its MAC.
	idp.auth = time.Now().Add(-30 * time.Minute)
	backdate := func(c *http.Cookie) {
		_, mac, _ := strings.Cut(c.Value, ".")
		c.Value = "1." + mac
	}
	if w := stepUpLogin(t, srv, idp, "ann", backdate); w.Code != http.StatusUnauthorized || hasSession(w) {
		t.Fatalf("edited step-up start accepted: %d", w.Code)
	}

	idp.auth = time.Now()
	w = stepUpLogin(t, srv, idp, "ann", nil)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/#/logs" {
		t.Fatalf("fresh step-up: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := postPairing(srv, sessionCookie(t, w), `{}`); w.Code != http.StatusOK {
		t.Fatalf("after a step-up: got %d %s", w.Code, w.Body.String())
	}
}
