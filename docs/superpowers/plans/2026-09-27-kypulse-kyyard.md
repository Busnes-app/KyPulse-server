# kyPulse KyYard integration (step 3b) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pair kyPulse to one KyYard organization with a pairing code, pull endpoints, container inventory and resource samples every 60 s, show KyYard container facts on the app detail page, offer KyYard containers as suggestions when adding an app, and mark KyYard data stale in the alert bar when KyYard is unreachable.

**Architecture:** A new `internal/kyyard` package owns the sealed pairing (URL, organization, token — sealed under the deployment key like the webhook), a thin client over the egress guard (bearer `Authorization`), and a `Service` that keeps the latest snapshot in memory (endpoints, containers with derived facts, per-container restart history for the last hour) and refreshes it on a 60 s loop. Nothing KyYard-derived is stored in the database: a restart re-pulls within a minute. The API exposes pairing (admin), a status view, container suggestions, and merges container facts into the target detail; `/api/status` carries the stale flag for the alert bar. Logs and the audit feed from KyYard are step 4, where their storage and retention land.

**Tech Stack:** Go 1.26, `internal/egress` (one new `GetWith`), `ky-primitives/recoveryclient` sealer (as `monitor.Webhooks` uses), React 19 + TypeScript strict, vitest, Playwright not extended (no LAN KyYard in CI), smoke test.

**Spec:** `docs/superpowers/specs/2026-09-26-kypulse-design.md` §3 "kyPulse side", §2 "Watched apps and polling" (KyYard suggestions prefill only), Screens → Alert bar ("marks KyYard data stale") and App detail (KyYard container facts). KyYard's API: `Busnes-app/KyYard-server` PR #83 (`POST /api/service-tokens/claim`, bearer reads of `GET /api/organizations/{org}/endpoints`, `.../endpoints/{id}/inventory`, `.../endpoints/{id}/samples`).

## Global Constraints

- Every setting is `KYPULSE_*`: new `KYPULSE_KYYARD_ALLOW_HTTP` (default false) admits a plain-http KyYard URL; loopback and link-local stay refused by the egress guard regardless.
- The token is sealed in `server_settings.kyyard_enc` under label `kypulse:setting:kyyard`, filtered from `/api/settings` by the `_enc` suffix, never logged, never audited, never returned by any route. The pairing code is never logged or audited either.
- Pull cadence 60 s (`PullEvery`), request budget through the egress client's 5 s timeout, `StaleAfter` 3 min: stale means paired and no successful pull within 3 min (or never).
- KyYard data is read-only and best-effort: a failed pull keeps the previous snapshot, records the reason (`egress.Cause` vocabulary or `status_NNN`, `unauthorized` for 401), logs one line, and health polling is unaffected.
- Container link format on a target: `<endpoint_id>/<container_name>` (the existing `container` field, ≤128 chars). Suggestions only prefill the add form; nothing is polled until an admin saves.
- Facts shown per linked container: state, Docker status text, health (parsed from the status text: `(healthy)`, `(unhealthy)`, `(health: starting)`, else `none`), exit code (parsed from `Exited (N)`), image, memory bytes against limit (latest sample), restart count (latest sample) and restarts in the last hour (from the in-memory history), observed-at, and whether the data is stale.
- Pairing and unpairing are admin-only and audited (`admin.kyyard_pair` with `host=<host> org=<org id>`, `admin.kyyard_unpair`); the UI names KyYard's revoke step on unpair and KyYard's pairing screen on pair.
- Viewers can read `/api/kyyard` status and container suggestions? No: suggestions are for the add form, admin-only; the status view is for any session (the alert bar shows stale to viewers).
- `web/dist` rebuilt and committed in the last task; DOX pass; trailer `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.

## Review Focus

1. **KyYard revokes the token**: the next pull gets 401; the alert bar must say stale with reason `unauthorized` within a minute, and the Settings card must show it so the admin re-pairs. Pinned in Task 2 (service test) and Task 4 (card renders the reason).
2. **A linked container disappears from inventory** (recreated under a new name or removed): the detail page says "not seen in KyYard" rather than showing the old facts forever. Pinned in Task 2 (`Facts` returns false after a snapshot without it) and Task 4.
3. **Unpair while a pull is in flight**: the loop must not write a snapshot for a pairing that no longer exists, and must not log the old token. Pinned in Task 2.
4. **A KyYard URL pointing at kyPulse itself or loopback**: refused at pair time with a clear 400. Pinned in Task 3.
5. **Two endpoints with a container of the same name**: links are per endpoint, suggestions show the endpoint name, and facts resolve by the full link. Pinned in Task 2.

---

### Task 1: Pairing store, client and egress `GetWith`

**Files:**
- Modify: `internal/egress/egress.go` (`GetWith`), `internal/egress/egress_test.go`
- Modify: `internal/config/config.go` (`KyYard.AllowHTTP`), `internal/config/AGENTS.md`
- Create: `internal/kyyard/pairing.go`, `internal/kyyard/client.go`, `internal/kyyard/AGENTS.md`
- Test: `internal/kyyard/client_test.go`, `internal/kyyard/pairing_test.go`

**Interfaces:**
- Produces: `egress.Client.GetWith(ctx, rawURL string, headers map[string]string) (*Response, error)`; `config.Config.KyYard.AllowHTTP`; in `kyyard`: `Config{URL, Token, OrganizationID, OrganizationName string}`, `Pairing{Settings, Sealer}` with `NewPairing(cfg, settings)`, `Save/Load/Delete` (Load returns `(Config, bool, error)`, unreadable → `ErrUnreadable`), `HTTP` interface `{ GetWith(...); Post(...) }` satisfied by `*egress.Client`, `Claim(ctx, h HTTP, baseURL, code string) (Config, error)` with `ErrPairingRefused`, `ErrRateLimited`, and `Client{HTTP HTTP; Config Config}` with `Endpoints(ctx) ([]Endpoint, error)`, `Inventory(ctx, endpointID) (Inventory, error)`, `Samples(ctx, endpointID) ([]Sample, error)`, errors classified by `Reason(err) string` (`unauthorized`, `status_NNN`, egress causes).

- [ ] **Step 1: Write the failing egress test**

Add to `internal/egress/egress_test.go`, following the file's fake-dial pattern:

```go
func TestGetWithSendsHeaders(t *testing.T) {
	var got http.Header
	c, base := testClientAndServer(t, func(w http.ResponseWriter, r *http.Request) { got = r.Header.Clone() }) // use the file's helper that stands up a server reachable through the fake resolver
	if _, err := c.GetWith(context.Background(), base+"/x", map[string]string{"Authorization": "Bearer abc"}); err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer abc" || got.Get("User-Agent") != "kypulse" {
		t.Fatalf("headers: %v", got)
	}
}
```

Read the test file first and reuse its exact helper names (the guard refuses loopback, so its tests already route a test server through an allowed address).

- [ ] **Step 2: Implement `GetWith`**

```go
// GetWith is Get with extra headers, for a bearer-authenticated read.
func (c *Client) GetWith(ctx context.Context, rawURL string, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodGet, rawURL, "", nil, headers)
}
```

Run: `go test ./internal/egress/` → PASS.

- [ ] **Step 3: Config**

`config.go`: add `KyYard KyYardConfig` to `Config` and

```go
// KyYardConfig is the KyYard integration's process configuration; the pairing itself is a
// sealed setting.
type KyYardConfig struct {
	// AllowHTTP admits a plain-http KyYard URL. Off by default: the token rides in a header.
	AllowHTTP bool `json:"allow_http"`
}
```

with `KyYard: KyYardConfig{AllowHTTP: getEnvBool("KYPULSE_KYYARD_ALLOW_HTTP", false)}` in `LoadFromEnv`. `internal/config/AGENTS.md`: one sentence beside `KYPULSE_ALERT_ALLOW_HTTP`.

- [ ] **Step 4: Write the failing kyyard tests**

```go
// internal/kyyard/client_test.go
package kyyard_test

import (
	"context"
	"encoding/json"
	"errors"
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
	posts []struct{ url string; body []byte; headers map[string]string }
	gets  []struct{ url string; headers map[string]string }
}

func (f *fakeHTTP) GetWith(_ context.Context, rawURL string, headers map[string]string) (*egress.Response, error) {
	f.gets = append(f.gets, struct{ url string; headers map[string]string }{rawURL, headers})
	return f.answer(rawURL)
}
func (f *fakeHTTP) Post(_ context.Context, rawURL, _ string, body []byte, headers map[string]string) (*egress.Response, error) {
	f.posts = append(f.posts, struct{ url string; body []byte; headers map[string]string }{rawURL, body, headers})
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

func answers(m map[string]struct{ code int; body any }) *fakeHTTP { return &fakeHTTP{answers: m} }

func TestClaimPostsServiceNameAndSealsNothingInErrors(t *testing.T) {
	h := answers(map[string]struct{ code int; body any }{
		"/api/service-tokens/claim": {200, map[string]any{"token": "ab12", "organization": map[string]string{"id": "org_a", "name": "A"}}},
	})
	cfg, err := kyyard.Claim(context.Background(), h, "https://yard.lan/", "123456")
	if err != nil || cfg.Token != "ab12" || cfg.OrganizationID != "org_a" || cfg.OrganizationName != "A" || cfg.URL != "https://yard.lan" {
		t.Fatalf("claim: %+v %v", cfg, err)
	}
	if len(h.posts) != 1 || h.posts[0].url != "https://yard.lan/api/service-tokens/claim" || string(h.posts[0].body) != `{"pairing_code":"123456","service_name":"kypulse"}` {
		t.Fatalf("post: %+v", h.posts)
	}
	h = answers(map[string]struct{ code int; body any }{"/api/service-tokens/claim": {403, map[string]string{"error": "Pairing refused"}}})
	if _, err := kyyard.Claim(context.Background(), h, "https://yard.lan", "000000"); !errors.Is(err, kyyard.ErrPairingRefused) {
		t.Fatalf("refused: %v", err)
	}
	h = answers(map[string]struct{ code int; body any }{"/api/service-tokens/claim": {429, map[string]string{"error": "slow down"}}})
	if _, err := kyyard.Claim(context.Background(), h, "https://yard.lan", "000000"); !errors.Is(err, kyyard.ErrRateLimited) {
		t.Fatalf("limited: %v", err)
	}
	if _, err := kyyard.Claim(context.Background(), h, "https://yard.lan", "12345"); err == nil {
		t.Fatal("a code that is not six digits must be refused before any request")
	}
}

func TestClientReadsWithBearerAndClassifiesErrors(t *testing.T) {
	h := answers(map[string]struct{ code int; body any }{
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
	h.answers["/api/organizations/org_a/endpoints?limit=200"] = struct{ code int; body any }{401, map[string]string{"error": "Authentication required"}}
	if _, err := c.Endpoints(context.Background()); kyyard.Reason(err) != "unauthorized" {
		t.Fatalf("401 reason: %v", err)
	}
	h.answers["/api/organizations/org_a/endpoints?limit=200"] = struct{ code int; body any }{500, nil}
	if _, err := c.Endpoints(context.Background()); kyyard.Reason(err) != "status_500" {
		t.Fatalf("500 reason: %v", err)
	}
	h.err = egress.ErrRefusedAddress
	if _, err := c.Endpoints(context.Background()); kyyard.Reason(err) != "address_refused" {
		t.Fatalf("egress reason: %v", err)
	}
}
```

```go
// internal/kyyard/pairing_test.go
package kyyard_test

import (
	"context"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func TestPairingSealsAndFiltersFromSettings(t *testing.T) {
	cfg := &config.Config{Database: testdb.Config(t)}
	cfg.Security.EncryptionKey = make([]byte, 32)
	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	p, err := kyyard.NewPairing(cfg, st.Settings())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := p.Load(context.Background()); err != nil || ok {
		t.Fatalf("empty: %v %v", ok, err)
	}
	if err := p.Save(context.Background(), kyyard.Config{URL: "https://yard.lan", Token: "secret-token", OrganizationID: "org_a", OrganizationName: "A"}); err != nil {
		t.Fatal(err)
	}
	all, _ := st.Settings().GetAllSettings(context.Background())
	for k, v := range all {
		if k != "kyyard_enc" && (contains(v, "secret-token")) {
			t.Fatalf("token leaked into %s", k)
		}
	}
	if raw := all["kyyard_enc"]; raw == "" || contains(raw, "secret-token") {
		t.Fatal("kyyard_enc must be sealed")
	}
	got, ok, err := p.Load(context.Background())
	if err != nil || !ok || got.Token != "secret-token" || got.OrganizationName != "A" {
		t.Fatalf("load: %+v %v %v", got, ok, err)
	}
	if err := p.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := p.Load(context.Background()); ok {
		t.Fatal("deleted pairing still loads")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

Use `strings.Contains` instead of the helper if the file already imports `strings`.

- [ ] **Step 5: Run them to see them fail**

Run: `go test ./internal/kyyard/` → FAIL to compile.

- [ ] **Step 6: Implement**

```go
// internal/kyyard/pairing.go
// Package kyyard pairs kyPulse to one KyYard organization and reads what a pulse_reader
// may: endpoints, container inventory and resource samples. Everything it learns lives in
// memory; the only durable state is the sealed pairing.
package kyyard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const (
	pairingKey   = "kyyard_enc"
	pairingLabel = "kypulse:setting:kyyard"
)

// ErrUnreadable is a pairing that is set but cannot be read back (rotated key, bad JSON).
var ErrUnreadable = errors.New("kyyard: pairing cannot be read")

// Config is the pairing: where KyYard is, which organization, and the token that reads it.
// Token is never logged, audited or returned by any route.
type Config struct {
	URL              string `json:"url"`
	Token            string `json:"token"`
	OrganizationID   string `json:"organization_id"`
	OrganizationName string `json:"organization_name"`
}

// Pairing stores Config sealed under the deployment key.
type Pairing struct {
	Settings store.SettingsStore
	Sealer   recoveryclient.Sealer
}

func NewPairing(cfg *config.Config, s store.SettingsStore) (*Pairing, error) {
	sealer, err := recoveryclient.NewAESGCMSealer(cfg.Security.EncryptionKey, pairingLabel)
	if err != nil {
		return nil, err
	}
	return &Pairing{Settings: s, Sealer: sealer}, nil
}

func (p *Pairing) Save(ctx context.Context, c Config) error {
	plain, err := json.Marshal(c)
	if err != nil {
		return err
	}
	sealed, err := p.Sealer.Seal(plain)
	if err != nil {
		return err
	}
	return p.Settings.SetSetting(ctx, pairingKey, sealed)
}

func (p *Pairing) Load(ctx context.Context) (Config, bool, error) {
	sealed, err := p.Settings.GetSetting(ctx, pairingKey)
	if errors.Is(err, store.ErrNotFound) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	var c Config
	plain, err := p.Sealer.Open(sealed)
	if err == nil {
		err = json.Unmarshal(plain, &c)
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	return c, true, nil
}

func (p *Pairing) Delete(ctx context.Context) error {
	return p.Settings.DeleteSetting(ctx, pairingKey)
}
```

```go
// internal/kyyard/client.go
package kyyard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// HTTP is what the client needs from the egress guard; *egress.Client satisfies it.
type HTTP interface {
	GetWith(ctx context.Context, rawURL string, headers map[string]string) (*egress.Response, error)
	Post(ctx context.Context, rawURL, contentType string, body []byte, headers map[string]string) (*egress.Response, error)
}

var (
	ErrPairingRefused = errors.New("kyyard: pairing refused")
	ErrRateLimited    = errors.New("kyyard: too many pairing attempts; wait a minute")
	ErrUnauthorized   = errors.New("kyyard: token refused")
)

// StatusError is a non-2xx answer other than the ones above; the body is never kept.
type StatusError struct{ Code int }

func (e StatusError) Error() string { return fmt.Sprintf("kyyard: status %d", e.Code) }

var codeRe = regexp.MustCompile(`^[0-9]{6}$`)

// Claim exchanges a pairing code for a token. The code and the token never reach a log.
func Claim(ctx context.Context, h HTTP, baseURL, code string) (Config, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if _, err := url.Parse(base); err != nil || base == "" {
		return Config{}, fmt.Errorf("kyyard: bad url")
	}
	if !codeRe.MatchString(code) {
		return Config{}, ErrPairingRefused
	}
	body, _ := json.Marshal(map[string]string{"pairing_code": code, "service_name": "kypulse"})
	resp, err := h.Post(ctx, base+"/api/service-tokens/claim", "application/json", body, nil)
	if err != nil {
		return Config{}, err
	}
	switch resp.StatusCode {
	case 200:
	case 403:
		return Config{}, ErrPairingRefused
	case 429:
		return Config{}, ErrRateLimited
	default:
		return Config{}, StatusError{resp.StatusCode}
	}
	var out struct {
		Token        string `json:"token"`
		Organization struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"organization"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil || out.Token == "" || out.Organization.ID == "" {
		return Config{}, fmt.Errorf("kyyard: claim answer not understood")
	}
	return Config{URL: base, Token: out.Token, OrganizationID: out.Organization.ID, OrganizationName: out.Organization.Name}, nil
}

// Endpoint is what kyPulse keeps of a KyYard endpoint.
type Endpoint struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Runtime    string     `json:"runtime"`
	State      string     `json:"state"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// Container is one inventory entry; Status is Docker's human text ("Up 3 hours (healthy)",
// "Exited (137) 2 hours ago"), which is where health and exit code come from.
type Container struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
}

type Inventory struct {
	EndpointID string
	State      string
	ObservedAt time.Time
	ReceivedAt time.Time
	Containers []Container
}

type Sample struct {
	ContainerID  string    `json:"container_id"`
	ObservedAt   time.Time `json:"observed_at"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemoryBytes  int64     `json:"memory_bytes"`
	MemoryLimit  int64     `json:"memory_limit"`
	RestartCount int64     `json:"restart_count"`
}

// Client reads one organization with a bearer token.
type Client struct {
	HTTP   HTTP
	Config Config
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	resp, err := c.HTTP.GetWith(ctx, c.Config.URL+"/api/organizations/"+url.PathEscape(c.Config.OrganizationID)+path, map[string]string{"Authorization": "Bearer " + c.Config.Token})
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == 401:
		return ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return StatusError{resp.StatusCode}
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		return fmt.Errorf("kyyard: answer not understood: %w", err)
	}
	return nil
}

func (c *Client) Endpoints(ctx context.Context) ([]Endpoint, error) {
	var out []Endpoint
	if err := c.get(ctx, "/endpoints?limit=200", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Inventory(ctx context.Context, endpointID string) (Inventory, error) {
	var raw struct {
		EndpointID string    `json:"endpoint_id"`
		State      string    `json:"state"`
		ObservedAt time.Time `json:"observed_at"`
		ReceivedAt time.Time `json:"received_at"`
		Snapshot   struct {
			Containers []Container `json:"containers"`
		} `json:"snapshot"`
	}
	if err := c.get(ctx, "/endpoints/"+url.PathEscape(endpointID)+"/inventory", &raw); err != nil {
		return Inventory{}, err
	}
	return Inventory{EndpointID: raw.EndpointID, State: raw.State, ObservedAt: raw.ObservedAt, ReceivedAt: raw.ReceivedAt, Containers: raw.Snapshot.Containers}, nil
}

func (c *Client) Samples(ctx context.Context, endpointID string) ([]Sample, error) {
	var out []Sample
	if err := c.get(ctx, "/endpoints/"+url.PathEscape(endpointID)+"/samples", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Reason names a pull failure in a fixed vocabulary for the status view and the log.
func Reason(err error) string {
	var se StatusError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.As(err, &se):
		return fmt.Sprintf("status_%d", se.Code)
	}
	if c := egress.Cause(err); c != "" {
		return c
	}
	return "network"
}
```

Confirm the inventory snapshot's container list key is `containers` by reading `/home/yoshi/git/busnes.app/KyYard-Server/internal/agent/protocol/inventory.go` (`Snapshot` struct); adjust if it differs.

- [ ] **Step 7: Run, docs, commit**

Run: `go test -race ./internal/kyyard/ ./internal/egress/ ./internal/config/` → PASS.

`internal/kyyard/AGENTS.md`:

```markdown
# KyYard

## Purpose
Pairs kyPulse to one KyYard organization and reads what a `pulse_reader` may: endpoints, container inventory and resource samples.

## Ownership
`Pairing` (sealed URL, organization and token in `server_settings.kyyard_enc`), `Client` (bearer reads over the egress guard), `Claim` (pairing-code exchange), and the `Service` snapshot loop.

## Local Contracts
- The token and the pairing code never reach a log line, an audit row or an HTTP response; `Reason(err)` is the only text stored about a failure.
- `Claim` posts `{pairing_code, service_name:"kypulse"}`; 403 is `ErrPairingRefused`, 429 `ErrRateLimited`; a code that is not six digits is refused before any request.
- Reads carry `Authorization: Bearer`; 401 is `ErrUnauthorized` (reason `unauthorized`), other non-2xx `status_NNN`, transport failures the egress vocabulary.
- Nothing KyYard-derived is written to the database.

## Verification
- `go test -race ./internal/kyyard/`

## Child DOX Index
None.
```

```bash
git add internal/egress internal/config internal/kyyard
git commit -m "kyyard: sealed pairing, claim and bearer reads over the egress guard"
```

---

### Task 2: The snapshot service

**Files:**
- Create: `internal/kyyard/service.go`, `internal/kyyard/facts.go`
- Test: `internal/kyyard/service_test.go`, `internal/kyyard/facts_test.go`
- Modify: `internal/kyyard/AGENTS.md`

**Interfaces:**
- Consumes: Task 1.
- Produces: `Service{Pairing *Pairing; HTTP HTTP; Logger *logging.Logger; Now func() time.Time}` with `Run(ctx, every time.Duration, done chan<- struct{})`, `PullNow(ctx) error`, `Clear()`, `Status(now) StatusView`, `Facts(link string, now) (ContainerFacts, bool)`, `Suggestions() []Suggestion`; constants `PullEvery = 60 * time.Second`, `StaleAfter = 3 * time.Minute`; pure `ParseStatus(status string) (health string, exitCode *int)`, `LinkFor(endpointID, name string) string`.

- [ ] **Step 1: Write the failing facts test**

```go
// internal/kyyard/facts_test.go
package kyyard_test

import (
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/kyyard"
)

func TestParseStatus(t *testing.T) {
	cases := []struct {
		status string
		health string
		exit   int // -1 = nil
	}{
		{"Up 3 hours (healthy)", "healthy", -1},
		{"Up 2 minutes (unhealthy)", "unhealthy", -1},
		{"Up 10 seconds (health: starting)", "starting", -1},
		{"Up 5 days", "none", -1},
		{"Exited (137) 2 hours ago", "none", 137},
		{"Exited (0) 3 days ago", "none", 0},
		{"Restarting (1) 5 seconds ago", "none", 1},
		{"", "none", -1},
	}
	for _, c := range cases {
		h, code := kyyard.ParseStatus(c.status)
		if h != c.health || (c.exit == -1 && code != nil) || (c.exit != -1 && (code == nil || *code != c.exit)) {
			t.Errorf("%q: got %s %v", c.status, h, code)
		}
	}
	if kyyard.LinkFor("ep_1", "kyvault") != "ep_1/kyvault" {
		t.Fatal("link format")
	}
}
```

- [ ] **Step 2: Write the failing service test**

```go
// internal/kyyard/service_test.go
package kyyard_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func pairedService(t *testing.T, h *fakeHTTP) (*kyyard.Service, *time.Time) {
	t.Helper()
	cfg := &config.Config{Database: testdb.Config(t)}
	cfg.Security.EncryptionKey = make([]byte, 32)
	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := kyyard.NewPairing(cfg, st.Settings())
	if err := p.Save(context.Background(), kyyard.Config{URL: "https://yard.lan", Token: "tok", OrganizationID: "org_a", OrganizationName: "A"}); err != nil {
		t.Fatal(err)
	}
	lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	svc := &kyyard.Service{Pairing: p, HTTP: h, Logger: lg, Now: func() time.Time { return now }}
	return svc, &now
}

func yard() *fakeHTTP {
	return answers(map[string]struct{ code int; body any }{
		"/api/organizations/org_a/endpoints?limit=200": {200, []map[string]any{{"id": "ep_1", "name": "host-1", "runtime": "docker", "state": "active"}, {"id": "ep_2", "name": "host-2", "runtime": "docker", "state": "active"}, {"id": "ep_k", "name": "k8s", "runtime": "kubernetes", "state": "active"}}},
		"/api/organizations/org_a/endpoints/ep_1/inventory": {200, map[string]any{"endpoint_id": "ep_1", "state": "complete", "observed_at": "2026-09-27T09:59:50Z", "received_at": "2026-09-27T09:59:51Z",
			"snapshot": map[string]any{"containers": []map[string]any{{"id": "c1", "name": "kyvault", "image": "ghcr.io/busnes-app/kyvault:1.2", "state": "running", "status": "Up 3 hours (healthy)"}}}}},
		"/api/organizations/org_a/endpoints/ep_1/samples": {200, []map[string]any{{"container_id": "c1", "observed_at": "2026-09-27T09:59:50Z", "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 2}}},
		"/api/organizations/org_a/endpoints/ep_2/inventory": {200, map[string]any{"endpoint_id": "ep_2", "state": "complete", "observed_at": "2026-09-27T09:59:50Z", "received_at": "2026-09-27T09:59:51Z",
			"snapshot": map[string]any{"containers": []map[string]any{{"id": "c9", "name": "kyvault", "image": "ghcr.io/busnes-app/kyvault:1.1", "state": "exited", "status": "Exited (137) 2 hours ago"}}}}},
		"/api/organizations/org_a/endpoints/ep_2/samples": {200, []map[string]any{}},
	})
}

func TestPullBuildsFactsPerEndpointAndSkipsKubernetes(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	if err := svc.PullNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, g := range h.gets {
		if len(g.url) >= 4 && g.url[len(g.url)-4:] == "ep_k" || containsStr(g.url, "ep_k/") {
			t.Fatalf("kubernetes endpoint must not be pulled: %s", g.url)
		}
	}
	f, ok := svc.Facts("ep_1/kyvault", *now)
	if !ok || f.Health != "healthy" || f.MemoryBytes != 1000 || f.MemoryLimit != 4000 || f.RestartCount != 2 || f.EndpointName != "host-1" || f.Stale {
		t.Fatalf("facts: %+v %v", f, ok)
	}
	g, ok := svc.Facts("ep_2/kyvault", *now)
	if !ok || g.ExitCode == nil || *g.ExitCode != 137 || g.State != "exited" {
		t.Fatalf("second endpoint's facts: %+v %v", g, ok)
	}
	if _, ok := svc.Facts("ep_1/nothere", *now); ok {
		t.Fatal("unknown link must not resolve")
	}
	sug := svc.Suggestions()
	if len(sug) != 2 || sug[0].Link != "ep_1/kyvault" || sug[0].EndpointName != "host-1" || sug[1].Link != "ep_2/kyvault" {
		t.Fatalf("suggestions: %+v", sug)
	}
	st := svc.Status(*now)
	if !st.Paired || st.Stale || st.Organization != "A" || st.URL != "https://yard.lan" || st.Error != "" || st.FetchedAt == nil {
		t.Fatalf("status: %+v", st)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRestartsInTheLastHourAndStaleness(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	_ = svc.PullNow(context.Background())
	*now = now.Add(30 * time.Minute)
	h.answers["/api/organizations/org_a/endpoints/ep_1/samples"] = struct{ code int; body any }{200, []map[string]any{{"container_id": "c1", "observed_at": now.Format(time.RFC3339), "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 5}}}
	_ = svc.PullNow(context.Background())
	f, _ := svc.Facts("ep_1/kyvault", *now)
	if f.RestartsLastHour != 3 {
		t.Fatalf("restarts in the last hour: %+v", f)
	}
	*now = now.Add(45 * time.Minute) // the first sample (restart_count 2) is older than an hour now
	_ = svc.PullNow(context.Background())
	f, _ = svc.Facts("ep_1/kyvault", *now)
	if f.RestartsLastHour != 0 {
		t.Fatalf("history not trimmed: %+v", f)
	}
	// A failing pull keeps the last snapshot, records the reason, and goes stale after 3 min.
	h.err = context.DeadlineExceeded
	*now = now.Add(time.Minute)
	if err := svc.PullNow(context.Background()); err == nil {
		t.Fatal("failed pull must report")
	}
	st := svc.Status(*now)
	if st.Error != "timeout" || st.Stale {
		t.Fatalf("one failure is not yet stale: %+v", st)
	}
	f, ok := svc.Facts("ep_1/kyvault", *now)
	if !ok || f.Stale {
		t.Fatalf("last snapshot must survive one failure: %+v %v", f, ok)
	}
	*now = now.Add(3 * time.Minute)
	if st := svc.Status(*now); !st.Stale {
		t.Fatalf("must be stale after StaleAfter: %+v", st)
	}
	if f, _ := svc.Facts("ep_1/kyvault", *now); !f.Stale {
		t.Fatal("facts must carry stale")
	}
}

func TestRevokedTokenReportsUnauthorized(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	h.answers["/api/organizations/org_a/endpoints?limit=200"] = struct{ code int; body any }{401, map[string]string{"error": "Authentication required"}}
	_ = svc.PullNow(context.Background())
	if st := svc.Status(*now); st.Error != "unauthorized" {
		t.Fatalf("status: %+v", st)
	}
}

func TestUnpairClearsAndDisappearedContainerIsGone(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	_ = svc.PullNow(context.Background())
	h.answers["/api/organizations/org_a/endpoints/ep_1/inventory"] = struct{ code int; body any }{200, map[string]any{"endpoint_id": "ep_1", "state": "complete", "observed_at": now.Format(time.RFC3339), "received_at": now.Format(time.RFC3339), "snapshot": map[string]any{"containers": []map[string]any{}}}}
	_ = svc.PullNow(context.Background())
	if _, ok := svc.Facts("ep_1/kyvault", *now); ok {
		t.Fatal("a container missing from the latest inventory must not resolve")
	}
	if err := svc.Pairing.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.Clear()
	if st := svc.Status(*now); st.Paired || st.FetchedAt != nil {
		t.Fatalf("after unpair: %+v", st)
	}
	if err := svc.PullNow(context.Background()); err != nil {
		t.Fatalf("an unpaired pull is a no-op: %v", err)
	}
	if len(svc.Suggestions()) != 0 {
		t.Fatal("suggestions must be empty when unpaired")
	}
}

func TestRunPullsOnScheduleAndStops(t *testing.T) {
	h := yard()
	svc, _ := pairedService(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go svc.Run(ctx, 10*time.Millisecond, done)
	deadline := time.After(2 * time.Second)
	for len(h.gets) < 6 {
		select {
		case <-deadline:
			t.Fatalf("only %d requests", len(h.gets))
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}
```

`fakeHTTP.gets` is read concurrently in the last test; add a mutex to the fake (`mu sync.Mutex`) around the appends and a `getCount()` accessor, and use it there.

- [ ] **Step 3: Run to see them fail**

Run: `go test ./internal/kyyard/` → FAIL to compile.

- [ ] **Step 4: Implement facts**

```go
// internal/kyyard/facts.go
package kyyard

import (
	"regexp"
	"strconv"
	"time"
)

var (
	exitRe   = regexp.MustCompile(`^(?:Exited|Restarting) \((\d+)\)`)
	healthRe = regexp.MustCompile(`\((healthy|unhealthy|health: starting)\)`)
)

// ParseStatus reads Docker's status text: health from the "(healthy)" suffix Docker adds
// for a container with a healthcheck, exit code from "Exited (N)" or "Restarting (N)".
func ParseStatus(status string) (health string, exitCode *int) {
	health = "none"
	if m := healthRe.FindStringSubmatch(status); m != nil {
		switch m[1] {
		case "healthy", "unhealthy":
			health = m[1]
		default:
			health = "starting"
		}
	}
	if m := exitRe.FindStringSubmatch(status); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			exitCode = &n
		}
	}
	return health, exitCode
}

// LinkFor is the value a target's container field holds.
func LinkFor(endpointID, name string) string { return endpointID + "/" + name }

// ContainerFacts is what the app detail page shows for a linked container.
type ContainerFacts struct {
	Link             string    `json:"link"`
	EndpointID       string    `json:"endpoint_id"`
	EndpointName     string    `json:"endpoint_name"`
	ContainerID      string    `json:"container_id"`
	Name             string    `json:"name"`
	Image            string    `json:"image"`
	State            string    `json:"state"`
	Status           string    `json:"status"`
	Health           string    `json:"health"`
	ExitCode         *int      `json:"exit_code,omitempty"`
	ObservedAt       time.Time `json:"observed_at"`
	MemoryBytes      int64     `json:"memory_bytes"`
	MemoryLimit      int64     `json:"memory_limit"`
	RestartCount     int64     `json:"restart_count"`
	RestartsLastHour int64     `json:"restarts_last_hour"`
	Stale            bool      `json:"stale"`
}

// Suggestion prefills the add form: the container's name and its link.
type Suggestion struct {
	Link         string `json:"link"`
	EndpointName string `json:"endpoint_name"`
	Name         string `json:"name"`
	Image        string `json:"image"`
	State        string `json:"state"`
}

// StatusView is the pairing as the API and the alert bar see it. The token is not here.
type StatusView struct {
	Paired       bool       `json:"paired"`
	URL          string     `json:"url,omitempty"`
	Organization string     `json:"organization,omitempty"`
	FetchedAt    *time.Time `json:"fetched_at,omitempty"`
	Stale        bool       `json:"stale"`
	Error        string     `json:"error,omitempty"`
}
```

- [ ] **Step 5: Implement the service**

```go
// internal/kyyard/service.go
package kyyard

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
)

const (
	PullEvery  = 60 * time.Second
	StaleAfter = 3 * time.Minute
	// historyWindow is how far back restart counts are kept, for "restarts in the last hour".
	historyWindow = time.Hour
)

var (
	evPullFailed = logging.DeclareEvent("kyyard_pull_failed", "KyYard could not be read", slog.LevelWarn)
	evPulled     = logging.DeclareEvent("kyyard_pulled", "KyYard inventory refreshed", slog.LevelDebug)
	fEndpoints   = logging.DeclareInt("endpoints")
	fContainers  = logging.DeclareInt("containers")
)

type restartPoint struct {
	at    time.Time
	count int64
}

// Service keeps the latest KyYard snapshot in memory and refreshes it on a schedule.
type Service struct {
	Pairing *Pairing
	HTTP    HTTP
	Logger  *logging.Logger
	Now     func() time.Time

	mu        sync.Mutex
	paired    bool
	cfg       Config
	fetchedAt *time.Time // last successful pull
	lastErr   string     // reason of the last failed pull, "" after a success
	facts     map[string]ContainerFacts
	history   map[string][]restartPoint
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Run pulls every `every` until ctx ends, then closes done. The first pull is immediate.
func (s *Service) Run(ctx context.Context, every time.Duration, done chan<- struct{}) {
	defer close(done)
	_ = s.PullNow(ctx)
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.PullNow(ctx)
		}
	}
}

// PullNow reads the pairing, then endpoints, inventory and samples. A failure keeps the last
// snapshot and records its reason. Unpaired is a no-op that clears nothing (Clear does).
func (s *Service) PullNow(ctx context.Context) error {
	cfg, ok, err := s.Pairing.Load(ctx)
	if err != nil {
		s.fail("unreadable")
		return err
	}
	if !ok {
		s.Clear()
		return nil
	}
	c := &Client{HTTP: s.HTTP, Config: cfg}
	eps, err := c.Endpoints(ctx)
	if err != nil {
		s.fail(Reason(err))
		s.Logger.Log(ctx, evPullFailed, logging.ReasonCode(Reason(err)))
		return err
	}
	now := s.now()
	facts := map[string]ContainerFacts{}
	for _, ep := range eps {
		if ep.Runtime != "docker" {
			continue
		}
		inv, err := c.Inventory(ctx, ep.ID)
		if err != nil {
			s.fail(Reason(err))
			s.Logger.Log(ctx, evPullFailed, logging.ReasonCode(Reason(err)))
			return err
		}
		samples, err := c.Samples(ctx, ep.ID)
		if err != nil {
			s.fail(Reason(err))
			s.Logger.Log(ctx, evPullFailed, logging.ReasonCode(Reason(err)))
			return err
		}
		byContainer := map[string]Sample{}
		for _, smp := range samples {
			if prev, ok := byContainer[smp.ContainerID]; !ok || smp.ObservedAt.After(prev.ObservedAt) {
				byContainer[smp.ContainerID] = smp
			}
		}
		for _, ct := range inv.Containers {
			health, exit := ParseStatus(ct.Status)
			f := ContainerFacts{Link: LinkFor(ep.ID, ct.Name), EndpointID: ep.ID, EndpointName: ep.Name, ContainerID: ct.ID, Name: ct.Name, Image: ct.Image, State: ct.State, Status: ct.Status, Health: health, ExitCode: exit, ObservedAt: inv.ObservedAt}
			if smp, ok := byContainer[ct.ID]; ok {
				f.MemoryBytes, f.MemoryLimit, f.RestartCount = smp.MemoryBytes, smp.MemoryLimit, smp.RestartCount
				f.RestartsLastHour = s.recordRestarts(f.Link, now, smp.RestartCount)
			}
			facts[f.Link] = f
		}
	}
	s.mu.Lock()
	s.paired, s.cfg, s.facts, s.fetchedAt, s.lastErr = true, cfg, facts, &now, ""
	s.trimHistory(now)
	s.mu.Unlock()
	s.Logger.Log(ctx, evPulled, fEndpoints(len(eps)), fContainers(len(facts)))
	return nil
}

func (s *Service) fail(reason string) {
	s.mu.Lock()
	s.paired, s.lastErr = true, reason
	s.mu.Unlock()
}

// recordRestarts appends the count and answers how many restarts happened in the window.
// Called without the lock held; takes it itself.
func (s *Service) recordRestarts(link string, now time.Time, count int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.history == nil {
		s.history = map[string][]restartPoint{}
	}
	pts := append(s.history[link], restartPoint{at: now, count: count})
	cut := 0
	for cut < len(pts) && now.Sub(pts[cut].at) > historyWindow {
		cut++
	}
	pts = pts[cut:]
	s.history[link] = pts
	if len(pts) == 0 {
		return 0
	}
	if d := count - pts[0].count; d > 0 {
		return d
	}
	return 0
}

// trimHistory drops containers no longer in the snapshot and points older than the window.
// Caller holds the lock.
func (s *Service) trimHistory(now time.Time) {
	for link, pts := range s.history {
		if _, ok := s.facts[link]; !ok {
			delete(s.history, link)
			continue
		}
		cut := 0
		for cut < len(pts) && now.Sub(pts[cut].at) > historyWindow {
			cut++
		}
		s.history[link] = pts[cut:]
	}
}

// Clear forgets the snapshot and the pairing; called after an unpair.
func (s *Service) Clear() {
	s.mu.Lock()
	s.paired, s.cfg, s.facts, s.history, s.fetchedAt, s.lastErr = false, Config{}, nil, nil, nil, ""
	s.mu.Unlock()
}

func (s *Service) stale(now time.Time) bool {
	return s.paired && (s.fetchedAt == nil || now.Sub(*s.fetchedAt) > StaleAfter)
}

func (s *Service) Status(now time.Time) StatusView {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.paired {
		return StatusView{}
	}
	return StatusView{Paired: true, URL: s.cfg.URL, Organization: s.cfg.OrganizationName, FetchedAt: s.fetchedAt, Stale: s.stale(now), Error: s.lastErr}
}

// Facts answers for a target's container link; false when the latest snapshot has no such
// container (removed, renamed, or never seen).
func (s *Service) Facts(link string, now time.Time) (ContainerFacts, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.facts[link]
	if !ok {
		return ContainerFacts{}, false
	}
	f.Stale = s.stale(now)
	return f, true
}

func (s *Service) Suggestions() []Suggestion {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Suggestion, 0, len(s.facts))
	for _, f := range s.facts {
		out = append(out, Suggestion{Link: f.Link, EndpointName: f.EndpointName, Name: f.Name, Image: f.Image, State: f.State})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Link < out[j].Link })
	return out
}
```

Notes: `Status` for a paired service that has never pulled successfully reports `Stale: true` and the reason — that is the "revoked token" and "unreachable at start" case. On an unpaired service after `Clear`, `PullNow` finds no pairing and returns nil. Check `logging.DeclareInt` exists in ky-primitives v0.9.0 (`logging.Count` is the existing int field kyPulse uses; if `DeclareInt` is absent, use `logging.DeclareInt64`/whatever the package offers — read `$(go list -m -f '{{.Dir}}' github.com/Busnes-app/ky-primitives)/logging/fields.go`).

- [ ] **Step 6: Run and commit**

Run: `go test -race -count=3 ./internal/kyyard/` → PASS.

`internal/kyyard/AGENTS.md` Local Contracts add: "`Service.PullNow` pulls endpoints (Docker runtime only), each endpoint's inventory and latest samples, and replaces the snapshot atomically; a failed pull keeps the last snapshot and records `Reason`. `StaleAfter` 3 min without a successful pull marks everything stale; `Facts` is false for a link absent from the latest inventory. Restart history per container is kept in memory for one hour for `restarts_last_hour`. `Clear` after unpair forgets everything; nothing is written to the database."

```bash
git add internal/kyyard
git commit -m "kyyard: in-memory snapshot service with container facts and staleness"
```

---

### Task 3: API, status, target detail, wiring, smoke test

**Files:**
- Create: `internal/api/kyyard_handlers.go`
- Modify: `internal/api/server.go` (routes, `Server.kyyard`, `NewServer` signature), `internal/api/monitor_handlers.go` (`handleStatus`, `handleGetTarget`), `internal/api/AGENTS.md`
- Modify: `cmd/server/main.go` (build the service, run the loop, wait), `cmd/server/monitor.go` or a new `cmd/server/kyyard.go`
- Modify: `scripts/smoke-test.sh`, root `AGENTS.md`
- Test: `internal/api/kyyard_test.go`

**Interfaces:**
- Consumes: Task 2.
- Produces: `GET /api/kyyard` (session) → `StatusView`; `POST /api/kyyard/pair` (admin) `{url, pairing_code}` → 200 `StatusView`, 400 bad url / refused address, 403 refused, 429 limited, 502 network; `DELETE /api/kyyard` (admin) → 204; `GET /api/kyyard/containers` (admin) → `[]Suggestion`; `/api/status` gains `"kyyard": StatusView`; `GET /api/targets/{id}` gains `"kyyard": ContainerFacts|null`. `api.NewServer(cfg, st, lg, mon, yard *kyyard.Service)`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/api/kyyard_test.go
package api_test

// Uses the file's existing helpers: newTestServer (or the constructor api_test.go uses),
// loginAs/adminSession, do/doJSON, decodeMap. Read monitor_test.go and copy its fixture.

func TestKyYardStatusUnpairedAndPairRefusals(t *testing.T) {
	srv, st, admin, viewer := monitorFixture(t) // returns server, store, admin cookie, viewer cookie — adapt
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
	// The test server's kyyard service uses a fake HTTP that answers the claim and reads.
	srv, st, admin, viewer, fake := monitorFixtureWithYard(t) // builds the Service with the fake from kyyard_test-style answers; see Step 3
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
			if !strings.Contains(r.Details, "host=yard.lan") || !strings.Contains(r.Details, "org=org_a") {
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
```

- [ ] **Step 2: Run to see them fail**

Run: `go test ./internal/api/ -run TestKyYard` → FAIL (404 / compile).

- [ ] **Step 3: Handlers and wiring**

```go
// internal/api/kyyard_handlers.go
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
)

// pairBudget bounds a claim: the egress client's own 5 s plus sealing and the first pull.
const pairBudget = 20 * time.Second

func (s *Server) handleKyYardStatus(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.kyyard.Status(time.Now()))
}

func (s *Server) handleKyYardContainers(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, s.kyyard.Suggestions())
}

type pairRequest struct {
	URL         string `json:"url"`
	PairingCode string `json:"pairing_code"`
}

// handleKyYardPair claims the code and seals the token. The URL is validated by the egress
// guard (loopback, link-local and metadata addresses refused; http only by opt-in) before any
// request; the audit row names host and organization, never the code or the token.
func (s *Server) handleKyYardPair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON request body")
		return
	}
	base := trimURL(req.URL)
	if err := egress.ValidateURL(base, s.config.KyYard.AllowHTTP); err != nil {
		s.writeError(w, http.StatusBadRequest, "KyYard URL: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), pairBudget)
	defer cancel()
	actor := s.actorID(r)
	cfg, err := kyyard.Claim(ctx, s.kyyard.HTTP, base, req.PairingCode)
	u, _ := url.Parse(base)
	if err != nil {
		s.audit(ctx, actor, r, "admin.kyyard_pair", "", fmt.Sprintf("outcome=failure host=%s reason=%s", auditValue(u.Host), kyyard.Reason(err)))
		switch {
		case errors.Is(err, kyyard.ErrPairingRefused):
			s.writeError(w, http.StatusForbidden, "KyYard refused the pairing code")
		case errors.Is(err, kyyard.ErrRateLimited):
			s.writeError(w, http.StatusTooManyRequests, "KyYard is rate-limiting pairing attempts; wait a minute")
		default:
			s.writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Could not reach KyYard", "reason": kyyard.Reason(err)})
		}
		return
	}
	if err := s.kyyard.Pairing.Save(ctx, cfg); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to save the pairing")
		return
	}
	s.audit(ctx, actor, r, "admin.kyyard_pair", "", fmt.Sprintf("outcome=success host=%s org=%s allow_http=%v", auditValue(u.Host), auditValue(cfg.OrganizationID), s.config.KyYard.AllowHTTP))
	_ = s.kyyard.PullNow(ctx) // the first snapshot; a failure shows as stale with its reason
	s.writeJSON(w, http.StatusOK, s.kyyard.Status(time.Now()))
}

// handleKyYardUnpair deletes the URL and the sealed token here. Revoking the token in KyYard
// is a KyYard administrator's separate step; the UI names it.
func (s *Server) handleKyYardUnpair(w http.ResponseWriter, r *http.Request) {
	if err := s.kyyard.Pairing.Delete(r.Context()); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to remove the pairing")
		return
	}
	s.kyyard.Clear()
	s.audit(r.Context(), s.actorID(r), r, "admin.kyyard_unpair", "", "")
	w.WriteHeader(http.StatusNoContent)
}

func trimURL(raw string) string {
	base := strings.TrimSpace(raw)
	for strings.HasSuffix(base, "/") {
		base = strings.TrimSuffix(base, "/")
	}
	return base
}
```

(add `context` and `strings` imports; `auditValue` and `s.actorID` exist in `monitor_handlers.go`.) Routes in `server.go`, after the alerts block:

```go
	// KyYard: any session reads the pairing status (the alert bar shows stale); pairing,
	// unpairing and container suggestions (they prefill the admin's add form) are admin-only.
	s.mux.HandleFunc("GET /api/kyyard", s.requireSession(s.handleKyYardStatus))
	s.mux.HandleFunc("POST /api/kyyard/pair", s.tracked(s.requireAdmin(s.handleKyYardPair)))
	s.mux.HandleFunc("DELETE /api/kyyard", s.requireAdmin(s.handleKyYardUnpair))
	s.mux.HandleFunc("GET /api/kyyard/containers", s.requireAdmin(s.handleKyYardContainers))
```

`Server` gains `kyyard *kyyard.Service`; `NewServer(cfg, st, lg, mon, yard)`. `handleStatus`: add `"kyyard": s.kyyard.Status(time.Now())` to the map. `handleGetTarget`: after `targetView`, `var facts any; if t.Container != "" { if f, ok := s.kyyard.Facts(t.Container, time.Now()); ok { facts = f } }` and add `"kyyard": facts` to the response.

Test fixture: `monitorFixtureWithYard` builds a `kyyard.Service` with a fake `HTTP` (copy the `fakeHTTP` from Task 1's test into `internal/api/kyyard_test.go` as `fakeYard` with the same `yard()` answers plus a claim answer whose token is exposed as `fake.token`) and passes it to `NewServer`. The plain fixture passes a service with a fake that answers nothing (every read 404).

- [ ] **Step 4: cmd/server**

New `cmd/server/kyyard.go`:

```go
package main

import (
	"context"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// newKyYard builds the KyYard reader. Its egress client admits plain http only by opt-in.
func newKyYard(cfg *config.Config, st store.Store, lg *logging.Logger) (*kyyard.Service, error) {
	pairing, err := kyyard.NewPairing(cfg, st.Settings())
	if err != nil {
		return nil, err
	}
	return &kyyard.Service{Pairing: pairing, HTTP: egress.New(egress.Options{AllowHTTP: cfg.KyYard.AllowHTTP}), Logger: lg}, nil
}

// kyyardLoop refreshes the snapshot until ctx ends and closes done.
func kyyardLoop(ctx context.Context, svc *kyyard.Service, done chan<- struct{}) {
	svc.Run(ctx, kyyard.PullEvery, done)
}
```

In `runServer`: build it after the monitor (`yard, err := newKyYard(cfg, st, lg)`), pass to `api.NewServer`, start `go kyyardLoop(ctx, yard, kyyardDone)`, and add `kyyardDone` to `waitForBackupWork` (rename or extend its signature; read it and its test `backuploop_test.go` — keep the same wait budget). Log `KYPULSE_KYYARD_ALLOW_HTTP` at start like the alerts flag.

- [ ] **Step 5: Smoke test**

After the webhook checks in `scripts/smoke-test.sh`:

```bash
echo "==> KyYard"
contains "kyyard status is unpaired" "$(curl -s -b "$WORK/cookies" "$BASE/api/kyyard")" '"paired":false'
check "kyyard pair refuses loopback" "$(status -X POST -H 'Content-Type: application/json' -b "$WORK/cookies" -H "X-CSRF-Token: $CSRF" -d '{"url":"http://127.0.0.1:1","pairing_code":"123456"}' "$BASE/api/kyyard/pair")" "400"
check "kyyard pair needs a session" "$(status -X POST -H 'Content-Type: application/json' -d '{"url":"https://yard.lan","pairing_code":"123456"}' "$BASE/api/kyyard/pair")" "401"
contains "status carries kyyard" "$(curl -s -b "$WORK/cookies" "$BASE/api/status")" '"kyyard":{'
```

(place it where the cookie jar holds the admin session and `$CSRF` is set).

- [ ] **Step 6: Run everything, docs, commit**

Run: `go test -race ./internal/api/ ./cmd/server/` then `make ci`.

Docs: `internal/api/AGENTS.md` Monitoring table gains the four routes and the `kyyard` fields on `/api/status` and `/api/targets/{id}`; audit actions `admin.kyyard_pair` (details `outcome=… host=… org=… allow_http=…`, never the code or token) and `admin.kyyard_unpair`. Root `AGENTS.md`: Ownership lists `internal/kyyard`; Child DOX Index gains `internal/kyyard/AGENTS.md`; the `cmd/server` bullet names the KyYard loop.

```bash
git add internal/api cmd/server scripts/smoke-test.sh AGENTS.md
git commit -m "api: KyYard pairing, status, suggestions and container facts on the target"
```

---

### Task 4: Web: Settings card, alert bar, detail section, suggestions; docs; dist

**Files:**
- Modify: `web/src/monitor.ts` (types + calls), `web/src/components/AlertBar.tsx` (+ test), `web/src/pages/AppDetail.tsx` (+ test), `web/src/components/TargetForm.tsx` (+ Status test), `web/src/pages/Settings.tsx`
- Create: `web/src/components/KyYardCard.tsx` (+ test), `web/src/components/KyYardFacts.tsx`
- Modify: `web/AGENTS.md`, `README.md`, `UI-VERIFICATION.md`, `web/dist`

**Interfaces:**
- Consumes: Task 3's routes and JSON.
- Produces: `KyYardStatus`, `ContainerFacts`, `Suggestion` types; `getKyYard`, `pairKyYard`, `unpairKyYard`, `kyYardContainers`; `StatusSummary.kyyard`; `TargetDetail.kyyard`.

- [ ] **Step 1: Types and calls**

In `web/src/monitor.ts`:

```ts
export interface KyYardStatus { paired: boolean; url?: string; organization?: string; fetched_at?: string | null; stale: boolean; error?: string }
export interface ContainerFacts { link: string; endpoint_id: string; endpoint_name: string; container_id: string; name: string; image: string; state: string; status: string; health: string; exit_code?: number; observed_at: string; memory_bytes: number; memory_limit: number; restart_count: number; restarts_last_hour: number; stale: boolean }
export interface Suggestion { link: string; endpoint_name: string; name: string; image: string; state: string }
export const getKyYard = () => fetch('/api/kyyard').then((r) => readJSON<KyYardStatus>(r));
export const pairKyYard = (input: { url: string; pairing_code: string }) => secureFetch('/api/kyyard/pair', { method: 'POST', ...json(input) }).then((r) => readJSON<KyYardStatus>(r));
export const unpairKyYard = () => secureFetch('/api/kyyard', { method: 'DELETE' }).then((r) => readJSON<void>(r));
export const kyYardContainers = () => fetch('/api/kyyard/containers').then((r) => readJSON<Suggestion[]>(r));
```

`StatusSummary` gains `kyyard?: KyYardStatus`; `getTarget`'s response type gains `kyyard: ContainerFacts | null`.

- [ ] **Step 2: Alert bar**

In `AlertBar.tsx`, beside `delivery`, add:

```tsx
  const yard = status.kyyard?.paired && status.kyyard.stale ? (
    <div className="alert-bar-line alert-bar-delivery">
      <Database size={14} />
      <span>KyYard data stale{status.kyyard.error ? ` (${status.kyyard.error})` : ''}{status.kyyard.fetched_at ? `, last pull ${sinceLabel(status.kyyard.fetched_at)} ago` : ', never pulled'} · {isAdmin ? <a href={hrefFor('/settings')}>Settings</a> : 'tell an admin'}</span>
    </div>
  ) : null;
```

(import `Database` from lucide-react) and render `{yard}` wherever `{delivery}` is rendered (red, green and the `ok === 0` neutral branches). Test in `AlertBar.test.tsx`: a healthy status with `kyyard: { paired: true, stale: true, error: 'unauthorized', fetched_at: null }` renders "KyYard data stale (unauthorized), never pulled"; with `stale: false` nothing about KyYard renders; unpaired renders nothing.

- [ ] **Step 3: Settings card**

```tsx
// web/src/components/KyYardCard.tsx
import React, { useEffect, useState } from 'react';
import { Database, Link2, Unlink } from 'lucide-react';
import { ApiError, getKyYard, pairKyYard, sinceLabel, timeLabel, unpairKyYard, type KyYardStatus } from '../monitor';

// KyYardCard pairs this kyPulse to one KyYard organization with a code a KyYard
// administrator generated (Members → Service tokens → Pair kyPulse). Unpairing here deletes
// the URL and token here only; revoking the token is KyYard's step.
export const KyYardCard: React.FC<{ onChanged: () => void }> = ({ onChanged }) => {
  const [status, setStatus] = useState<KyYardStatus | null>(null);
  const [url, setUrl] = useState('');
  const [code, setCode] = useState('');
  const [message, setMessage] = useState<{ kind: 'ok' | 'error'; text: string } | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () => getKyYard().then(setStatus).catch(() => setStatus(null));
  useEffect(() => { void load(); }, []);

  const pair = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setMessage(null);
    try {
      const s = await pairKyYard({ url: url.trim(), pairing_code: code.trim() });
      setStatus(s);
      setCode('');
      setMessage({ kind: 'ok', text: `Paired to ${s.organization ?? 'KyYard'}` });
      onChanged();
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not reach the server' });
    } finally {
      setBusy(false);
    }
  };
  const unpair = async () => {
    if (!window.confirm('Unpair from KyYard? This deletes the URL and token here. A KyYard administrator must also revoke the token on the Members page; unpairing here does not.')) return;
    setBusy(true);
    try {
      await unpairKyYard();
      await load();
      setMessage({ kind: 'ok', text: 'Unpaired. Ask a KyYard administrator to revoke the token as well.' });
      onChanged();
    } catch (err) {
      setMessage({ kind: 'error', text: err instanceof ApiError ? err.message : 'Could not reach the server' });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="panel">
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
        <Database size={20} style={{ color: 'var(--accent)' }} />
        <h3 style={{ fontSize: 16 }}>KyYard</h3>
      </div>
      {status?.paired ? (
        <>
          <div className="dr-facts">
            <div className="dr-fact"><span className="dr-fact-label">Organization</span><span className="dr-fact-value">{status.organization}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">URL</span><span className="dr-fact-value dr-mono">{status.url}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">Last pull</span><span className="dr-fact-value">{status.fetched_at ? `${timeLabel(status.fetched_at)} (${sinceLabel(status.fetched_at)} ago)` : 'never'}</span></div>
            <div className="dr-fact"><span className="dr-fact-label">State</span><span className={status.stale ? 'dr-fact-value dr-danger' : 'dr-fact-value dr-ok'}>{status.stale ? `stale${status.error ? ` (${status.error})` : ''}` : 'fresh'}</span></div>
          </div>
          {status.error === 'unauthorized' && <p className="form-error">KyYard refused the token: it was revoked. Unpair and pair again with a new code.</p>}
          <p className="dr-hint">Revoking the token happens in KyYard (Members → Service tokens); unpairing here only forgets it.</p>
          <div className="dr-actions"><button type="button" className="btn-danger" disabled={busy} onClick={unpair}><Unlink size={14} />Unpair</button></div>
        </>
      ) : (
        <form onSubmit={pair} aria-label="Pair KyYard">
          <p className="dr-hint">A KyYard administrator generates a six-digit code on the organization's Members page (Service tokens → Pair kyPulse). It is valid for 15 minutes.</p>
          <div className="form-grid">
            <label>KyYard URL<input type="url" value={url} onChange={(e) => setUrl(e.target.value)} required placeholder="https://kyyard.lan" /></label>
            <label>Pairing code<input inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(e) => setCode(e.target.value)} required autoComplete="one-time-code" /></label>
          </div>
          <div className="dr-actions"><button type="submit" disabled={busy}><Link2 size={14} />Pair</button></div>
        </form>
      )}
      {message && <p className={message.kind === 'ok' ? 'form-ok' : 'form-error'} role={message.kind === 'ok' ? 'status' : 'alert'}>{message.text}</p>}
    </div>
  );
};
```

Render it in `Settings.tsx` for admins beside the webhook form. Test `KyYardCard.test.tsx`: unpaired renders the form and posts `{url, pairing_code}`; paired with `error: 'unauthorized'` renders the revoked hint and the Unpair button; Unpair confirms and calls DELETE; the token never appears (stub returns none).

- [ ] **Step 4: Detail section**

```tsx
// web/src/components/KyYardFacts.tsx
import React from 'react';
import type { ContainerFacts } from '../monitor';
import { sinceLabel, timeLabel } from '../monitor';

const mib = (n: number) => `${Math.round(n / 1048576)} MiB`;

export const KyYardFacts: React.FC<{ facts: ContainerFacts | null; link?: string; paired: boolean; isAdmin: boolean }> = ({ facts, link, paired, isAdmin }) => {
  if (!link) {
    return <p className="dr-hint">Not linked to a KyYard container.{isAdmin ? ' Edit the app and pick one under Container.' : ''}</p>;
  }
  if (!paired) return <p className="dr-hint">KyYard is not paired.</p>;
  if (!facts) return <p className="dr-hint">Not seen in KyYard: <span className="dr-mono">{link}</span> is not in the latest inventory.</p>;
  return (
    <div className="dr-facts" aria-label="KyYard container">
      <div className="dr-fact"><span className="dr-fact-label">Container</span><span className="dr-fact-value dr-mono">{facts.name} on {facts.endpoint_name}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">State</span><span className={`dr-fact-value state-${facts.state === 'running' ? 'ok' : 'down'}`}>{facts.state}{facts.exit_code !== undefined ? ` (exit ${facts.exit_code})` : ''}</span><span className="dr-fact-note">{facts.status}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Docker health</span><span className="dr-fact-value">{facts.health}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Image</span><span className="dr-fact-value dr-mono">{facts.image}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Memory</span><span className="dr-fact-value">{facts.memory_limit > 0 ? `${mib(facts.memory_bytes)} of ${mib(facts.memory_limit)}` : mib(facts.memory_bytes)}</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Restarts</span><span className="dr-fact-value">{facts.restarts_last_hour} in the last hour</span><span className="dr-fact-note">{facts.restart_count} total</span></div>
      <div className="dr-fact"><span className="dr-fact-label">Observed</span><span className="dr-fact-value">{timeLabel(facts.observed_at)} ({sinceLabel(facts.observed_at)} ago){facts.stale ? ' · stale' : ''}</span></div>
    </div>
  );
};
```

In `AppDetail.tsx`: keep `kyyard` from the detail response in state; fetch `getKyYard()` once for `paired`; add a `dr-section` titled "KyYard" rendering `<KyYardFacts facts={facts} link={target.container} paired={paired} isAdmin={isAdmin} />` after the Checks/Settings pair. Tests in `AppDetail.test.tsx`: facts render name, health, exit code and "in the last hour"; a linked target with `kyyard: null` while paired renders "Not seen in KyYard"; an unlinked target renders the hint.

- [ ] **Step 5: Suggestions in the form**

`TargetForm.tsx`: a `Container` field (text input with a `<datalist>` of suggestions, label "KyYard container", placeholder `endpoint/name`, optional). Props gain `suggestions?: Suggestion[]`; choosing a suggestion (a small list of buttons "Use" under the field, or the datalist option) sets `container` and, when the name field is empty, the container's name. Submit sends `container` from the field (not `initial?.container`). `Status.tsx` and `AppDetail.tsx` fetch `kyYardContainers()` when the user is an admin and pass them (ignore a 403/412; empty list otherwise). Test in `Status.test.tsx`: with suggestions stubbed, clicking "Use" on `ep_1/kyvault` prefills name `kyvault` and container `ep_1/kyvault`, and the POST body carries `container`.

- [ ] **Step 6: Build, docs, commit**

Run: `cd web && npx vitest run && npm run build` and at the root `make ci`.

Docs: `web/AGENTS.md` (KyYard card on Settings for admins, stale line in the bar, facts section, suggestions prefill only), `README.md` "KyYard" section (both pairing steps, `KYPULSE_KYYARD_ALLOW_HTTP`, what is shown, stale semantics, unpair is two steps), `UI-VERIFICATION.md` (component coverage; no browser run against KyYard in CI).

```bash
git add -A
git commit -m "web: KyYard pairing card, stale bar line, container facts and suggestions; dist; docs"
```
