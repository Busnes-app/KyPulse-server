package kyyard_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
)

// fakeHTTP answers by path and records requests.
type fakeHTTP struct {
	answers map[string]struct {
		code int
		body any
	}
	err   error
	mu    sync.Mutex
	posts []struct {
		url     string
		body    []byte
		headers map[string]string
	}
	gets []struct {
		url     string
		headers map[string]string
	}
}

func (f *fakeHTTP) GetWith(_ context.Context, rawURL string, headers map[string]string) (*egress.Response, error) {
	f.mu.Lock()
	f.gets = append(f.gets, struct {
		url     string
		headers map[string]string
	}{rawURL, headers})
	f.mu.Unlock()
	return f.answer(rawURL)
}

// getCount answers how many GETs have landed so far; safe for concurrent use.
func (f *fakeHTTP) getCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.gets)
}
func (f *fakeHTTP) Post(_ context.Context, rawURL, _ string, body []byte, headers map[string]string) (*egress.Response, error) {
	f.mu.Lock()
	f.posts = append(f.posts, struct {
		url     string
		body    []byte
		headers map[string]string
	}{rawURL, body, headers})
	f.mu.Unlock()
	return f.answer(rawURL)
}
func (f *fakeHTTP) answer(rawURL string) (*egress.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	for suffix, a := range f.answers {
		if len(rawURL) >= len(suffix) && rawURL[len(rawURL)-len(suffix):] == suffix {
			b, _ := json.Marshal(a.body)
			return &egress.Response{StatusCode: a.code, Body: b}, nil
		}
	}
	return &egress.Response{StatusCode: 404, Body: []byte(`{"error":"no"}`)}, nil
}

func answers(m map[string]struct {
	code int
	body any
}) *fakeHTTP {
	return &fakeHTTP{answers: m}
}

func TestClaimPostsServiceNameAndSealsNothingInErrors(t *testing.T) {
	h := answers(map[string]struct {
		code int
		body any
	}{
		"/api/service-tokens/claim": {200, map[string]any{"token": "ab12", "organization": map[string]string{"id": "org_a", "name": "A"}}},
	})
	cfg, err := kyyard.Claim(context.Background(), h, "https://yard.lan/", "123456")
	if err != nil || cfg.Token != "ab12" || cfg.OrganizationID != "org_a" || cfg.OrganizationName != "A" || cfg.URL != "https://yard.lan" {
		t.Fatalf("claim: %+v %v", cfg, err)
	}
	if len(h.posts) != 1 || h.posts[0].url != "https://yard.lan/api/service-tokens/claim" || string(h.posts[0].body) != `{"pairing_code":"123456","service_name":"kypulse"}` {
		t.Fatalf("post: %+v", h.posts)
	}
	h = answers(map[string]struct {
		code int
		body any
	}{"/api/service-tokens/claim": {403, map[string]string{"error": "Pairing refused"}}})
	if _, err := kyyard.Claim(context.Background(), h, "https://yard.lan", "000000"); !errors.Is(err, kyyard.ErrPairingRefused) {
		t.Fatalf("refused: %v", err)
	}
	h = answers(map[string]struct {
		code int
		body any
	}{"/api/service-tokens/claim": {429, map[string]string{"error": "slow down"}}})
	if _, err := kyyard.Claim(context.Background(), h, "https://yard.lan", "000000"); !errors.Is(err, kyyard.ErrRateLimited) {
		t.Fatalf("limited: %v", err)
	}
	if _, err := kyyard.Claim(context.Background(), h, "https://yard.lan", "12345"); err == nil {
		t.Fatal("a code that is not six digits must be refused before any request")
	}
}

func TestClientReadsWithBearerAndClassifiesErrors(t *testing.T) {
	h := answers(map[string]struct {
		code int
		body any
	}{
		"/api/organizations/org_a/endpoints?limit=200": {200, []map[string]any{{"id": "ep_1", "name": "host-1", "runtime": "docker", "state": "active"}, {"id": "ep_2", "name": "k8s", "runtime": "kubernetes", "state": "active"}}},
		"/api/organizations/org_a/endpoints/ep_1/inventory": {200, map[string]any{"endpoint_id": "ep_1", "state": "complete", "observed_at": "2026-09-27T10:00:00Z", "received_at": "2026-09-27T10:00:01Z",
			"snapshot": map[string]any{"containers": []map[string]any{{"id": "c1", "name": "kyvault", "image": "ghcr.io/busnes-app/kyvault:1.2", "state": "running", "status": "Up 3 hours (healthy)"}}}}},
		"/api/organizations/org_a/endpoints/ep_1/samples": {200, []map[string]any{{"container_id": "c1", "observed_at": "2026-09-27T10:00:00Z", "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 2}}},
	})
	c := &kyyard.Client{HTTP: h, Config: kyyard.Config{URL: "https://yard.lan", Token: "tok", OrganizationID: "org_a"}}
	eps, err := c.Endpoints(context.Background())
	if err != nil || len(eps) != 2 || eps[0].ID != "ep_1" {
		t.Fatalf("endpoints: %+v %v", eps, err)
	}
	if h.gets[0].headers["Authorization"] != "Bearer tok" {
		t.Fatalf("bearer missing: %+v", h.gets[0])
	}
	inv, err := c.Inventory(context.Background(), "ep_1")
	if err != nil || inv.EndpointID != "ep_1" || len(inv.Containers) != 1 || inv.Containers[0].Name != "kyvault" || inv.Containers[0].Status != "Up 3 hours (healthy)" {
		t.Fatalf("inventory: %+v %v", inv, err)
	}
	s, err := c.Samples(context.Background(), "ep_1")
	if err != nil || len(s) != 1 || s[0].MemoryLimit != 4000 || s[0].RestartCount != 2 {
		t.Fatalf("samples: %+v %v", s, err)
	}
	h.answers["/api/organizations/org_a/endpoints?limit=200"] = struct {
		code int
		body any
	}{401, map[string]string{"error": "Authentication required"}}
	if _, err := c.Endpoints(context.Background()); kyyard.Reason(err) != "unauthorized" {
		t.Fatalf("401 reason: %v", err)
	}
	h.answers["/api/organizations/org_a/endpoints?limit=200"] = struct {
		code int
		body any
	}{500, nil}
	if _, err := c.Endpoints(context.Background()); kyyard.Reason(err) != "status_500" {
		t.Fatalf("500 reason: %v", err)
	}
	h.err = egress.ErrRefusedAddress
	if _, err := c.Endpoints(context.Background()); kyyard.Reason(err) != "address_refused" {
		t.Fatalf("egress reason: %v", err)
	}
}
