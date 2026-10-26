package sso_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/sso"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
	"github.com/coreos/go-oidc/v3/oidc"
)

func TestOAuthAuthorizationURLUsesDiscoveryAndPKCE(t *testing.T) {
	var issuer string
	idp := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": issuer, "authorization_endpoint": issuer + "/authorize",
			"token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys",
		})
	}))
	defer idp.Close()
	issuer = idp.URL

	client := sso.NewKySignOnClient(config.SSOConfig{KySignOnIssuer: issuer, KySignOnClientID: "client"}, nil)
	authURL, err := client.BuildAuthURL(oidc.ClientContext(context.Background(), idp.Client()), "https://app.example/callback", "state", "verifier", "nonce")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/authorize" || query.Get("state") != "state" || query.Get("nonce") != "nonce" || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("unexpected authorization URL: %s", authURL)
	}
}

func TestKySignOnWebhookSync(t *testing.T) {
	st, err := store.Open(context.Background(), testdb.Config(t))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	hmacSecret := "webhook-secret-999"
	client := sso.NewKySignOnClient(config.SSOConfig{
		KySignOnHMACSecret: hmacSecret,
	}, st)

	payload := sso.KySignOnSyncPayload{
		Event:       "user.created",
		ID:          "ext-usr-456",
		Username:    "bob",
		Email:       "bob@busnes.app",
		DisplayName: "Bob Engineer",
		Role:        "viewer",
		Status:      "active",
		Timestamp:   time.Now().Unix(),
	}
	body, _ := json.Marshal(payload)
	sig := crypto.ComputeHMACSHA256(body, hmacSecret)

	// 1. Sync create user
	if err := client.HandleSyncWebhook(context.Background(), body, sig); err != nil {
		t.Fatalf("HandleSyncWebhook failed: %v", err)
	}

	created, err := st.Users().GetUserBySSO(context.Background(), "kysignon", "ext-usr-456")
	if err != nil {
		t.Fatalf("GetUserBySSO failed: %v", err)
	}
	if created.Username != "bob" || created.DisplayName != "Bob Engineer" {
		t.Errorf("unexpected created user: %+v", created)
	}

	// 2. Sync deactivation
	payload.Event = "user.deactivated"
	body, _ = json.Marshal(payload)
	sig = crypto.ComputeHMACSHA256(body, hmacSecret)

	if err := client.HandleSyncWebhook(context.Background(), body, sig); err != nil {
		t.Fatalf("HandleSyncWebhook deactivation failed: %v", err)
	}

	updated, _ := st.Users().GetUserBySSO(context.Background(), "kysignon", "ext-usr-456")
	if updated.Status != "inactive" {
		t.Errorf("expected inactive status, got %s", updated.Status)
	}
}

// A captured delivery replayed inside the timestamp window must not reapply: replaying the
// earlier "created, active" body would reactivate the account just deactivated.
func TestKySignOnWebhookRefusesReplay(t *testing.T) {
	st, err := store.Open(context.Background(), testdb.Config(t))
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()
	const secret = "webhook-secret-999"
	client := sso.NewKySignOnClient(config.SSOConfig{KySignOnHMACSecret: secret}, st)
	send := func(p sso.KySignOnSyncPayload) error {
		body, _ := json.Marshal(p)
		return client.HandleSyncWebhook(context.Background(), body, crypto.ComputeHMACSHA256(body, secret))
	}
	created := sso.KySignOnSyncPayload{Event: "user.created", ID: "ext-replay", Username: "eve", Role: "viewer", Status: "active", Timestamp: time.Now().Unix()}
	if err := send(created); err != nil {
		t.Fatal(err)
	}
	deactivated := created
	deactivated.Event = "user.deactivated"
	if err := send(deactivated); err != nil {
		t.Fatal(err)
	}
	if err := send(created); err == nil {
		t.Fatal("replayed delivery accepted")
	}
	if u, _ := st.Users().GetUserBySSO(context.Background(), "kysignon", "ext-replay"); u.Status != "inactive" {
		t.Fatalf("replay reactivated the account: %s", u.Status)
	}
	// A failed delivery is not recorded, so the provider's retry still applies.
	broken := sso.NewKySignOnClient(config.SSOConfig{KySignOnHMACSecret: secret}, st)
	st.Close()
	retry := created
	retry.ID, retry.Timestamp = "ext-retry", time.Now().Unix()
	body, _ := json.Marshal(retry)
	sig := crypto.ComputeHMACSHA256(body, secret)
	if err := broken.HandleSyncWebhook(context.Background(), body, sig); err == nil {
		t.Fatal("closed store accepted a delivery")
	}
	if err := broken.HandleSyncWebhook(context.Background(), body, sig); err == nil || err.Error() == "webhook already delivered" {
		t.Fatalf("failed delivery was recorded as delivered: %v", err)
	}
}

func TestSAMLServiceProvider(t *testing.T) {
	sp := sso.NewSAMLServiceProvider("https://app.busnes.app/saml/metadata", "https://app.busnes.app/saml/acs")

	metadata := sp.GenerateMetadata()
	if len(metadata) == 0 || !testing.Verbose() && len(metadata) < 50 {
		if len(metadata) == 0 {
			t.Errorf("expected non-empty metadata")
		}
	}

}

func TestRoleForMapsUnknownRolesToViewer(t *testing.T) {
	cases := map[string]string{
		"admin":   "admin",
		"viewer":  "viewer",
		"user":    "viewer",
		"manager": "viewer",
		"":        "viewer",
		"Admin":   "viewer",
	}
	for claimed, want := range cases {
		if got := sso.RoleFor(claimed); got != want {
			t.Errorf("RoleFor(%q) = %q, want %q", claimed, got, want)
		}
	}
}
