package api_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/coreos/go-oidc/v3/oidc"
)

// jwt signs claims for client "pulse" with the provider's key under typ.
func (idp *fakeIdP) jwt(t *testing.T, typ string, claims map[string]any) string {
	t.Helper()
	claims["iss"], claims["aud"] = idp.URL, "pulse"
	claims["iat"], claims["exp"] = time.Now().Unix(), time.Now().Add(2*time.Minute).Unix()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "k1", "typ": typ})
	c, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(c)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, idp.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (idp *fakeIdP) logoutToken(t *testing.T, sub, sid string) string {
	claims := map[string]any{"sub": sub, "jti": sid + sub, "events": map[string]any{"http://schemas.openid.net/event/backchannel-logout": map[string]any{}}}
	if sid != "" {
		claims["sid"] = sid
	}
	return idp.jwt(t, "logout+jwt", claims)
}

func ssoLogin(srv *api.Server, idp *fakeIdP, sub, sid string) *httptest.ResponseRecorder {
	idp.sub, idp.sid = sub, sid
	return ssoCallback(srv, idp.Client())
}

func signedIn(srv *api.Server, c *http.Cookie) bool {
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(c)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var me struct{ Authenticated bool }
	return json.Unmarshal(w.Body.Bytes(), &me) == nil && me.Authenticated
}

func postLogout(srv *api.Server, idp *fakeIdP, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/sso/kysignon/backchannel-logout", strings.NewReader(url.Values{"logout_token": {token}}.Encode()))
	req = req.WithContext(oidc.ClientContext(req.Context(), idp.Client()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestBackchannelLogoutEndsTheSSOLogin(t *testing.T) {
	idp := newFakeIdP(t)
	t.Setenv("KYPULSE_KYSIGNON_ISSUER", idp.URL)
	t.Setenv("KYPULSE_KYSIGNON_CLIENT_ID", "pulse")
	t.Setenv("KYPULSE_SSO_AUTO_PROVISION", "true")
	srv, st, _ := setupTestServer(t)

	ended := sessionCookie(t, ssoLogin(srv, idp, "alice", "sid-a1"))
	kept := sessionCookie(t, ssoLogin(srv, idp, "alice", "sid-a2"))
	if !signedIn(srv, ended) || !signedIn(srv, kept) {
		t.Fatal("SSO login did not issue working sessions")
	}

	// An ID token is not a logout token, even when signed by the provider.
	idToken := idp.jwt(t, "JWT", map[string]any{"sub": "alice", "sid": "sid-a2", "nonce": "n1"})
	if w := postLogout(srv, idp, idToken); w.Code != http.StatusBadRequest || !signedIn(srv, kept) {
		t.Fatalf("ID token as logout token: got %d", w.Code)
	}
	w := postLogout(srv, idp, idp.logoutToken(t, "alice", "sid-a1"))
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("logout: got %d %q %s", w.Code, w.Header().Get("Cache-Control"), w.Body.String())
	}
	if signedIn(srv, ended) {
		t.Fatal("session survived its back-channel logout")
	}
	if !signedIn(srv, kept) {
		t.Fatal("logout of one login ended another login of the same user")
	}
	// A replay is harmless and still acknowledged.
	if w := postLogout(srv, idp, idp.logoutToken(t, "alice", "sid-a1")); w.Code != http.StatusOK {
		t.Fatalf("replayed logout: got %d", w.Code)
	}

	// A logout that lands before the callback finishes fences that login.
	if w := postLogout(srv, idp, idp.logoutToken(t, "alice", "sid-late")); w.Code != http.StatusOK {
		t.Fatalf("early logout: got %d", w.Code)
	}
	if w := ssoLogin(srv, idp, "alice", "sid-late"); w.Code != http.StatusUnauthorized || hasSession(w) {
		t.Fatalf("callback revived an ended login: %d", w.Code)
	}

	// A subject-wide token ends the user's logins issued by then, and nobody else's.
	bob := sessionCookie(t, ssoLogin(srv, idp, "bob", "sid-b1"))
	if w := postLogout(srv, idp, idp.logoutToken(t, "alice", "")); w.Code != http.StatusOK {
		t.Fatalf("subject-wide logout: got %d", w.Code)
	}
	if signedIn(srv, kept) || !signedIn(srv, bob) {
		t.Fatal("subject-wide logout ended the wrong sessions")
	}
	if w := postLogout(srv, idp, idp.logoutToken(t, "nobody", "")); w.Code != http.StatusOK {
		t.Fatalf("logout for an unknown subject: got %d", w.Code)
	}

	recs, _, err := st.Audit().ListAuditRecords(context.Background(), 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	audits := 0
	for _, r := range recs {
		if r.Action == "auth.sso_backchannel_logout" {
			audits++
		}
	}
	// sid-a1, its replay, sid-late and alice's subject-wide logout; not the unknown subject.
	if audits != 4 {
		t.Fatalf("back-channel logout audit rows: got %d, want 4", audits)
	}
}
