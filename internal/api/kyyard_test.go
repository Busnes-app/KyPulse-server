package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

// fakeYard answers by URL suffix; copied from internal/kyyard's own test fixture. token is
// exposed so a test can assert an audit row or a response never carries it. err, when set,
// is returned from every call instead of an answer -- a transport failure, standing in for
// what the real egress.Client would hand back from a dial or a read. blockFirst and release,
// when both set, make the very first call block: it closes blockFirst to report it has
// started, then waits on release -- standing in for a slow network round trip so a test can
// hold pullMu (via PullNow) while something else runs concurrently. blocked guards that this
// fires once via CompareAndSwap, not sync.Once, so a second, unrelated concurrent caller (the
// handler's own claim, on the same fake) is never made to wait for the first caller's release.
type fakeYard struct {
	answers map[string]struct {
		code int
		body any
	}
	err        error
	token      string
	blockFirst chan struct{}
	release    chan struct{}
	blocked    atomic.Bool
	mu         sync.Mutex
}

func (f *fakeYard) GetWith(_ context.Context, rawURL string, _ map[string]string) (*egress.Response, error) {
	return f.answer(rawURL)
}

func (f *fakeYard) Post(_ context.Context, rawURL, _ string, _ []byte, _ map[string]string) (*egress.Response, error) {
	return f.answer(rawURL)
}

func (f *fakeYard) answer(rawURL string) (*egress.Response, error) {
	if f.blockFirst != nil && f.blocked.CompareAndSwap(false, true) {
		close(f.blockFirst)
		<-f.release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
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

// serverWithYard is setupTestServerWith, except it also returns the *kyyard.Service: the
// server's own field is unexported, and some tests need to call PullNow directly (the
// background loop Kick would otherwise wake is never started in this harness) or seed an
// existing pairing ahead of an HTTP call.
func serverWithYard(t *testing.T, yardHTTP kyyard.HTTP) (*api.Server, store.Store, *kyyard.Service) {
	t.Helper()
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, _ := config.LoadFromEnv()
	db := testdb.Config(t)
	db.DataDir = cfg.Database.DataDir
	cfg.Database = db
	cfg.Captcha.Provider = "none"

	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	lg, err := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	mon := newTestMonitor(t, cfg, st, lg)
	yard := newTestKyYard(t, cfg, st, lg, yardHTTP)
	return api.NewServer(cfg, st, lg, mon, yard), st, yard
}

// monitorFixtureWithYard builds the server with a fake KyYard that answers a claim and the
// usual endpoint/inventory/samples fixture.
func monitorFixtureWithYard(t *testing.T) (*api.Server, store.Store, *http.Cookie, *http.Cookie, *fakeYard, *kyyard.Service) {
	t.Helper()
	fake := pairedYard()
	srv, st, yard := serverWithYard(t, fake)
	admin := loginAs(t, srv, st, "alice", "admin")
	viewer := loginAs(t, srv, st, "bob", "viewer")
	return srv, st, admin, viewer, fake, yard
}

func TestKyYardStatusUnpairedAndPairRefusals(t *testing.T) {
	srv, _, admin, viewer := monitorFixture(t)
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
	srv, st, admin, viewer, fake, yard := monitorFixtureWithYard(t)
	w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	if w.Code != http.StatusOK {
		t.Fatalf("pair: %d %s", w.Code, w.Body)
	}
	// The response does not wait for a pull: paired and stale immediately, no fetched_at yet.
	s := decodeMap(t, w)
	if s["paired"] != true || s["organization"] != "A" || s["stale"] != true {
		t.Fatalf("paired status: %v", s)
	}
	if _, has := s["fetched_at"]; has {
		t.Fatal("fetched_at present before the first pull")
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

	// This harness never starts the background loop Kick wakes, so the first pull is run
	// directly here in place of it.
	if err := yard.PullNow(context.Background()); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// Suggestions and status for the bar, now that a snapshot exists.
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

func TestKyYardPairReplacesAnExistingPairing(t *testing.T) {
	srv, st, admin, _, _, _ := monitorFixtureWithYard(t)
	first := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	if first.Code != http.StatusOK {
		t.Fatalf("first pair: %d %s", first.Code, first.Body)
	}
	second := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	if second.Code != http.StatusOK {
		t.Fatalf("second pair: %d %s", second.Code, second.Body)
	}
	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
	var successes []*store.AuditRecord
	for _, r := range rows {
		if r.Action == "admin.kyyard_pair" && strings.Contains(r.Details, "outcome=success") {
			successes = append(successes, r)
		}
	}
	// ListAuditRecords orders newest first: successes[0] is the second (replacing) pairing.
	if len(successes) != 2 {
		t.Fatalf("expected two successful pairing rows, got %d", len(successes))
	}
	if !strings.Contains(successes[0].Details, "replaced=true") {
		t.Fatalf("second (replacing) pairing must be audited replaced=true: %q", successes[0].Details)
	}
	if strings.Contains(successes[1].Details, "replaced=true") {
		t.Fatalf("first pairing must not be replaced: %q", successes[1].Details)
	}
}

// TestKyYardClaimErrorMapping pins the status, body and audit row for each way a claim can
// fail before a token is even issued.
func TestKyYardClaimErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		code       int
		body       any
		wantStatus int
		wantReason string // checked against the 502 body's "reason" field; ignored otherwise
	}{
		{"refused", 403, map[string]string{"error": "no"}, http.StatusForbidden, ""},
		{"rate_limited", 429, map[string]string{"error": "slow down"}, http.StatusTooManyRequests, ""},
		{"upstream_error", 500, nil, http.StatusBadGateway, "status_500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeYard{answers: map[string]struct {
				code int
				body any
			}{"/api/service-tokens/claim": {tc.code, tc.body}}}
			srv, st, _ := setupTestServerWith(t, nil, fake)
			admin := loginAs(t, srv, st, "alice", "admin")
			w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
			if w.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d body=%s", w.Code, tc.wantStatus, w.Body)
			}
			if tc.name == "refused" && !strings.Contains(strings.ToLower(w.Body.String()), "refused") {
				t.Fatalf("body must say refused: %s", w.Body)
			}
			if tc.wantStatus == http.StatusBadGateway {
				var body map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if len(body) != 2 {
					t.Fatalf("body must carry only error and reason: %v", body)
				}
				if body["reason"] != tc.wantReason {
					t.Fatalf("reason: got %q want %q", body["reason"], tc.wantReason)
				}
			}
			rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
			var found bool
			for _, r := range rows {
				if r.Action != "admin.kyyard_pair" {
					continue
				}
				found = true
				if !strings.Contains(r.Details, "outcome=failure") {
					t.Fatalf("details must record the failure: %q", r.Details)
				}
				if strings.Contains(r.Details, "123456") {
					t.Fatalf("audit leaks the pairing code: %q", r.Details)
				}
			}
			if !found {
				t.Fatal("no failure audit row")
			}
		})
	}
}

// TestKyYardPairTransportErrorIsBadGateway covers a network failure below the HTTP layer
// (the fake stands in for the real egress.Client's dial/read errors): 502, reason from the
// egress vocabulary, and an audited failure naming that reason.
func TestKyYardPairTransportErrorIsBadGateway(t *testing.T) {
	fake := &fakeYard{err: errors.New("dial tcp 10.0.0.5:443: connect: connection refused")}
	srv, st, _ := setupTestServerWith(t, nil, fake)
	admin := loginAs(t, srv, st, "alice", "admin")
	w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 2 || body["reason"] != "refused" {
		t.Fatalf("body: %v", body)
	}
	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
	var found bool
	for _, r := range rows {
		if r.Action == "admin.kyyard_pair" && strings.Contains(r.Details, "reason=refused") {
			found = true
		}
	}
	if !found {
		t.Fatal("no failure row naming the transport reason")
	}
}

// TestKyYardPairRefusesADialTimeAddress is the case ValidateURL's pre-flight check cannot
// catch: a name, not a literal IP, that only resolves to a refused address. The fake's error
// is wrapped in a *url.Error the way the real egress.Client's http.Client wraps a failed
// dial, so this also proves errors.Is unwraps through it.
func TestKyYardPairRefusesADialTimeAddress(t *testing.T) {
	fake := &fakeYard{err: &url.Error{Op: "Post", URL: "https://yard.lan/api/service-tokens/claim", Err: egress.ErrRefusedAddress}}
	srv, st, _ := setupTestServerWith(t, nil, fake)
	admin := loginAs(t, srv, st, "alice", "admin")
	w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}
	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
	var found bool
	for _, r := range rows {
		if r.Action == "admin.kyyard_pair" && strings.Contains(r.Details, "reason=address_refused") {
			found = true
		}
	}
	if !found {
		t.Fatal("no failure row naming address_refused")
	}
}

// failingSettings makes every write fail, to test a pairing whose claim succeeds but whose
// sealed row cannot be saved.
type failingSettings struct{ store.SettingsStore }

func (failingSettings) SetSetting(context.Context, string, string) error {
	return errors.New("disk full")
}

// serverWithFailingKyYardSave is setupTestServerWith, except the KyYard pairing's settings
// store refuses every write: the claim can still succeed, only Save cannot.
func serverWithFailingKyYardSave(t *testing.T, yardHTTP kyyard.HTTP) (*api.Server, store.Store) {
	t.Helper()
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, _ := config.LoadFromEnv()
	db := testdb.Config(t)
	db.DataDir = cfg.Database.DataDir
	cfg.Database = db
	cfg.Captcha.Provider = "none"

	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	lg, err := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	mon := newTestMonitor(t, cfg, st, lg)
	pairing, err := kyyard.NewPairing(cfg, failingSettings{st.Settings()})
	if err != nil {
		t.Fatalf("kyyard pairing: %v", err)
	}
	yard := &kyyard.Service{Pairing: pairing, HTTP: yardHTTP, Logger: lg}
	return api.NewServer(cfg, st, lg, mon, yard), st
}

func TestKyYardPairSaveFailureIsAuditedAndReported(t *testing.T) {
	fake := pairedYard()
	srv, st := serverWithFailingKyYardSave(t, fake)
	admin := loginAs(t, srv, st, "alice", "admin")
	w := doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}
	body := strings.ToLower(w.Body.String())
	if !strings.Contains(body, "kyyard") || !strings.Contains(body, "revoke") {
		t.Fatalf("500 must say the token must be revoked in KyYard: %s", w.Body)
	}
	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
	var found bool
	for _, r := range rows {
		if r.Action != "admin.kyyard_pair" {
			continue
		}
		found = true
		if !strings.Contains(r.Details, "outcome=failure") || !strings.Contains(r.Details, `host="yard.lan"`) ||
			!strings.Contains(r.Details, `org="org_a"`) || !strings.Contains(r.Details, "reason=save") {
			t.Fatalf("details: %q", r.Details)
		}
		if strings.Contains(r.Details, fake.token) || strings.Contains(r.Details, "123456") {
			t.Fatalf("audit leaks a secret: %q", r.Details)
		}
	}
	if !found {
		t.Fatal("no failure audit row")
	}
}

// TestKyYardPairDoesNotBlockOnALoopPull is the regression this round fixes: handleKyYardPair
// used to call PullNow inline, which single-flights on the service's own mutex, so a pair
// landing while the background loop was mid-pull (one HTTP round trip per endpoint) would
// queue behind it and could stall past the listener's write timeout. It no longer calls
// PullNow at all -- Clear, Save, Adopt and a non-blocking Kick -- so the request must return
// promptly even while a pull is genuinely in flight and holding pullMu.
func TestKyYardPairDoesNotBlockOnALoopPull(t *testing.T) {
	fake := pairedYard()
	started := make(chan struct{})
	release := make(chan struct{})
	fake.blockFirst = started
	fake.release = release

	srv, st, yard := serverWithYard(t, fake)
	admin := loginAs(t, srv, st, "alice", "admin")

	// Seed an existing pairing and simulate the loop's own pull already in flight, blocked on
	// the network and holding pullMu.
	if err := yard.Pairing.Save(context.Background(), kyyard.Config{URL: "https://yard.lan", Token: "old-token", OrganizationID: "org_a", OrganizationName: "A"}); err != nil {
		t.Fatalf("seed pairing: %v", err)
	}
	pullDone := make(chan struct{})
	go func() {
		defer close(pullDone)
		_ = yard.PullNow(context.Background())
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("the simulated loop pull never started")
	}

	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- doJSON(t, srv, "POST", "/api/kyyard/pair", admin, map[string]any{"url": "https://yard.lan", "pairing_code": "123456"})
	}()
	select {
	case w := <-result:
		if w.Code != http.StatusOK {
			t.Fatalf("pair: %d %s", w.Code, w.Body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pair request blocked behind the loop's in-flight pull")
	}

	close(release)
	select {
	case <-pullDone:
	case <-time.After(2 * time.Second):
		t.Fatal("the simulated loop pull never finished after release")
	}
}
