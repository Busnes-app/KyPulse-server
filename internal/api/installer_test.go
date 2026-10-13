package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/sso"
)

func TestInstallerLiveActivationRolesAndSignedRevocation(t *testing.T) {
	ctx := context.Background()
	_, st, cfg := setupTestServer(t)
	cfg.Server.AppURL = "https://pulse.example.test"
	dir := t.TempDir()
	secret, directory := filepath.Join(dir, "client"), filepath.Join(dir, "directory")
	for _, p := range []string{secret, directory} {
		if err := os.WriteFile(p, []byte("private-fixture-secret"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	b := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530", SSO: &applysetup.SSO{IssuerURL: "https://identity.example.test", ClientID: "pulse", ClientSecretFile: secret, DirectorySecretFile: directory, RedirectURI: cfg.Server.AppURL + "/api/sso/kysignon/callback"}, Admins: []applysetup.Identity{{Issuer: "https://identity.example.test", Subject: "owner-sub", Username: "owner-admin"}}, Everyday: []applysetup.Identity{{Issuer: "https://identity.example.test", Subject: "daily-sub", Username: "everyday"}}}
	raw, _ := json.Marshal(b)
	lg, err := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	makeServer := func() *api.Server {
		return api.NewServer(cfg, st, lg, newTestMonitor(t, cfg, st, lg), newTestKyYard(t, cfg, st, lg, nil))
	}
	srv := makeServer()
	socket, err := config.InstallerSocket(cfg.Database.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	closeSetup, err := applysetup.StartLocal(ctx, socket, srv.ApplyInstaller, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := applysetup.CallLocal(ctx, socket, raw, false)
	if err != nil || len(report.Created) != 3 || len(report.Pending) != 1 {
		t.Fatal(report, err)
	}
	if cfg.SSO.KySignOnIssuer != "" {
		t.Fatal("mutable runtime snapshot")
	}
	if err = closeSetup(); err != nil {
		t.Fatal(err)
	}
	if err = config.ApplyInstallerConfiguration(cfg, b); err != nil {
		t.Fatal(err)
	}
	srv = makeServer()
	closeSetup, err = applysetup.StartLocal(ctx, socket, srv.ApplyInstaller, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSetup()
	report, err = applysetup.CallLocal(ctx, socket, raw, true)
	if err != nil || len(report.Present) != 3 || len(report.Pending) != 0 {
		t.Fatal(report, err)
	}
	for _, sub := range []string{"owner-sub", "daily-sub"} {
		user, err := st.Users().GetUserBySSO(ctx, "kysignon", sub)
		if err != nil {
			t.Fatal(err)
		}
		login := httptest.NewRecorder()
		if _, _, err = auth.NewSessionManager(st, cfg.Security).IssueSession(ctx, login, httptest.NewRequest("GET", "/", nil), user); err != nil {
			t.Fatal(err)
		}
		request := func() *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodGet, "/api/backup/status", nil)
			for _, c := range login.Result().Cookies() {
				r.AddCookie(c)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			return w
		}
		expected := 200
		if sub == "daily-sub" {
			expected = 403
		}
		if w := request(); w.Code != expected {
			t.Fatal(sub, w.Code, w.Body.String())
		}
		payload := sso.KySignOnSyncPayload{Event: "user.deactivated", ID: sub, Timestamp: time.Now().Unix()}
		body, _ := json.Marshal(payload)
		client := sso.NewKySignOnClient(cfg.SSO, st)
		if err = client.HandleSyncWebhook(ctx, body, crypto.ComputeHMACSHA256(body, cfg.SSO.KySignOnHMACSecret)); err != nil {
			t.Fatal(err)
		}
		if w := request(); w.Code != 401 {
			t.Fatal("inactive identity retained session", sub, w.Code)
		}
	}
}
