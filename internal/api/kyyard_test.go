package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// fakeYard answers by URL suffix; copied from internal/kyyard's own test fixture. token is
// exposed so a test can assert an audit row or a response never carries it.
type fakeYard struct {
	answers map[string]struct {
		code int
		body any
	}
	token string
	mu    sync.Mutex
}

func (f *fakeYard) GetWith(_ context.Context, rawURL string, _ map[string]string) (*egress.Response, error) {
	return f.answer(rawURL)
}

func (f *fakeYard) Post(_ context.Context, rawURL, _ string, _ []byte, _ map[string]string) (*egress.Response, error) {
	return f.answer(rawURL)
}

func (f *fakeYard) answer(rawURL string) (*egress.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for suffix, a := range f.answers {
		if len(rawURL) >= len(suffix) && rawURL[len(rawURL)-len(suffix):] == suffix {
			b, _ := json.Marshal(a.body)
			return &egress.Response{StatusCode: a.code, Body: b}, nil
		}
	}
	return &egress.Response{StatusCode: 404, Body: []byte(`{"error":"no"}`)}, nil
}

// pairedYard is a fakeYard that claims successfully and answers the same endpoint, inventory
// and samples fixture internal/kyyard's own tests use: one docker endpoint host-1, one
// container kyvault, healthy, no exit code.
func pairedYard() *fakeYard {
	return &fakeYard{
		token: "kyyard-test-token",
		answers: map[string]struct {
			code int
			body any
		}{
			"/api/service-tokens/claim":                    {200, map[string]any{"token": "kyyard-test-token", "organization": map[string]string{"id": "org_a", "name": "A"}}},
			"/api/organizations/org_a/endpoints?limit=200": {200, []map[string]any{{"id": "ep_1", "name": "host-1", "runtime": "docker", "state": "active"}}},
			"/api/organizations/org_a/endpoints/ep_1/inventory": {200, map[string]any{"endpoint_id": "ep_1", "state": "complete", "observed_at": "2026-09-27T10:00:00Z", "received_at": "2026-09-27T10:00:01Z",
				"snapshot": map[string]any{"containers": []map[string]any{{"id": "c1", "name": "kyvault", "image": "ghcr.io/busnes-app/kyvault:1.2", "state": "running", "status": "Up 3 hours (healthy)"}}}}},
			"/api/organizations/org_a/endpoints/ep_1/samples": {200, []map[string]any{{"container_id": "c1", "observed_at": "2026-09-27T10:00:00Z", "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 2}}},
		},
	}
}

// monitorFixture is the unpaired server: every KyYard read is a 404.
func monitorFixture(t *testing.T) (*api.Server, store.Store, *http.Cookie, *http.Cookie) {
	t.Helper()
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")
	viewer := loginAs(t, srv, st, "bob", "viewer")
	return srv, st, admin, viewer
}

// monitorFixtureWithYard builds the server with a fake KyYard that answers a claim and the
// usual endpoint/inventory/samples fixture.
func monitorFixtureWithYard(t *testing.T) (*api.Server, store.Store, *http.Cookie, *http.Cookie, *fakeYard) {
	t.Helper()
	fake := pairedYard()
	srv, st, _ := setupTestServerWith(t, nil, fake)
	admin := loginAs(t, srv, st, "alice", "admin")
	viewer := loginAs(t, srv, st, "bob", "viewer")
	return srv, st, admin, viewer, fake
}

func TestKyYardStatusUnpairedAndPairRefusals(t *testing.T) {
	srv, st, admin, viewer := monitorFixture(t)
	_ = st
	s := decodeMap(t, do(t, srv, "GET", "/api/kyyard", viewer))
	if s["paired"] != false {
		t.Fatalf("unpaired status: %v", s)
	}
	if w := do(t, srv, "GET", "/api/kyyard/containers", viewer); w.Code != http.StatusForbidden {
		t.Fatalf("suggestions are admin-only: %d", w.Code)
	}
	if w := doJSON(t, srv, "POST", "/api/kyyard/pair", viewer, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"}); w.Code != http.StatusForbidden {
		t.Fatalf("pair is admin-only: %d", w.Code)
	}
	for _, u := range []string{"http://127.0.0.1:8080", "http://169.254.169.254", "https://localhost", "ftp://yard.lan", ""} {
		if w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": u, "pairing_code": "123456"}); w.Code != http.StatusBadRequest {
			t.Fatalf("%q must be refused with 400, got %d %s", u, w.Code, w.Body)
		}
	}
	if w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "http://yard.lan", "pairing_code": "123456"}); w.Code != http.StatusBadRequest {
		t.Fatalf("plain http without KYPULSE_KYYARD_ALLOW_HTTP must be 400, got %d", w.Code)
	}
	if w := do(t, srv, "DELETE", "/api/kyyard", admin); w.Code != http.StatusNoContent {
		t.Fatalf("unpair when unpaired is idempotent: %d", w.Code)
	}
}

func TestKyYardPairUnpairAndFacts(t *testing.T) {
	srv, st, admin, viewer, fake := monitorFixtureWithYard(t)
	w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	if w.Code != http.StatusOK {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	s := decodeMap(t, w)
	if s["paired"] != true || s["organization"] != "A" || s["stale"] != false {
		t.Fatalf("paired status: %v", s)
	}
	if _, has := s["token"]; has {
		t.Fatal("token in response")
	}
	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
	var paired bool
	for _, r := range rows {
		if r.Action == "admin.kyyard_pair" {
			paired = true
			if strings.Contains(r.Details, "123456") || strings.Contains(r.Details, fake.token) {
				t.Fatalf("audit leaks a secret: %q", r.Details)
			}
			// auditValue quotes every field, matching admin.webhook_set's convention.
			if !strings.Contains(r.Details, `host="yard.lan"`) || !strings.Contains(r.Details, `org="org_a"`) {
				t.Fatalf("audit details: %q", r.Details)
			}
		}
	}
	if !paired {
		t.Fatal("no pairing audit row")
	}
	// Settings never expose the sealed row.
	if body := do(t, srv, "GET", "/api/settings", admin).Body.String(); strings.Contains(body, "kyyard_enc") || strings.Contains(body, fake.token) {
		t.Fatal("settings leak the pairing")
	}
	// Suggestions and status for the bar.
	sug := do(t, srv, "GET", "/api/kyyard/containers", admin)
	if sug.Code != http.StatusOK || !strings.Contains(sug.Body.String(), `"link":"ep_1/kyvault"`) {
		t.Fatalf("suggestions: %d %s", sug.Code, sug.Body)
	}
	st1 := decodeMap(t, do(t, srv, "GET", "/api/status", viewer))
	if ky, _ := st1["kyyard"].(map[string]any); ky == nil || ky["paired"] != true || ky["stale"] != false {
		t.Fatalf("status kyyard: %v", st1["kyyard"])
	}
	// A target linked to a container carries facts.
	c := doJSON(t, srv, "POST", "/api/targets", admin, map[string]any{"name": "KyVault", "url": "http://vault.lan/healthz", "container": "ep_1/kyvault"})
	id := decodeMap(t, c)["target"].(map[string]any)["id"].(string)
	d := decodeMap(t, do(t, srv, "GET", "/api/targets/"+id, viewer))
	facts, _ := d["kyyard"].(map[string]any)
	if facts == nil || facts["health"] != "healthy" || facts["endpoint_name"] != "host-1" {
		t.Fatalf("facts: %v", d["kyyard"])
	}
	// Unpair: audited, status unpaired, facts gone, KyYard-side step named.
	if w := do(t, srv, "DELETE", "/api/kyyard", admin); w.Code != http.StatusNoContent {
		t.Fatalf("unpair: %d", w.Code)
	}
	if s := decodeMap(t, do(t, srv, "GET", "/api/kyyard", admin)); s["paired"] != false {
		t.Fatalf("after unpair: %v", s)
	}
	if d := decodeMap(t, do(t, srv, "GET", "/api/targets/"+id, viewer)); d["kyyard"] != nil {
		t.Fatalf("facts after unpair: %v", d["kyyard"])
	}
}
