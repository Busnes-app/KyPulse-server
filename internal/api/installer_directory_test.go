package api_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/syncauth"
	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

// KyIdentity's suiteWebhook signs with syncauth v1 and posts a SCIM user; both the suite
// route and the legacy path must apply it and revoke sessions on loss of availability.
func TestInstallerDirectorySyncAuthRevokesAndRestores(t *testing.T) {
	ctx := context.Background()
	_, st, cfg := setupTestServer(t)
	cfg.Server.AppURL = "https://pulse.example.test"
	dir := t.TempDir()
	clientSecret, directory := filepath.Join(dir, "client"), filepath.Join(dir, "directory")
	key := []byte("directory-fixture-secret-32bytes")
	for p, v := range map[string][]byte{clientSecret: []byte("client-fixture-secret"), directory: key} {
		if err := os.WriteFile(p, v, 0600); err != nil {
			t.Fatal(err)
		}
	}
	issuer := "https://identity.example.test"
	b := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530", SSO: &applysetup.SSO{IssuerURL: issuer, ClientID: "pulse", ClientSecretFile: clientSecret, DirectorySecretFile: directory, RedirectURI: cfg.Server.AppURL + "/api/sso/kysignon/callback"}, Admins: []applysetup.Identity{{Issuer: issuer, Subject: "owner-sub", Username: "owner-admin"}}, Everyday: []applysetup.Identity{{Issuer: issuer, Subject: "daily-sub", Username: "everyday"}}}
	if _, err := st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, false); err != nil {
		t.Fatal(err)
	}
	if err := config.ApplyInstallerConfiguration(cfg, b); err != nil {
		t.Fatal(err)
	}
	lg, err := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	srv := api.NewServer(cfg, st, lg, newTestMonitor(t, cfg, st, lg), newTestKyYard(t, cfg, st, lg, nil))

	login := func(sub string) func() int {
		user, err := st.Users().GetUserBySSO(ctx, "kysignon", sub)
		if err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		if _, _, err = auth.NewSessionManager(st, cfg.Security).IssueSession(ctx, rec, httptest.NewRequest("GET", "/", nil), user); err != nil {
			t.Fatal(err)
		}
		return func() int {
			r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			for _, c := range rec.Result().Cookies() {
				r.AddCookie(c)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			return w.Code
		}
	}
	deliver := func(path string, signKey []byte, event, sub string, rev int, active bool, roles string) int {
		body := []byte(fmt.Sprintf(`{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"id":%q,"active":%t,"roles":[%s],"meta":{"resourceType":"User","version":"W/\"%d\""}}`, sub, active, roles, rev))
		h, err := syncauth.Sign(signKey, time.Now(), event, fmt.Sprintf("evt-%s-%d", sub, rev), body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/scim+json")
		h.Apply(r)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w.Code
	}
	const suite, legacy = "/api/installer/directory", "/api/sso/kysignon/sync"

	daily := login("daily-sub")
	if code := daily(); code != 200 {
		t.Fatal("daily session", code)
	}
	if code := deliver(suite, []byte("wrong-key-wrong-key-wrong-key-32"), "user.updated", "daily-sub", 1, false, ""); code != 401 {
		t.Fatal("forged delivery", code)
	}
	if code := daily(); code != 200 {
		t.Fatal("forged delivery revoked a session", code)
	}
	// Assignment loss: active false.
	if code := deliver(suite, key, "user.updated", "daily-sub", 1, false, ""); code != 204 {
		t.Fatal("unassign", code)
	}
	if code := daily(); code != 401 {
		t.Fatal("unassigned identity kept its session", code)
	}
	if code := deliver(suite, key, "user.updated", "daily-sub", 1, false, ""); code != 204 {
		t.Fatal("retry of the applied revision", code)
	}
	if code := deliver(suite, key, "user.updated", "daily-sub", 1, true, ""); code != 422 {
		t.Fatal("conflicting body at an applied revision", code)
	}
	// Reassignment restores sign-in; a viewer asserted admin is not escalated.
	if code := deliver(suite, key, "user.updated", "daily-sub", 2, true, `{"value":"kypulse.admin"}`); code != 204 {
		t.Fatal("reassign", code)
	}
	if u, _ := st.Users().GetUserBySSO(ctx, "kysignon", "daily-sub"); u.Status != "active" || u.Role != "viewer" {
		t.Fatal("reassigned identity", u.Status, u.Role)
	}
	if code := deliver(suite, key, "user.updated", "daily-sub", 1, false, ""); code != 422 {
		t.Fatal("stale revision applied", code)
	}
	if code := deliver(suite, key, "user.mfa_reset", "daily-sub", 2, true, ""); code != 204 {
		t.Fatal("mfa reset at the current revision", code)
	}
	if u, _ := st.Users().GetUserBySSO(ctx, "kysignon", "daily-sub"); u.Status != "active" {
		t.Fatal("mfa reset changed availability", u.Status)
	}

	// The legacy path dispatches v1 signatures; an admin losing the admin role is unavailable.
	owner := login("owner-sub")
	if code := deliver(legacy, key, "user.updated", "owner-sub", 1, true, `{"value":"kypulse.admin"}`); code != 204 || owner() != 200 {
		t.Fatal("admin with admin role", code)
	}
	if code := deliver(legacy, key, "user.updated", "owner-sub", 2, true, ""); code != 204 {
		t.Fatal("admin role loss", code)
	}
	if code := owner(); code != 401 {
		t.Fatal("admin without the admin role kept its session", code)
	}
	if code := deliver(suite, key, "user.updated", "foreign-sub", 1, false, ""); code != 422 {
		t.Fatal("foreign identity", code)
	}
	if _, err := st.Audit().VerifyChain(ctx); err != nil {
		t.Fatal(err)
	}
}
