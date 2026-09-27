# kyPulse monitoring backend (step 2b) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** kyPulse polls every watched app's health URL, decides `ok`/`degraded`/`down` with a state machine, records every change, sends one webhook per change plus hourly reminders, and exposes admin and viewer API routes for all of it — with no UI yet.

**Architecture:** Five new packages with one responsibility each, wired by a sixth. `egress` is the only outbound HTTP client (LAN allowed, loopback/link-local refused at dial time, no redirects, capped bodies). `poller` normalises responses and runs polls on a bounded worker pool. `alerts` is the pure state machine. `notify` renders and delivers the four webhook shapes with retries. `monitor` glues them to the store: it decides what is due, persists each observation, records transitions, and sends. The store gains two tables; the API gains target, alert, status and webhook routes; `cmd/server` starts and drains the loop.

**Tech Stack:** Go 1.26, `net/http`, `database/sql` over SQLite/Postgres via the existing `internal/store`, ky-primitives v0.9.0 (`health` types, `logging`, `recoveryclient.NewAESGCMSealer` for the sealed webhook).

**Spec:** `docs/superpowers/specs/2026-09-26-kypulse-design.md`, section 2 "kyPulse server": Packages, Outbound network guard, Watched apps and polling, State machine, Webhook. Section 1 "Reading responses in kyPulse" is the normaliser.

**Repository:** `/home/yoshi/git/busnes.app/kyPulse-server`, branch `feat/monitoring-backend` off `master` (`d903944`); this plan is its first commit. Paths are relative to the repo root.

**Provenance:** the code blocks in Tasks 1–4 were written in a scratch clone of `master`, and passed `gofmt`, `go vet` and `go test -race -count=10` there. Tasks 5–9 give exact signatures and edits but their code was not pre-compiled; the task reviewer and `go test ./...` are the gate.

**Deferred to a later plan (2d):** kyPulse's own audit trail through `auditchain` (spec "Own logs"). The scaffold's `audit_records` table keeps serving; this plan writes audit rows to it.

## Global Constraints

- Outbound guard (spec): addresses checked at connect time; private and LAN allowed; loopback, link-local (cloud metadata included), unspecified, multicast and reserved refused always; redirects refused; health targets may be plain `http://`; a webhook may be plain HTTP only with `KYPULSE_ALERT_ALLOW_HTTP=true` (off by default, and enabling it is audited); response bodies capped at 64 KiB; request timeout 5 s.
- Normaliser (spec section 1): `ky.health/1` as-is; a 2xx JSON `status` of `ok|alive|ready|healthy` → ok, `degraded` → degraded, boolean `healthy` → ok/degraded; any other 2xx → ok shown as `basic`; non-2xx, timeout, TLS, DNS or refused → down with the specific cause recorded.
- Polling: interval default 30 s, minimum 10 s; a bounded worker pool; KyYard suggestions only prefill (not in this plan); stored per target: the last normalised result plus the list of state transitions, never every poll.
- State machine: to `down` after 3 consecutive down polls; to `degraded` after 2; to `ok` after 2; every transition is one alert and one webhook; a reminder every hour while not ok; silence (1 h, 8 h, until fixed) stops the webhook only and "until fixed" ends on the next transition to ok.
- Webhook: presets ntfy (text body, Title/Priority headers), Gotify (`X-Gotify-Key` header, never in the URL), Discord (JSON `content`), generic (`{"app","state","previous","reason","time","url"}`); the URL and token are sealed at rest under the deployment key; the token is write-only in the UI/API; messages never carry log lines, user names or IPs; 3 retries with backoff; continued failure is surfaced (alert bar) and every send and failure is audited.
- Roles: viewers read status, targets and alerts; admins do everything else; every route enforces this server-side.
- Every process line is JSON on stderr through ky-primitives `logging`, with declared events; no `log.Printf` in new packages.
- `web/dist` is untouched by this plan (no UI); Playwright still passes.

## Review Focus

1. **A health URL that resolves to 127.0.0.1 or 169.254.169.254 via DNS** (a rebind, or a mis-set hosts file). Expect the poll to be refused at dial time and recorded as down with cause `address_refused`, never a request to the metadata service. Pinned by `TestDialRefusesANameThatResolvesToLoopback` (Task 1).
2. **An app that flaps once between polls.** Expect no alert and no state change until the run of three. Pinned by `TestTransitions` "one failure changes nothing" and "mixed bad polls never reach a threshold" (Task 3).
3. **A webhook receiver that is down for an hour.** Expect retries, then the failure recorded and shown, then reminders continue to be attempted hourly; expect no unbounded goroutines. Pinned by `TestSendRetriesTransportAnd5xxButNot4xx` (Task 4) and `TestObserveRecordsAFailedSend` (Task 7).
4. **An admin pastes a Gotify token, then reads the webhook config back.** Expect `has_token: true` and never the token string, in `/api/alerts/webhook` and in `/api/settings`. Pinned by `TestWebhookTokenIsWriteOnly` (Task 8).
5. **SIGTERM while a poll is in flight.** Expect the poll to finish or be cancelled before the store closes, never a write into a closed store. Pinned by `TestRunStopsWithContextAfterDrainingPolls` (Task 2) and the wiring in Task 9.

---

### Task 1: `internal/egress` — the outbound guard

**Files:**
- Create: `internal/egress/egress.go`, `internal/egress/egress_test.go`
- Create: `internal/egress/AGENTS.md`

**Interfaces:**
- Consumes: nothing in the repo.
- Produces: `egress.New(Options) *Client`, `(*Client).Get(ctx, url) (*Response, error)`, `(*Client).Post(ctx, url, contentType string, body []byte, headers map[string]string) (*Response, error)`, `egress.ValidateURL(raw string, allowHTTP bool) error`, `egress.Cause(err) string` (vocabulary: `address_refused`, `redirect`, `body_too_large`, `bad_url`, `timeout`, `dns`, `tls`, `refused`, `network`), `Response{StatusCode, Header, Body}`, errors `ErrRefusedAddress`, `ErrRedirect`, `ErrBodyTooLarge`, `ErrScheme`.

- [ ] **Step 1: Confirm the branch**

Run: `git branch --show-current`
Expected: `feat/monitoring-backend`.

- [ ] **Step 2: Write the failing tests** at `internal/egress/egress_test.go`

```go
package egress

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testClient resolves every name to fakeIP and dials the httptest server instead, so the
// guard runs against a LAN-looking address while the bytes go to a loopback listener the
// guard would otherwise refuse.
func testClient(t *testing.T, srv *httptest.Server, fakeIP string, o Options) *Client {
	t.Helper()
	lookup := func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP(fakeIP)}, nil }
	dial := func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	return newClient(o, lookup, dial)
}

func TestValidateURL(t *testing.T) {
	cases := []struct {
		url       string
		allowHTTP bool
		want      error
	}{
		{"https://vault.lan/healthz", false, nil},
		{"https://10.0.0.5:8443/healthz", false, nil},
		{"https://100.64.1.2/healthz", false, nil},
		{"http://vault.lan/healthz", false, ErrScheme},
		{"http://vault.lan/healthz", true, nil},
		{"https://user:pw@vault.lan/", false, ErrScheme},
		{"https:///healthz", false, ErrScheme},
		{"ftp://vault.lan/", true, ErrScheme},
		{"https://127.0.0.1/healthz", false, ErrRefusedAddress},
		{"https://[::1]/healthz", false, ErrRefusedAddress},
		{"http://169.254.169.254/latest/meta-data", true, ErrRefusedAddress},
		{"https://0.0.0.0/", false, ErrRefusedAddress},
		{"https://224.0.0.1/", false, ErrRefusedAddress},
		{"https://[fe80::1]/", false, ErrRefusedAddress},
		{"https://198.18.0.1/", false, ErrRefusedAddress},
	}
	for _, tc := range cases {
		if got := ValidateURL(tc.url, tc.allowHTTP); !errors.Is(got, tc.want) {
			t.Errorf("ValidateURL(%q, %v) = %v, want %v", tc.url, tc.allowHTTP, got, tc.want)
		}
	}
}

func TestDialRefusesANameThatResolvesToLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	for _, ip := range []string{"127.0.0.1", "::1", "169.254.169.254", "0.0.0.0"} {
		c := testClient(t, srv, ip, Options{AllowHTTP: true})
		_, err := c.Get(context.Background(), "http://rebound.test/healthz")
		if !errors.Is(err, ErrRefusedAddress) {
			t.Errorf("%s: err = %v, want ErrRefusedAddress", ip, err)
		}
		if Cause(err) != "address_refused" {
			t.Errorf("%s: cause = %q", ip, Cause(err))
		}
	}
}

func TestGetReturnsTheBodyFromALANAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "kypulse" {
			t.Errorf("user agent = %q", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"status":"down"}`))
	}))
	defer srv.Close()
	c := testClient(t, srv, "10.0.0.5", Options{AllowHTTP: true})
	resp, err := c.Get(context.Background(), "http://vault.lan/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 503 || string(resp.Body) != `{"status":"down"}` {
		t.Fatalf("resp = %d %q", resp.StatusCode, resp.Body)
	}
}

func TestRedirectIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://evil.test/", http.StatusFound)
	}))
	defer srv.Close()
	c := testClient(t, srv, "10.0.0.5", Options{AllowHTTP: true})
	_, err := c.Get(context.Background(), "http://vault.lan/healthz")
	if !errors.Is(err, ErrRedirect) || Cause(err) != "redirect" {
		t.Fatalf("err = %v, cause = %q", err, Cause(err))
	}
}

func TestBodyOverTheCapIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer srv.Close()
	c := testClient(t, srv, "10.0.0.5", Options{AllowHTTP: true, MaxBody: 99})
	_, err := c.Get(context.Background(), "http://vault.lan/healthz")
	if !errors.Is(err, ErrBodyTooLarge) || Cause(err) != "body_too_large" {
		t.Fatalf("err = %v", err)
	}
	c = testClient(t, srv, "10.0.0.5", Options{AllowHTTP: true, MaxBody: 100})
	if _, err := c.Get(context.Background(), "http://vault.lan/healthz"); err != nil {
		t.Fatalf("exactly at the cap: %v", err)
	}
}

func TestTimeoutIsNamed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	c := testClient(t, srv, "10.0.0.5", Options{AllowHTTP: true, Timeout: 50 * time.Millisecond})
	_, err := c.Get(context.Background(), "http://vault.lan/healthz")
	if err == nil || Cause(err) != "timeout" {
		t.Fatalf("err = %v, cause = %q", err, Cause(err))
	}
}

func TestPostSendsHeadersAndBody(t *testing.T) {
	var gotType, gotKey, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.Header.Get("Content-Type")
		gotKey = r.Header.Get("X-Gotify-Key")
		b := make([]byte, 64)
		n, _ := r.Body.Read(b)
		gotBody = string(b[:n])
	}))
	defer srv.Close()
	c := testClient(t, srv, "10.0.0.5", Options{AllowHTTP: true})
	_, err := c.Post(context.Background(), "http://ntfy.lan/topic", "text/plain", []byte("hi"), map[string]string{"X-Gotify-Key": "k"})
	if err != nil {
		t.Fatal(err)
	}
	if gotType != "text/plain" || gotKey != "k" || gotBody != "hi" {
		t.Fatalf("got %q %q %q", gotType, gotKey, gotBody)
	}
}

func TestCauseVocabulary(t *testing.T) {
	if got := Cause(&net.DNSError{IsNotFound: true}); got != "dns" {
		t.Errorf("dns: %q", got)
	}
	if got := Cause(errors.New("dial tcp 10.0.0.5:443: connect: connection refused")); got != "refused" {
		t.Errorf("refused: %q", got)
	}
	if got := Cause(errors.New("remote error: tls: handshake failure")); got != "tls" {
		t.Errorf("tls: %q", got)
	}
	if got := Cause(errors.New("something else")); got != "network" {
		t.Errorf("network: %q", got)
	}
}
```

- [ ] **Step 3: Run them to verify they fail**

Run: `go test ./internal/egress/`
Expected: FAIL, build error `undefined: newClient` (and `Options`, `ValidateURL`, ...).

- [ ] **Step 4: Write the implementation** at `internal/egress/egress.go`

```go
// Package egress is the one outbound HTTP client: health polls, KyYard and webhooks all go
// through it. Private and LAN addresses are allowed, because that is where the apps live;
// loopback, link-local (cloud metadata included), unspecified, multicast and reserved ranges
// are refused at dial time, so a DNS rebind cannot slip past a check made at resolution.
// Redirects are refused and response bodies are capped.
package egress

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrRefusedAddress = errors.New("egress: destination resolves only to loopback, link-local or reserved addresses")
	ErrRedirect       = errors.New("egress: redirects are refused")
	ErrBodyTooLarge   = errors.New("egress: response body over the cap")
	ErrScheme         = errors.New("egress: URL must be https (or http where explicitly allowed), with a host and no credentials")
)

// Options configures a Client. Zero values mean the defaults below.
type Options struct {
	AllowHTTP bool          // plain http:// URLs; off for webhooks and KyYard, on for health targets
	Timeout   time.Duration // whole request; default 5s
	MaxBody   int64         // bytes read from a response; default 64 KiB
}

const (
	DefaultTimeout = 5 * time.Second
	DefaultMaxBody = 64 << 10
)

// Response is what a caller gets: the body is already read and capped.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

type lookupFunc func(ctx context.Context, network, host string) ([]net.IP, error)
type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Client is safe for concurrent use.
type Client struct {
	http      *http.Client
	allowHTTP bool
	maxBody   int64
}

// New builds a Client with the system resolver.
func New(o Options) *Client {
	return newClient(o, net.DefaultResolver.LookupIP, (&net.Dialer{Timeout: 10 * time.Second}).DialContext)
}

func newClient(o Options, lookup lookupFunc, dial dialFunc) *Client {
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.MaxBody <= 0 {
		o.MaxBody = DefaultMaxBody
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := lookup(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if allowedIP(ip) {
					return dial(ctx, network, net.JoinHostPort(ip.String(), port))
				}
			}
			return nil, ErrRefusedAddress
		},
	}
	return &Client{
		http:      &http.Client{Timeout: o.Timeout, Transport: transport, CheckRedirect: refuseRedirect},
		allowHTTP: o.AllowHTTP,
		maxBody:   o.MaxBody,
	}
}

func refuseRedirect(*http.Request, []*http.Request) error { return ErrRedirect }

// ValidateURL is the check a URL gets when an admin saves it: scheme, host, no credentials,
// and a literal IP is judged the same way the dialer will judge a resolved one.
func ValidateURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil {
		return ErrScheme
	}
	if u.Scheme != "https" && !(allowHTTP && u.Scheme == "http") {
		return ErrScheme
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !allowedIP(ip) {
		return ErrRefusedAddress
	}
	return nil
}

// Get fetches url and returns the capped body. A body over the cap is ErrBodyTooLarge, not a
// truncated success: the caller must never parse half a document as a health report.
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	return c.do(ctx, http.MethodGet, rawURL, "", nil, nil)
}

// Post sends body with the given content type and extra headers.
func (c *Client) Post(ctx context.Context, rawURL, contentType string, body []byte, headers map[string]string) (*Response, error) {
	return c.do(ctx, http.MethodPost, rawURL, contentType, body, headers)
}

func (c *Client) do(ctx context.Context, method, rawURL, contentType string, body []byte, headers map[string]string) (*Response, error) {
	if err := ValidateURL(rawURL, c.allowHTTP); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kypulse")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > c.maxBody {
		return nil, ErrBodyTooLarge
	}
	return &Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// Cause names why a request failed, in a fixed vocabulary the UI can show and tests can pin.
func Cause(err error) string {
	var netErr net.Error
	var dnsErr *net.DNSError
	var tlsRecord tls.RecordHeaderError
	var certErr *tls.CertificateVerificationError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrRefusedAddress):
		return "address_refused"
	case errors.Is(err, ErrRedirect):
		return "redirect"
	case errors.Is(err, ErrBodyTooLarge):
		return "body_too_large"
	case errors.Is(err, ErrScheme):
		return "bad_url"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
		return "timeout"
	case errors.As(err, &dnsErr):
		return "dns"
	case errors.As(err, &certErr), errors.As(err, &tlsRecord), strings.Contains(err.Error(), "tls:"):
		return "tls"
	case strings.Contains(err.Error(), "connection refused"):
		return "refused"
	}
	return "network"
}

var reservedRanges = mustCIDRs("0.0.0.0/8", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4", "64:ff9b::/96", "fec0::/10")

// allowedIP admits private (RFC 1918, ULA) and CGNAT addresses and refuses everything that
// can only mean this host, this link, or nowhere.
func allowedIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	for _, n := range reservedRanges {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(fmt.Sprintf("egress: bad CIDR %q: %v", c, err))
		}
		out = append(out, n)
	}
	return out
}
```

- [ ] **Step 5: Run the tests**

Run: `gofmt -l internal/egress; go vet ./internal/egress/ && go test -race -count=5 ./internal/egress/`
Expected: nothing from gofmt, `ok`.

- [ ] **Step 6: Write `internal/egress/AGENTS.md`**

```markdown
# Egress

## Purpose
The one outbound HTTP client: health polls, KyYard and webhooks. Private and LAN destinations are allowed; loopback, link-local (cloud metadata), unspecified, multicast and reserved ranges are refused at dial time; redirects are refused; bodies are capped.

## Ownership
Owns `Client`, `ValidateURL`, `Cause` and the address policy. No caller builds its own `http.Client`.

## Local Contracts
- `ValidateURL` is the save-time check; the dialer repeats the address check on every resolved IP, so a DNS rebind cannot pass.
- `AllowHTTP` is per client: on for health targets, off for webhooks unless `KYPULSE_ALERT_ALLOW_HTTP`, always off for KyYard.
- A body over `MaxBody` is `ErrBodyTooLarge`, never a truncated success.
- `Cause` is the fixed vocabulary the UI shows and tests pin.

## Verification
- `go test -race ./internal/egress/`

## Child DOX Index
None.
```

- [ ] **Step 7: Commit**

```bash
git add internal/egress
git commit -m "egress: guarded outbound HTTP client"
```

---

### Task 2: `internal/poller` — normaliser and poll loop

**Files:**
- Create: `internal/poller/normalize.go`, `internal/poller/normalize_test.go`, `internal/poller/poller.go`, `internal/poller/poller_test.go`, `internal/poller/AGENTS.md`

**Interfaces:**
- Consumes: `egress.Response`, `egress.Cause`, `health.Schema`, `health.CheckResult`.
- Produces: `poller.State` (`OK`, `Degraded`, `Down`), `poller.Result{State, Basic, Cause, Service, Checks}`, `poller.Normalize(resp *egress.Response, err error) Result`, `poller.Target{ID, Name, URL, Interval}`, `poller.Observation{Target, Result, At, Latency}`, `poller.Getter` interface, `poller.Poller{Get, Workers, Due, Observe}` with `Run(ctx, tick)` (drains in-flight polls before returning) and `Tick(ctx, now)`.

- [ ] **Step 1: Write the failing normaliser tests** at `internal/poller/normalize_test.go`

```go
package poller

import (
	"errors"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

func resp(code int, body string) *egress.Response {
	return &egress.Response{StatusCode: code, Body: []byte(body)}
}

// The fixtures are the real answers of the suite's apps on 2026-09-26.
func TestNormalizeAgainstTodaysApps(t *testing.T) {
	cases := []struct {
		name string
		resp *egress.Response
		err  error
		want Result
	}{
		{"ky.health/1 degraded", resp(200, `{"schema":"ky.health/1","service":"kyvault","status":"degraded","time":"2026-09-26T10:41:00Z","checks":[{"name":"database","status":"ok"},{"name":"audit","status":"degraded","reason":"append_disabled"}]}`), nil,
			Result{State: Degraded, Service: "kyvault"}},
		{"ky.health/1 down 503", resp(503, `{"schema":"ky.health/1","service":"kyvault","status":"down","checks":[{"name":"database","status":"down"}]}`), nil,
			Result{State: Down, Cause: "status_503"}},
		{"ky.health/1 unknown status", resp(200, `{"schema":"ky.health/1","service":"x","status":"weird"}`), nil,
			Result{State: Down, Cause: "bad_health_document"}},
		{"KyIdentity healthz alive", resp(200, `{"status":"alive"}`), nil, Result{State: OK}},
		{"kynotes readyz", resp(200, `{"status":"ready"}`), nil, Result{State: OK}},
		{"kybookmarks always-200 degraded", resp(200, `{"status":"degraded","service":"kymark-server","time":"2026-09-26T10:00:00Z"}`), nil,
			Result{State: Degraded, Service: "kymark-server"}},
		{"KyVault ok", resp(200, `{"status":"ok","service":"kyvault-server","time":"x"}`), nil, Result{State: OK, Service: "kyvault-server"}},
		{"kypost healthy true", resp(200, `{"healthy":true,"unhealthyForSeconds":0}`), nil, Result{State: OK}},
		{"kypost healthy false", resp(200, `{"healthy":false,"failureReason":["imap"]}`), nil, Result{State: Degraded}},
		{"kypost 503 when unhealthy", resp(503, `{"healthy":false}`), nil, Result{State: Down, Cause: "status_503"}},
		{"kydns plain ok", resp(200, "ok"), nil, Result{State: OK, Basic: true}},
		{"KyYard live", resp(200, `{"status":"ok"}`), nil, Result{State: OK}},
		{"empty 204", resp(204, ""), nil, Result{State: OK, Basic: true}},
		{"JSON without a status", resp(200, `{"uptime":42}`), nil, Result{State: OK, Basic: true}},
		{"status is not a string", resp(200, `{"status":{"db":"ok"}}`), nil, Result{State: OK, Basic: true}},
		{"404", resp(404, "not found"), nil, Result{State: Down, Cause: "status_404"}},
		{"redirect status 301", resp(301, ""), nil, Result{State: Down, Cause: "status_301"}},
		{"transport error", nil, egress.ErrRefusedAddress, Result{State: Down, Cause: "address_refused"}},
		{"plain error", nil, errors.New("dial tcp: connection refused"), Result{State: Down, Cause: "refused"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(tc.resp, tc.err)
			got.Checks = nil // compared separately below
			if got.State != tc.want.State || got.Basic != tc.want.Basic || got.Cause != tc.want.Cause || got.Service != tc.want.Service {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestNormalizeKeepsTheChecksOfAHealthDocument(t *testing.T) {
	got := Normalize(resp(200, `{"schema":"ky.health/1","service":"kyvault","status":"ok","checks":[{"name":"database","status":"ok"},{"name":"audit","status":"degraded","reason":"append_disabled"}]}`), nil)
	if len(got.Checks) != 2 || got.Checks[1].Reason != "append_disabled" {
		t.Fatalf("checks = %+v", got.Checks)
	}
	if got := Normalize(resp(200, `{"status":"ok"}`), nil); got.Checks != nil {
		t.Fatalf("a bare status must not invent checks: %+v", got.Checks)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/poller/`
Expected: FAIL, `undefined: Normalize`.

- [ ] **Step 3: Write the normaliser** at `internal/poller/normalize.go`

```go
// Package poller checks every watched app's health URL on its interval and turns whatever
// comes back into one of three states. The rules for reading a response live in Normalize
// and nowhere else.
package poller

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Busnes-app/ky-primitives/health"
	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// State is what a poll concluded. It is the alerts package's input.
type State string

const (
	OK       State = "ok"
	Degraded State = "degraded"
	Down     State = "down"
)

// Result is one poll, normalised. Basic marks an app that answered 2xx without a health
// document, so the screen can say "basic health, no checks". Cause names a down: a transport
// failure from egress.Cause, or status_<code> for a non-2xx answer.
type Result struct {
	State   State                `json:"state"`
	Basic   bool                 `json:"basic,omitempty"`
	Cause   string               `json:"cause,omitempty"`
	Service string               `json:"service,omitempty"`
	Checks  []health.CheckResult `json:"checks,omitempty"`
}

// Normalize applies the spec's four rules in order: a ky.health/1 document, a known status
// field, any other 2xx, and everything else is down.
func Normalize(resp *egress.Response, err error) Result {
	if err != nil {
		return Result{State: Down, Cause: egress.Cause(err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{State: Down, Cause: "status_" + strconv.Itoa(resp.StatusCode)}
	}
	var doc struct {
		Schema  string               `json:"schema"`
		Service string               `json:"service"`
		Status  json.RawMessage      `json:"status"`
		Healthy *bool                `json:"healthy"`
		Checks  []health.CheckResult `json:"checks"`
	}
	if json.Unmarshal(resp.Body, &doc) != nil {
		return Result{State: OK, Basic: true}
	}
	if doc.Schema == health.Schema {
		var s string
		_ = json.Unmarshal(doc.Status, &s)
		switch State(s) {
		case OK, Degraded, Down:
			return Result{State: State(s), Service: doc.Service, Checks: doc.Checks}
		}
		return Result{State: Down, Cause: "bad_health_document"}
	}
	var s string
	if json.Unmarshal(doc.Status, &s) == nil {
		switch strings.ToLower(s) {
		case "ok", "alive", "ready", "healthy":
			return Result{State: OK, Service: doc.Service}
		case "degraded":
			return Result{State: Degraded, Service: doc.Service}
		}
	}
	if doc.Healthy != nil {
		if *doc.Healthy {
			return Result{State: OK}
		}
		return Result{State: Degraded}
	}
	return Result{State: OK, Basic: true}
}
```

- [ ] **Step 4: Run the normaliser tests**

Run: `go test ./internal/poller/`
Expected: PASS.

- [ ] **Step 5: Write the failing loop tests** at `internal/poller/poller_test.go`

```go
package poller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

type fakeGetter struct {
	mu        sync.Mutex
	calls     map[string]int
	release   chan struct{} // when non-nil, Get blocks until closed
	ignoreCtx bool          // a Get that does not honour cancellation, like a stuck dial
}

func (g *fakeGetter) Get(ctx context.Context, url string) (*egress.Response, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = map[string]int{}
	}
	g.calls[url]++
	g.mu.Unlock()
	if g.release != nil {
		if g.ignoreCtx {
			<-g.release
		} else {
			select {
			case <-g.release:
			case <-ctx.Done():
			}
		}
	}
	if url == "http://bad.lan/healthz" {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &egress.Response{StatusCode: 200, Body: []byte(`{"status":"ok"}`)}, nil
}

func (g *fakeGetter) count(url string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[url]
}

func TestTickPollsEveryDueTargetAndObserves(t *testing.T) {
	g := &fakeGetter{}
	obs := make(chan Observation, 4)
	p := &Poller{Get: g, Workers: 2,
		Due: func(context.Context, time.Time) ([]Target, error) {
			return []Target{{ID: "a", URL: "http://good.lan/healthz"}, {ID: "b", URL: "http://bad.lan/healthz"}}, nil
		},
		Observe: func(_ context.Context, o Observation) { obs <- o },
	}
	p.Tick(context.Background(), time.Now())
	got := map[string]Result{}
	for range 2 {
		select {
		case o := <-obs:
			got[o.Target.ID] = o.Result
		case <-time.After(2 * time.Second):
			t.Fatal("observations did not arrive")
		}
	}
	if got["a"].State != OK || got["b"].State != Down || got["b"].Cause != "refused" {
		t.Fatalf("got %+v", got)
	}
}

func TestInFlightTargetIsNotPolledAgain(t *testing.T) {
	g := &fakeGetter{release: make(chan struct{})}
	p := &Poller{Get: g, Workers: 4,
		Due: func(context.Context, time.Time) ([]Target, error) {
			return []Target{{ID: "a", URL: "http://slow.lan/healthz"}}, nil
		},
		Observe: func(context.Context, Observation) {},
	}
	p.Tick(context.Background(), time.Now())
	p.Tick(context.Background(), time.Now())
	p.Tick(context.Background(), time.Now())
	time.Sleep(20 * time.Millisecond)
	if n := g.count("http://slow.lan/healthz"); n != 1 {
		t.Fatalf("slow target polled %d times while in flight, want 1", n)
	}
	close(g.release)
}

func TestFullPoolLeavesTargetsForTheNextTick(t *testing.T) {
	g := &fakeGetter{release: make(chan struct{})}
	obs := make(chan Observation, 8)
	var mu sync.Mutex
	due := []Target{{ID: "a", URL: "http://a.lan/healthz"}, {ID: "b", URL: "http://b.lan/healthz"}}
	p := &Poller{Get: g, Workers: 1,
		Due: func(context.Context, time.Time) ([]Target, error) {
			mu.Lock()
			defer mu.Unlock()
			return append([]Target(nil), due...), nil
		},
		Observe: func(_ context.Context, o Observation) { obs <- o },
	}
	p.Tick(context.Background(), time.Now())
	time.Sleep(20 * time.Millisecond)
	if g.count("http://a.lan/healthz") != 1 || g.count("http://b.lan/healthz") != 0 {
		t.Fatalf("one worker must seat exactly one target: a=%d b=%d", g.count("http://a.lan/healthz"), g.count("http://b.lan/healthz"))
	}
	close(g.release)
	<-obs
	mu.Lock()
	due = due[1:] // a has been polled; only b is due now
	mu.Unlock()
	// The slot is released just after Observe returns; tick until b is seated.
	deadline := time.Now().Add(2 * time.Second)
	for g.count("http://b.lan/healthz") == 0 && time.Now().Before(deadline) {
		p.Tick(context.Background(), time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case o := <-obs:
		if o.Target.ID != "b" {
			t.Fatalf("unexpected %+v", o)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("next tick did not poll b")
	}
	if g.count("http://b.lan/healthz") != 1 || g.count("http://a.lan/healthz") != 1 {
		t.Fatalf("counts a=%d b=%d", g.count("http://a.lan/healthz"), g.count("http://b.lan/healthz"))
	}
}

func TestRunStopsWithContextAfterDrainingPolls(t *testing.T) {
	g := &fakeGetter{release: make(chan struct{}), ignoreCtx: true}
	observed := make(chan struct{}, 1)
	p := &Poller{Get: g, Due: func(context.Context, time.Time) ([]Target, error) {
		return []Target{{ID: "a", URL: "http://a.lan/healthz"}}, nil
	}, Observe: func(context.Context, Observation) { observed <- struct{}{} }}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx, time.Millisecond); close(done) }()
	for g.count("http://a.lan/healthz") == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
		t.Fatal("Run returned while a poll was in flight")
	case <-time.After(50 * time.Millisecond):
	}
	close(g.release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after the poll finished")
	}
	select {
	case <-observed:
	default:
		t.Fatal("the in-flight poll was not observed before Run returned")
	}
}
```

- [ ] **Step 6: Run them to verify they fail**

Run: `go test ./internal/poller/`
Expected: FAIL, `undefined: Poller`.

- [ ] **Step 7: Write the loop** at `internal/poller/poller.go`

```go
package poller

import (
	"context"
	"sync"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// Target is what the poller needs to know about a watched app.
type Target struct {
	ID       string
	Name     string
	URL      string
	Interval time.Duration
}

// Observation is one completed poll, handed to Observe.
type Observation struct {
	Target  Target
	Result  Result
	At      time.Time
	Latency time.Duration
}

// Getter is the slice of egress.Client the poller uses; a fake stands in for tests.
type Getter interface {
	Get(ctx context.Context, url string) (*egress.Response, error)
}

// Poller runs polls on a bounded worker pool, so one slow app never delays the others.
// Due and Observe are the store's: the poller owns no persistence.
type Poller struct {
	Get     Getter
	Workers int
	Due     func(ctx context.Context, now time.Time) ([]Target, error)
	Observe func(ctx context.Context, o Observation)

	once     sync.Once
	slots    chan struct{}
	inflight sync.Map // target ID -> struct{}
	wg       sync.WaitGroup
}

func (p *Poller) init() {
	p.once.Do(func() {
		if p.Workers <= 0 {
			p.Workers = 4
		}
		p.slots = make(chan struct{}, p.Workers)
	})
}

// Run ticks until ctx ends, then waits for the polls in flight, so the caller can close the
// store after Run returns. A tick dispatches every due target it can seat; the rest wait for
// the next tick rather than queueing, so a burst of slow apps degrades to late polls, never
// to unbounded goroutines.
func (p *Poller) Run(ctx context.Context, tick time.Duration) {
	p.init()
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.wg.Wait()
			return
		case now := <-t.C:
			p.Tick(ctx, now)
		}
	}
}

// Tick dispatches one round and returns without waiting for the polls to finish. A target
// already in flight is skipped, so a poll that outlives its interval cannot stack.
func (p *Poller) Tick(ctx context.Context, now time.Time) {
	p.init()
	due, err := p.Due(ctx, now)
	if err != nil {
		return
	}
	for _, tg := range due {
		if _, busy := p.inflight.LoadOrStore(tg.ID, struct{}{}); busy {
			continue
		}
		select {
		case p.slots <- struct{}{}:
		default:
			p.inflight.Delete(tg.ID)
			return // every worker is busy; the rest keep their place until the next tick
		}
		p.wg.Add(1)
		go p.poll(ctx, tg)
	}
}

func (p *Poller) poll(ctx context.Context, tg Target) {
	defer func() { <-p.slots; p.inflight.Delete(tg.ID); p.wg.Done() }()
	start := time.Now()
	resp, err := p.Get.Get(ctx, tg.URL)
	p.Observe(ctx, Observation{Target: tg, Result: Normalize(resp, err), At: start, Latency: time.Since(start)})
}
```

- [ ] **Step 8: Run everything under race**

Run: `gofmt -l internal/poller; go vet ./internal/poller/ && go test -race -count=10 ./internal/poller/`
Expected: nothing from gofmt, `ok`.

- [ ] **Step 9: Write `internal/poller/AGENTS.md`**

```markdown
# Poller

## Purpose
Polls every watched app's health URL on its interval and normalises the answer into `ok`, `degraded` or `down`.

## Ownership
Owns `Normalize` (the only place the spec's four reading rules live) and `Poller` (bounded worker pool, no persistence).

## Local Contracts
- `Normalize` order: `ky.health/1` document, known `status` field or boolean `healthy`, any other 2xx as `Basic`, everything else `Down` with a cause (`egress.Cause` or `status_<code>`). The fixtures in `normalize_test.go` are the suite's real answers on 2026-09-26; add a fixture when an app changes.
- `Tick` never blocks on a slow app: a target already in flight is skipped, a full pool leaves the rest for the next tick.
- `Run` returns only after in-flight polls finish, so the store may be closed after it.

## Verification
- `go test -race ./internal/poller/`

## Child DOX Index
None.
```

- [ ] **Step 10: Commit**

```bash
git add internal/poller
git commit -m "poller: response normaliser and bounded poll loop"
```

---

### Task 3: `internal/alerts` — the state machine

**Files:**
- Create: `internal/alerts/alerts.go`, `internal/alerts/alerts_test.go`, `internal/alerts/AGENTS.md`

**Interfaces:**
- Consumes: `poller.State`, `poller.Result`.
- Produces: `alerts.State` (alias of `poller.State`) with `Pending`; constants `DownAfter=3`, `DegradedAfter=2`, `RecoverAfter=2`, `ReminderEvery=time.Hour`; `alerts.Track` (JSON-tagged, stored on the target row); `alerts.Transition{From, To, At, Cause}`; `alerts.Next(t Track, obs poller.Result, now time.Time) (Track, *Transition)`; `alerts.Decision{Send, Reminder}`; `alerts.Decide(t Track, tr *Transition, now time.Time) (Decision, Track)`.

- [ ] **Step 1: Write the failing tests** at `internal/alerts/alerts_test.go`

```go
package alerts

import (
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/poller"
)

var t0 = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

// run feeds a sequence of states 30 s apart and returns the transitions and final track.
func run(t Track, seq ...poller.State) (Track, []Transition) {
	var out []Transition
	now := t0
	for _, s := range seq {
		var tr *Transition
		t, tr = Next(t, poller.Result{State: s, Cause: "refused"}, now)
		if tr != nil {
			out = append(out, *tr)
		}
		now = now.Add(30 * time.Second)
	}
	return t, out
}

func states(trs []Transition) []State {
	var out []State
	for _, tr := range trs {
		out = append(out, tr.To)
	}
	return out
}

func equal(a, b []State) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTransitions(t *testing.T) {
	cases := []struct {
		name string
		seq  []poller.State
		want []State
	}{
		{"fresh target comes up on the first ok", []poller.State{OK}, []State{OK}},
		{"one failure changes nothing", []poller.State{OK, Down, OK}, []State{OK}},
		{"two failures change nothing", []poller.State{OK, Down, Down, OK}, []State{OK}},
		{"three failures are down", []poller.State{OK, Down, Down, Down}, []State{OK, Down}},
		{"fresh target that never answers is down after three", []poller.State{Down, Down, Down}, []State{Down}},
		{"one good poll does not recover", []poller.State{OK, Down, Down, Down, OK, Down}, []State{OK, Down}},
		{"two good polls recover", []poller.State{OK, Down, Down, Down, OK, OK}, []State{OK, Down, OK}},
		{"one degraded changes nothing", []poller.State{OK, Degraded, OK}, []State{OK}},
		{"two degraded are degraded", []poller.State{OK, Degraded, Degraded}, []State{OK, Degraded}},
		{"degraded then down", []poller.State{OK, Degraded, Degraded, Down, Down, Down}, []State{OK, Degraded, Down}},
		{"down then degraded", []poller.State{Down, Down, Down, Degraded, Degraded}, []State{Down, Degraded}},
		{"mixed bad polls never reach a threshold", []poller.State{OK, Down, Degraded, Down, Degraded, Down}, []State{OK}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, trs := run(Track{}, tc.seq...)
			if got := states(trs); !equal(got, tc.want) {
				t.Fatalf("transitions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTransitionCarriesCauseAndTime(t *testing.T) {
	tr, trs := run(Track{}, OK, Down, Down, Down)
	if len(trs) != 2 || trs[1].Cause != "refused" || trs[1].From != OK || !trs[1].At.Equal(t0.Add(90*time.Second)) {
		t.Fatalf("transitions = %+v", trs)
	}
	if tr.Cause != "refused" || !tr.Since.Equal(t0.Add(90*time.Second)) {
		t.Fatalf("track = %+v", tr)
	}
}

func TestDecideSendsOnChangeAndRemindsHourly(t *testing.T) {
	tr := Track{}
	now := t0
	feed := func(s poller.State) Decision {
		var trans *Transition
		tr, trans = Next(tr, poller.Result{State: s}, now)
		var d Decision
		d, tr = Decide(tr, trans, now)
		now = now.Add(30 * time.Second)
		return d
	}
	if d := feed(OK); d.Send {
		t.Fatal("a fresh target coming up must not notify")
	}
	feed(Down)
	feed(Down)
	if d := feed(Down); !d.Send || d.Reminder {
		t.Fatalf("third failure: %+v", d)
	}
	if d := feed(Down); d.Send {
		t.Fatalf("still down a moment later must not resend: %+v", d)
	}
	now = now.Add(ReminderEvery)
	if d := feed(Down); !d.Send || !d.Reminder {
		t.Fatalf("an hour later: %+v", d)
	}
	if d := feed(Down); d.Send {
		t.Fatal("reminder must not repeat every poll")
	}
	feed(OK)
	if d := feed(OK); !d.Send || d.Reminder {
		t.Fatalf("recovery: %+v", d)
	}
	if !tr.LastNotified.IsZero() {
		t.Fatal("recovery must clear LastNotified")
	}
}

func TestSilenceStopsWebhooksNotTransitions(t *testing.T) {
	tr := Track{SilencedUntil: t0.Add(8 * time.Hour)}
	now := t0
	var last *Transition
	for _, s := range []poller.State{OK, Down, Down, Down} {
		tr, last = Next(tr, poller.Result{State: s}, now)
		now = now.Add(30 * time.Second)
	}
	if last == nil || last.To != Down {
		t.Fatalf("transition must still be recorded under silence: %+v", last)
	}
	d, tr := Decide(tr, last, now)
	if d.Send {
		t.Fatal("silenced target must not send")
	}
	// Silence expires: the hourly reminder resumes from the transition.
	now = t0.Add(9 * time.Hour)
	d, _ = Decide(tr, nil, now)
	if d.Send {
		t.Fatal("no notification was ever sent, so there is nothing to remind about")
	}
}

func TestSilenceUntilFixedEndsOnRecovery(t *testing.T) {
	tr := Track{UntilFixed: true}
	tr, _ = run(tr, OK, Down, Down, Down)
	if !tr.UntilFixed {
		t.Fatal("still down: silence must hold")
	}
	tr, trs := run(tr, OK, OK)
	if len(trs) != 1 || trs[0].To != OK {
		t.Fatalf("expected recovery, got %+v", trs)
	}
	if tr.UntilFixed || !tr.SilencedUntil.IsZero() {
		t.Fatalf("recovery must clear the silence: %+v", tr)
	}
	d, _ := Decide(tr, &trs[0], t0)
	if !d.Send {
		t.Fatal("the recovery itself is sent: the operator asked to hear when it is fixed")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/alerts/`
Expected: FAIL, `undefined: Track`.

- [ ] **Step 3: Write the implementation** at `internal/alerts/alerts.go`

```go
// Package alerts is the per-target state machine: a single bad poll changes nothing, a run of
// them does, and every change is one notification. It holds no I/O; the caller persists the
// Track it returns and sends what Decide tells it to.
package alerts

import (
	"time"

	"github.com/Busnes-app/kypulse-server/internal/poller"
)

// State is a target's settled state. Pending is a target that has not yet been classified.
type State = poller.State

const (
	Pending  State = "pending"
	OK             = poller.OK
	Degraded       = poller.Degraded
	Down           = poller.Down
)

// Thresholds, in consecutive polls. At the default 30 s interval, down takes 90 s to declare.
const (
	DownAfter     = 3
	DegradedAfter = 2
	RecoverAfter  = 2
	ReminderEvery = time.Hour
)

// Track is everything the machine needs between polls. It is stored on the target row.
type Track struct {
	State          State     `json:"state"`
	Since          time.Time `json:"since"`
	Cause          string    `json:"cause,omitempty"`
	OKStreak       int       `json:"ok_streak"`
	DegradedStreak int       `json:"degraded_streak"`
	DownStreak     int       `json:"down_streak"`
	// LastNotified is when the last webhook for the current problem went out; zero when the
	// target is fine. Reminders are counted from it.
	LastNotified time.Time `json:"last_notified,omitempty"`
	// SilencedUntil suppresses webhooks until then; UntilFixed silences until the next OK.
	SilencedUntil time.Time `json:"silenced_until,omitempty"`
	UntilFixed    bool      `json:"until_fixed,omitempty"`
}

// Transition is one state change, the unit the Alerts tab lists.
type Transition struct {
	From  State     `json:"from"`
	To    State     `json:"to"`
	At    time.Time `json:"at"`
	Cause string    `json:"cause,omitempty"`
}

// Next feeds one observation in. It returns the updated track and the transition, if any.
func Next(t Track, obs poller.Result, now time.Time) (Track, *Transition) {
	if t.State == "" {
		t.State, t.Since = Pending, now
	}
	switch obs.State {
	case OK:
		t.OKStreak, t.DegradedStreak, t.DownStreak = t.OKStreak+1, 0, 0
	case Degraded:
		t.OKStreak, t.DegradedStreak, t.DownStreak = 0, t.DegradedStreak+1, 0
	case Down:
		t.OKStreak, t.DegradedStreak, t.DownStreak = 0, 0, t.DownStreak+1
	}
	next := t.State
	switch {
	case t.State == Pending && t.OKStreak >= 1:
		next = OK
	case t.State != OK && t.OKStreak >= RecoverAfter:
		next = OK
	case t.State != Down && t.DownStreak >= DownAfter:
		next = Down
	case t.State != Degraded && t.State != Down && t.DegradedStreak >= DegradedAfter:
		next = Degraded
	case t.State == Down && t.DegradedStreak >= DegradedAfter:
		next = Degraded
	}
	if next == t.State {
		return t, nil
	}
	tr := &Transition{From: t.State, To: next, At: now, Cause: obs.Cause}
	t.State, t.Since, t.Cause = next, now, obs.Cause
	if next == OK {
		t.LastNotified = time.Time{}
		// "Until fixed" is over once the app recovers from a problem. A fresh target's first
		// OK is not a recovery, so a silence set before it was ever classified holds.
		if t.UntilFixed && tr.From != Pending {
			t.UntilFixed, t.SilencedUntil = false, time.Time{}
		}
	}
	return t, tr
}

// Decision is what the caller should send, if anything.
type Decision struct {
	Send     bool
	Reminder bool // a repeat for a problem already announced
}

// Decide says whether a webhook goes out now. A transition into a problem or back to OK is
// sent once; while the problem persists, a reminder goes out every ReminderEvery. Silence
// stops the webhook only; the transition is still recorded and shown on screen.
func Decide(t Track, tr *Transition, now time.Time) (Decision, Track) {
	silenced := t.UntilFixed || now.Before(t.SilencedUntil)
	if tr != nil {
		if tr.From == Pending && tr.To == OK {
			return Decision{}, t // a new target coming up is not news
		}
		if silenced && tr.To != OK {
			return Decision{}, t
		}
		if tr.To != OK {
			t.LastNotified = now // reminders count from here; a recovery has nothing to remind
		}
		return Decision{Send: true}, t
	}
	if t.State == OK || t.State == Pending || silenced {
		return Decision{}, t
	}
	if !t.LastNotified.IsZero() && now.Sub(t.LastNotified) >= ReminderEvery {
		t.LastNotified = now
		return Decision{Send: true, Reminder: true}, t
	}
	return Decision{}, t
}
```

- [ ] **Step 4: Run the tests**

Run: `gofmt -l internal/alerts; go vet ./internal/alerts/ && go test -race -count=1 ./internal/alerts/`
Expected: PASS.

- [ ] **Step 5: Write `internal/alerts/AGENTS.md`**

```markdown
# Alerts

## Purpose
The per-target state machine: consecutive-poll thresholds, transitions, hourly reminders and silences. Pure functions; no I/O.

## Ownership
Owns `Track` (persisted by the store as JSON), `Next` and `Decide`.

## Local Contracts
- `down` after 3 consecutive down polls, `degraded` after 2, back to `ok` after 2; a fresh target is `pending` and becomes `ok` on its first good poll without a notification.
- One notification per transition; a reminder every `ReminderEvery` while not ok, counted from the last notification.
- Silence stops webhooks only: transitions are still recorded. "Until fixed" ends on a recovery from a problem, not on a fresh target's first ok.

## Verification
- `go test ./internal/alerts/`

## Child DOX Index
None.
```

- [ ] **Step 6: Commit**

```bash
git add internal/alerts
git commit -m "alerts: transition thresholds, reminders and silences"
```

---

### Task 4: `internal/notify` — webhook presets and delivery

**Files:**
- Create: `internal/notify/notify.go`, `internal/notify/notify_test.go`, `internal/notify/AGENTS.md`

**Interfaces:**
- Consumes: `egress.ValidateURL`, `egress.Response`, `egress.ErrScheme`, `egress.ErrRefusedAddress`.
- Produces: `notify.Preset` (`Ntfy`, `Gotify`, `Discord`, `Generic`), `notify.Config{Preset, URL, Token}` (JSON-tagged, sealed by Task 6), `notify.Validate(c Config, allowHTTP bool) error`, `notify.Message{App, State, Previous, Reason, Time, URL, Reminder, Test}` with `Title()`, `notify.Build(c, m) (Request, error)`, `notify.Poster` interface, `notify.Notifier{Post Poster, Backoff []time.Duration}` with `Send(ctx, c, m) error`, `notify.DefaultBackoff`, `notify.ErrRejected`, `notify.ErrBadPreset`.

- [ ] **Step 1: Write the failing tests** at `internal/notify/notify_test.go`

```go
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

var msg = Message{App: "KyVault", State: "down", Previous: "ok", Reason: "refused",
	Time: time.Date(2026, 9, 26, 10, 41, 0, 0, time.UTC), URL: "https://pulse.lan/apps/kyvault"}

func TestValidate(t *testing.T) {
	if err := Validate(Config{Preset: "slack", URL: "https://x.lan/"}, false); !errors.Is(err, ErrBadPreset) {
		t.Errorf("bad preset: %v", err)
	}
	if err := Validate(Config{Preset: Ntfy, URL: "http://ntfy.lan/t"}, false); !errors.Is(err, egress.ErrScheme) {
		t.Errorf("http without opt-in: %v", err)
	}
	if err := Validate(Config{Preset: Ntfy, URL: "http://ntfy.lan/t"}, true); err != nil {
		t.Errorf("http with opt-in: %v", err)
	}
	if err := Validate(Config{Preset: Discord, URL: "https://127.0.0.1/hook"}, false); !errors.Is(err, egress.ErrRefusedAddress) {
		t.Errorf("loopback: %v", err)
	}
}

func TestBuildPresets(t *testing.T) {
	ntfy, _ := Build(Config{Preset: Ntfy, URL: "https://ntfy.lan/pulse", Token: "tk"}, msg)
	if ntfy.ContentType != "text/plain" || ntfy.Headers["Title"] != "kyPulse: KyVault is down" || ntfy.Headers["Priority"] != "5" || ntfy.Headers["Authorization"] != "Bearer tk" {
		t.Errorf("ntfy = %+v", ntfy)
	}
	if !strings.Contains(string(ntfy.Body), "(refused) at 2026-09-26T10:41:00Z\nhttps://pulse.lan/apps/kyvault") {
		t.Errorf("ntfy body = %q", ntfy.Body)
	}

	gotify, _ := Build(Config{Preset: Gotify, URL: "https://gotify.lan/", Token: "gk"}, msg)
	var g map[string]any
	_ = json.Unmarshal(gotify.Body, &g)
	if gotify.URL != "https://gotify.lan/message" || gotify.Headers["X-Gotify-Key"] != "gk" || g["priority"] != float64(8) || g["title"] != "kyPulse: KyVault is down" {
		t.Errorf("gotify = %+v body=%v", gotify, g)
	}
	if strings.Contains(gotify.URL, "gk") {
		t.Error("gotify token must not be in the URL")
	}

	discord, _ := Build(Config{Preset: Discord, URL: "https://discord.com/api/webhooks/1/x"}, msg)
	var d map[string]string
	_ = json.Unmarshal(discord.Body, &d)
	if !strings.HasPrefix(d["content"], "kyPulse: KyVault is down") {
		t.Errorf("discord = %v", d)
	}

	generic, _ := Build(Config{Preset: Generic, URL: "https://hooks.lan/x", Token: "gt"}, msg)
	var m Message
	if err := json.Unmarshal(generic.Body, &m); err != nil || m.App != "KyVault" || m.Previous != "ok" || generic.Headers["Authorization"] != "Bearer gt" {
		t.Errorf("generic = %+v err=%v", m, err)
	}
	if strings.Contains(string(generic.Body), "gt") {
		t.Error("generic body must not carry the token")
	}
}

func TestTitles(t *testing.T) {
	cases := map[string]Message{
		"kyPulse: KyVault is down":     msg,
		"kyPulse: KyVault still down":  {App: "KyVault", State: "down", Reminder: true},
		"kyPulse: KyVault recovered":   {App: "KyVault", State: "ok"},
		"kyPulse: KyVault is degraded": {App: "KyVault", State: "degraded"},
		"kyPulse: test alert":          {Test: true},
	}
	for want, m := range cases {
		if got := m.Title(); got != want {
			t.Errorf("Title() = %q, want %q", got, want)
		}
	}
}

type fakePoster struct {
	codes []int // one per call; a negative code means a transport error
	calls int
}

func (f *fakePoster) Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error) {
	f.calls++
	code := f.codes[f.calls-1]
	if code < 0 {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &egress.Response{StatusCode: code}, nil
}

func TestSendRetriesTransportAnd5xxButNot4xx(t *testing.T) {
	cfg := Config{Preset: Generic, URL: "https://hooks.lan/x"}
	fast := []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

	p := &fakePoster{codes: []int{-1, 503, 429, 200}}
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); err != nil || p.calls != 4 {
		t.Fatalf("recovering send: err=%v calls=%d", err, p.calls)
	}

	p = &fakePoster{codes: []int{500, 500, 500, 500}}
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); err == nil || p.calls != 4 {
		t.Fatalf("exhausted: err=%v calls=%d", err, p.calls)
	}

	p = &fakePoster{codes: []int{403}}
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); !errors.Is(err, ErrRejected) || p.calls != 1 {
		t.Fatalf("rejected: err=%v calls=%d", err, p.calls)
	}

	p = &fakePoster{codes: []int{500, 200}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&Notifier{Post: p, Backoff: []time.Duration{time.Hour}}).Send(ctx, cfg, msg); !errors.Is(err, context.Canceled) || p.calls != 1 {
		t.Fatalf("cancelled: err=%v calls=%d", err, p.calls)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/notify/`
Expected: FAIL, `undefined: Config`.

- [ ] **Step 3: Write the implementation** at `internal/notify/notify.go`

```go
// Package notify delivers alert messages to one outbound webhook. It knows four shapes
// (ntfy, Gotify, Discord, generic JSON) and nothing about why a message is sent.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

// Preset names the receiver's wire format.
type Preset string

const (
	Ntfy    Preset = "ntfy"
	Gotify  Preset = "gotify"
	Discord Preset = "discord"
	Generic Preset = "generic"
)

// Config is the admin's webhook. It is stored sealed; the token never leaves the process.
type Config struct {
	Preset Preset `json:"preset"`
	URL    string `json:"url"`
	Token  string `json:"token,omitempty"`
}

var ErrBadPreset = errors.New("notify: preset must be ntfy, gotify, discord or generic")

// Validate checks a config an admin saved. allowHTTP mirrors KYPULSE_ALERT_ALLOW_HTTP.
func Validate(c Config, allowHTTP bool) error {
	switch c.Preset {
	case Ntfy, Gotify, Discord, Generic:
	default:
		return ErrBadPreset
	}
	return egress.ValidateURL(c.URL, allowHTTP)
}

// Message is one alert. It carries names and codes only: no log lines, no user names, no
// IPs, because hosted ntfy and Discord are third parties.
type Message struct {
	App      string    `json:"app"`
	State    string    `json:"state"`
	Previous string    `json:"previous"`
	Reason   string    `json:"reason,omitempty"`
	Time     time.Time `json:"time"`
	URL      string    `json:"url"` // link back to the kyPulse app page
	Reminder bool      `json:"reminder,omitempty"`
	Test     bool      `json:"test,omitempty"`
}

// Title is the one-line summary every preset uses.
func (m Message) Title() string {
	switch {
	case m.Test:
		return "kyPulse: test alert"
	case m.Reminder:
		return fmt.Sprintf("kyPulse: %s still %s", m.App, m.State)
	case m.State == "ok":
		return fmt.Sprintf("kyPulse: %s recovered", m.App)
	}
	return fmt.Sprintf("kyPulse: %s is %s", m.App, m.State)
}

func (m Message) text() string {
	s := m.Title()
	if m.Reason != "" {
		s += " (" + m.Reason + ")"
	}
	return s + " at " + m.Time.UTC().Format(time.RFC3339) + "\n" + m.URL
}

// Request is the wire form of one message for one preset.
type Request struct {
	URL         string
	ContentType string
	Headers     map[string]string
	Body        []byte
}

// Build renders m for c's preset. Tokens ride in headers, never in the URL, so they do not
// land in the receiver's access log.
func Build(c Config, m Message) (Request, error) {
	h := map[string]string{}
	switch c.Preset {
	case Ntfy:
		h["Title"] = m.Title()
		h["Priority"] = "3"
		if m.State == "down" && !m.Reminder {
			h["Priority"] = "5"
		}
		if c.Token != "" {
			h["Authorization"] = "Bearer " + c.Token
		}
		return Request{URL: c.URL, ContentType: "text/plain", Headers: h, Body: []byte(m.text())}, nil
	case Gotify:
		if c.Token != "" {
			h["X-Gotify-Key"] = c.Token
		}
		prio := 5
		if m.State == "down" {
			prio = 8
		}
		body, _ := json.Marshal(map[string]any{"title": m.Title(), "message": m.text(), "priority": prio})
		return Request{URL: strings.TrimRight(c.URL, "/") + "/message", ContentType: "application/json", Headers: h, Body: body}, nil
	case Discord:
		body, _ := json.Marshal(map[string]string{"content": m.text()})
		return Request{URL: c.URL, ContentType: "application/json", Headers: h, Body: body}, nil
	case Generic:
		if c.Token != "" {
			h["Authorization"] = "Bearer " + c.Token
		}
		body, _ := json.Marshal(m)
		return Request{URL: c.URL, ContentType: "application/json", Headers: h, Body: body}, nil
	}
	return Request{}, ErrBadPreset
}

// Poster is what Notifier needs from egress; a fake stands in for tests.
type Poster interface {
	Post(ctx context.Context, url, contentType string, body []byte, headers map[string]string) (*egress.Response, error)
}

// Notifier sends with retries. Backoff is exported so tests do not wait.
type Notifier struct {
	Post    Poster
	Backoff []time.Duration // waits between attempts; len+1 attempts in total
}

// DefaultBackoff gives four attempts over about twenty seconds.
var DefaultBackoff = []time.Duration{2 * time.Second, 6 * time.Second, 12 * time.Second}

// ErrRejected is a 4xx other than 429: the receiver understood and refused, so retrying
// would only repeat the refusal.
var ErrRejected = errors.New("notify: receiver rejected the message")

// Send delivers m, retrying transport errors, 429 and 5xx. It returns the last error.
func (n *Notifier) Send(ctx context.Context, c Config, m Message) error {
	req, err := Build(c, m)
	if err != nil {
		return err
	}
	var last error
	for attempt := 0; ; attempt++ {
		resp, err := n.Post.Post(ctx, req.URL, req.ContentType, req.Body, req.Headers)
		switch {
		case err != nil:
			last = err
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return nil
		case resp.StatusCode == 429 || resp.StatusCode >= 500:
			last = fmt.Errorf("notify: receiver answered %d", resp.StatusCode)
		default:
			return fmt.Errorf("%w: %d", ErrRejected, resp.StatusCode)
		}
		if attempt >= len(n.Backoff) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(n.Backoff[attempt]):
		}
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `gofmt -l internal/notify; go vet ./internal/notify/ && go test -race -count=1 ./internal/notify/`
Expected: PASS.

- [ ] **Step 5: Write `internal/notify/AGENTS.md`**

```markdown
# Notify

## Purpose
Renders one alert message for one webhook preset (ntfy, Gotify, Discord, generic JSON) and delivers it with retries.

## Ownership
Owns `Config`, `Message`, `Build`, `Notifier`. Knows nothing about why a message is sent.

## Local Contracts
- Tokens ride in headers (`Authorization: Bearer`, `X-Gotify-Key`), never in the URL or body.
- Messages carry app name, state, previous state, reason code, time and a kyPulse link — never log lines, user names or IPs.
- Retries on transport errors, 429 and 5xx; a 4xx is `ErrRejected` and is not retried. Four attempts over `DefaultBackoff`.

## Verification
- `go test ./internal/notify/`

## Child DOX Index
None.
```

- [ ] **Step 6: Commit**

```bash
git add internal/notify
git commit -m "notify: webhook presets and retrying delivery"
```

---

### Task 5: Store — targets and target events

**Files:**
- Modify: `internal/store/store.go`, `internal/store/models.go`, `internal/store/sqlstore.go`, `internal/store/migrations/migrations.go`, `internal/store/AGENTS.md`
- Create: `internal/store/targets.go` (the new sub-store; keeps `sqlstore.go` from growing), `internal/store/targets_test.go`

**Interfaces:**
- Consumes: the existing `SQLStore`, `rebind`, `errorsIs`, `ErrNotFound`, `ErrAlreadyExists`.
- Produces, in package `store`:

```go
// Target is a watched app. Track and LastResult are JSON the monitor package owns
// (alerts.Track and poller.Result); the store never decodes them.
type Target struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	IntervalSec   int        `json:"interval_sec"`
	Enabled       bool       `json:"enabled"`
	Container     string     `json:"container,omitempty"` // KyYard container link, unused until step 3
	State         string     `json:"state"`               // pending|ok|degraded|down, denormalised from Track
	StateSince    time.Time  `json:"state_since"`
	Cause         string     `json:"cause,omitempty"`
	TrackJSON     string     `json:"-"`
	LastResult    string     `json:"last_result"` // poller.Result JSON, "" before the first poll
	LastPolledAt  *time.Time `json:"last_polled_at,omitempty"`
	LastLatencyMS int64      `json:"last_latency_ms"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TargetEvent is one transition or reminder, the unit the Alerts tab lists.
type TargetEvent struct {
	ID          int64     `json:"id"`
	TargetID    string    `json:"target_id"`
	At          time.Time `json:"at"`
	FromState   string    `json:"from"`
	ToState     string    `json:"to"`
	Cause       string    `json:"cause,omitempty"`
	Reminder    bool      `json:"reminder"`
	Notified    bool      `json:"notified"`
	NotifyError string    `json:"notify_error,omitempty"`
}

// PollUpdate is what one observation writes back.
type PollUpdate struct {
	PolledAt   time.Time
	LatencyMS  int64
	LastResult string
	TrackJSON  string
	State      string
	StateSince time.Time
	Cause      string
}

type TargetStore interface {
	CreateTarget(ctx context.Context, t *Target) error   // ErrAlreadyExists on a duplicate name
	GetTarget(ctx context.Context, id string) (*Target, error)
	ListTargets(ctx context.Context) ([]*Target, error) // ordered by name
	UpdateTarget(ctx context.Context, t *Target) error   // name, url, interval, enabled, container only
	DeleteTarget(ctx context.Context, id string) error   // cascades events
	RecordPoll(ctx context.Context, id string, u PollUpdate) error
	SetTrack(ctx context.Context, id string, trackJSON string) error // silences edit the track between polls
	RecordEvent(ctx context.Context, e *TargetEvent) error           // sets e.ID
	SetEventNotified(ctx context.Context, id int64, notified bool, notifyError string) error
	ListEvents(ctx context.Context, targetID string, offset, limit int) ([]*TargetEvent, int, error) // targetID "" = all; newest first
}
```

  and `Targets() TargetStore` on `Store`.

- [ ] **Step 1: Write the failing tests** at `internal/store/targets_test.go` (package `store_test`, using `newTestStore` from `store_test.go`):

```go
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestTargetLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	tg := &store.Target{ID: "tgt_1", Name: "KyVault", URL: "https://vault.lan/healthz", IntervalSec: 30, Enabled: true}
	if err := st.Targets().CreateTarget(ctx, tg); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "tgt_2", Name: "KyVault", URL: "https://x/", IntervalSec: 30}); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	got, err := st.Targets().GetTarget(ctx, "tgt_1")
	if err != nil || got.Name != "KyVault" || got.State != "pending" || got.LastPolledAt != nil || !got.Enabled {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := st.Targets().GetTarget(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}

	got.Name, got.IntervalSec, got.Enabled = "KyVault (prod)", 60, false
	if err := st.Targets().UpdateTarget(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	list, _ := st.Targets().ListTargets(ctx)
	if len(list) != 1 || list[0].Name != "KyVault (prod)" || list[0].IntervalSec != 60 || list[0].Enabled {
		t.Fatalf("list after update: %+v", list[0])
	}

	now := time.Date(2026, 9, 26, 10, 41, 0, 0, time.UTC)
	err = st.Targets().RecordPoll(ctx, "tgt_1", store.PollUpdate{PolledAt: now, LatencyMS: 12, LastResult: `{"state":"down","cause":"refused"}`, TrackJSON: `{"state":"down"}`, State: "down", StateSince: now, Cause: "refused"})
	if err != nil {
		t.Fatalf("record poll: %v", err)
	}
	got, _ = st.Targets().GetTarget(ctx, "tgt_1")
	if got.State != "down" || got.Cause != "refused" || got.LastPolledAt == nil || !got.LastPolledAt.Equal(now) || got.LastLatencyMS != 12 || got.TrackJSON != `{"state":"down"}` {
		t.Fatalf("after poll: %+v", got)
	}
	if err := st.Targets().SetTrack(ctx, "tgt_1", `{"state":"down","until_fixed":true}`); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Targets().GetTarget(ctx, "tgt_1")
	if got.TrackJSON != `{"state":"down","until_fixed":true}` {
		t.Fatalf("set track: %q", got.TrackJSON)
	}

	if err := st.Targets().DeleteTarget(ctx, "tgt_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Targets().GetTarget(ctx, "tgt_1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestTargetEvents(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, id := range []string{"a", "b"} {
		if err := st.Targets().CreateTarget(ctx, &store.Target{ID: id, Name: id, URL: "https://" + id + "/", IntervalSec: 30, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	for i, tid := range []string{"a", "b", "a"} {
		e := &store.TargetEvent{TargetID: tid, At: base.Add(time.Duration(i) * time.Minute), FromState: "ok", ToState: "down", Cause: "refused"}
		if err := st.Targets().RecordEvent(ctx, e); err != nil || e.ID == 0 {
			t.Fatalf("record %d: id=%d %v", i, e.ID, err)
		}
	}
	all, total, err := st.Targets().ListEvents(ctx, "", 0, 10)
	if err != nil || total != 3 || len(all) != 3 || !all[0].At.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("all: %d %v %+v", total, err, all)
	}
	onlyA, total, _ := st.Targets().ListEvents(ctx, "a", 0, 1)
	if total != 2 || len(onlyA) != 1 || onlyA[0].TargetID != "a" {
		t.Fatalf("a: %d %+v", total, onlyA)
	}
	if err := st.Targets().SetEventNotified(ctx, all[0].ID, false, "receiver answered 503"); err != nil {
		t.Fatal(err)
	}
	after, _, _ := st.Targets().ListEvents(ctx, "", 0, 1)
	if after[0].Notified || after[0].NotifyError != "receiver answered 503" {
		t.Fatalf("notified: %+v", after[0])
	}
	if err := st.Targets().DeleteTarget(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, total, _ = st.Targets().ListEvents(ctx, "", 0, 10); total != 1 {
		t.Fatalf("events must cascade on delete: %d left", total)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/store/ -run 'TestTarget'`
Expected: FAIL, `st.Targets undefined`.

- [ ] **Step 3: Migration 5.** Append to `registry` in `internal/store/migrations/migrations.go`, after migration 4:

```go
	{
		Version: 5,
		Name:    "targets",
		SQLite: `
CREATE TABLE IF NOT EXISTS targets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    interval_sec INTEGER NOT NULL DEFAULT 30,
    enabled INTEGER NOT NULL DEFAULT 1,
    container TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'pending',
    state_since DATETIME NOT NULL,
    cause TEXT NOT NULL DEFAULT '',
    track_json TEXT NOT NULL DEFAULT '',
    last_result TEXT NOT NULL DEFAULT '',
    last_polled_at DATETIME,
    last_latency_ms INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS target_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id TEXT NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    at DATETIME NOT NULL,
    from_state TEXT NOT NULL,
    to_state TEXT NOT NULL,
    cause TEXT NOT NULL DEFAULT '',
    reminder INTEGER NOT NULL DEFAULT 0,
    notified INTEGER NOT NULL DEFAULT 0,
    notify_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_target_events_target_at ON target_events(target_id, at);
CREATE INDEX IF NOT EXISTS idx_target_events_at ON target_events(at);
`,
		Postgres: `
CREATE TABLE IF NOT EXISTS targets (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    url TEXT NOT NULL,
    interval_sec INTEGER NOT NULL DEFAULT 30,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    container VARCHAR(255) NOT NULL DEFAULT '',
    state VARCHAR(16) NOT NULL DEFAULT 'pending',
    state_since TIMESTAMPTZ NOT NULL,
    cause VARCHAR(64) NOT NULL DEFAULT '',
    track_json TEXT NOT NULL DEFAULT '',
    last_result TEXT NOT NULL DEFAULT '',
    last_polled_at TIMESTAMPTZ,
    last_latency_ms BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS target_events (
    id BIGSERIAL PRIMARY KEY,
    target_id VARCHAR(64) NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
    at TIMESTAMPTZ NOT NULL,
    from_state VARCHAR(16) NOT NULL,
    to_state VARCHAR(16) NOT NULL,
    cause VARCHAR(64) NOT NULL DEFAULT '',
    reminder BOOLEAN NOT NULL DEFAULT FALSE,
    notified BOOLEAN NOT NULL DEFAULT FALSE,
    notify_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_target_events_target_at ON target_events(target_id, at);
CREATE INDEX IF NOT EXISTS idx_target_events_at ON target_events(at);
`,
	},
```

- [ ] **Step 4: Models and interface.** Add the `Target`, `TargetEvent` and `PollUpdate` types from the Interfaces block to `internal/store/models.go`, the `TargetStore` interface to `internal/store/store.go`, and `Targets() TargetStore` to the `Store` interface.

- [ ] **Step 5: Implement `internal/store/targets.go`.** Follow `settingsStore`'s shape exactly: a `targetStore struct{ store *SQLStore }`, every query through `t.store.rebind`, `time.Now().UTC()`, `errorsIs(err, sql.ErrNoRows)` → `ErrNotFound`, the `UNIQUE`/`duplicate key` string check → `ErrAlreadyExists`. `RecordPoll` is one `UPDATE targets SET last_polled_at=?, last_latency_ms=?, last_result=?, track_json=?, state=?, state_since=?, cause=?, updated_at=? WHERE id=?`. `ListEvents` runs `SELECT COUNT(1)` then `SELECT ... ORDER BY at DESC, id DESC LIMIT ? OFFSET ?`, with `WHERE target_id = ?` only when `targetID != ""`. Scan `last_polled_at` into `sql.NullTime`. `CreateTarget` defaults `State` to `"pending"` and `StateSince` to now when zero, and `IntervalSec` to 30 when zero. `DeleteTarget` returns `ErrNotFound` when no row was affected. Wire the sub-store in `sqlstore.go`: field `targets *targetStore`, construction in `newSQLStore`, accessor `func (s *SQLStore) Targets() TargetStore { return s.targets }`.

  SQLite foreign keys are on (`_pragma=foreign_keys(ON)` in the default DSN, and `testdb` uses the same store `Open`), so `ON DELETE CASCADE` works; the cascade test proves it on both dialects.

- [ ] **Step 6: Run the store tests on SQLite, then Postgres if available**

Run: `go test -count=1 ./internal/store/` and, if a local Postgres is up, `KYPULSE_TEST_POSTGRES_DSN='postgres://postgres:postgrespassword@127.0.0.1:5432/kypulse?sslmode=disable' go test -count=1 ./internal/store/` (CI runs the Postgres job regardless).
Expected: PASS.

- [ ] **Step 7: DOX.** `internal/store/AGENTS.md`: Ownership adds `TargetStore`; Local Contracts adds "- `targets.track_json` and `targets.last_result` are JSON owned by `internal/monitor` (`alerts.Track`, `poller.Result`); the store stores and returns them verbatim. `state`, `state_since` and `cause` are denormalised from the track for listing. Deleting a target cascades its events."

- [ ] **Step 8: Commit**

```bash
git add internal/store
git commit -m "store: targets and target_events"
```

---

### Task 6: Config and the sealed webhook

**Files:**
- Modify: `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/AGENTS.md`, `internal/api/settings_handlers.go`, `internal/api/authz_test.go`, `docker-compose.yml`, `README.md`
- Create: `internal/monitor/webhook.go`, `internal/monitor/webhook_test.go`

**Interfaces:**
- Consumes: `notify.Config`, `notify.Validate`, `recoveryclient.NewAESGCMSealer(key []byte, label string) (Sealer, error)` with `Seal(plain []byte) (string, error)` / `Open(sealed string) ([]byte, error)`, `store.SettingsStore`.
- Produces:
  - `config.AlertsConfig{AllowHTTP bool}` at `cfg.Alerts` from `KYPULSE_ALERT_ALLOW_HTTP` (default false); `config.PollConfig{Workers int}` at `cfg.Poll` from `KYPULSE_POLL_WORKERS` (default 4, 1..32 else startup error).
  - In package `monitor`: `const webhookKey = "alert_webhook_enc"`, `const webhookStatusKey = "alert_webhook_status"`, `const webhookLabel = "kypulse:setting:alert_webhook"`;
    `type Webhooks struct{ Settings store.SettingsStore; Sealer recoveryclient.Sealer }`,
    `func NewWebhooks(cfg *config.Config, s store.SettingsStore) (*Webhooks, error)`,
    `func (w *Webhooks) Save(ctx, c notify.Config) error` (seals JSON),
    `func (w *Webhooks) Load(ctx) (notify.Config, bool, error)` (false when unset),
    `func (w *Webhooks) Delete(ctx) error`,
    `type DeliveryStatus struct{ At time.Time; OK bool; Error string }` with `func (w *Webhooks) SetStatus(ctx, s DeliveryStatus) error` and `func (w *Webhooks) Status(ctx) (DeliveryStatus, bool, error)` (plaintext JSON setting; no secret in it).

- [ ] **Step 1: Write the failing config test.** Append to `internal/config/config_test.go` (match its existing style: `t.Setenv("KYPULSE_DATA_DIR", t.TempDir())` first):

```go
func TestAlertAndPollConfig(t *testing.T) {
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Alerts.AllowHTTP || cfg.Poll.Workers != 4 {
		t.Fatalf("defaults: %+v %+v", cfg.Alerts, cfg.Poll)
	}
	t.Setenv("KYPULSE_ALERT_ALLOW_HTTP", "true")
	t.Setenv("KYPULSE_POLL_WORKERS", "8")
	cfg, _ = config.LoadFromEnv()
	if !cfg.Alerts.AllowHTTP || cfg.Poll.Workers != 8 {
		t.Fatalf("set: %+v %+v", cfg.Alerts, cfg.Poll)
	}
	t.Setenv("KYPULSE_POLL_WORKERS", "0")
	if _, err := config.LoadFromEnv(); err == nil || !strings.Contains(err.Error(), "KYPULSE_POLL_WORKERS") {
		t.Fatalf("workers=0 must fail startup: %v", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/config/ -run TestAlertAndPollConfig`
Expected: FAIL, `cfg.Alerts undefined`.

- [ ] **Step 3: Add the config.** In `internal/config/config.go`: fields `Alerts AlertsConfig \`json:"alerts"\`` and `Poll PollConfig \`json:"poll"\`` on `Config`; types

```go
// AlertsConfig governs the outbound webhook.
type AlertsConfig struct {
	// AllowHTTP admits a plain-http webhook URL. Off by default: the webhook carries the
	// alert stream and, for Gotify and ntfy, a token in a header.
	AllowHTTP bool `json:"allow_http"`
}

// PollConfig sizes the health poller.
type PollConfig struct {
	Workers int `json:"workers"` // concurrent polls; one slow app never delays the rest
}
```

  and in `LoadFromEnv`, before `cfg := &Config{`:

```go
	pollWorkers := getEnvInt("KYPULSE_POLL_WORKERS", 4)
	if pollWorkers < 1 || pollWorkers > 32 {
		return nil, fmt.Errorf("KYPULSE_POLL_WORKERS: must be 1..32, got %d", pollWorkers)
	}
```

  and in the literal: `Alerts: AlertsConfig{AllowHTTP: getEnvBool("KYPULSE_ALERT_ALLOW_HTTP", false)}, Poll: PollConfig{Workers: pollWorkers},`.

- [ ] **Step 4: Run the config tests**

Run: `go test ./internal/config/`
Expected: PASS.

- [ ] **Step 5: Write the failing webhook-storage test** at `internal/monitor/webhook_test.go`:

```go
package monitor_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func testStore(t *testing.T) (*config.Config, store.Store) {
	t.Helper()
	t.Setenv("KYPULSE_DATA_DIR", t.TempDir())
	cfg, err := config.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = testdb.Config(t)
	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return cfg, st
}

func TestWebhookIsSealedAtRest(t *testing.T) {
	ctx := context.Background()
	cfg, st := testStore(t)
	w, err := monitor.NewWebhooks(cfg, st.Settings())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := w.Load(ctx); err != nil || ok {
		t.Fatalf("unset: ok=%v err=%v", ok, err)
	}
	in := notify.Config{Preset: notify.Gotify, URL: "https://gotify.lan", Token: "gk-secret"}
	if err := w.Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	raw, err := st.Settings().GetSetting(ctx, "alert_webhook_enc")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "gk-secret") || strings.Contains(raw, "gotify.lan") {
		t.Fatalf("stored row is not sealed: %q", raw)
	}
	out, ok, err := w.Load(ctx)
	if err != nil || !ok || out != in {
		t.Fatalf("round trip: %+v ok=%v err=%v", out, ok, err)
	}

	// A different deployment key must not open it.
	other := *cfg
	other.Security.EncryptionKey = []byte(strings.Repeat("k", 32))
	w2, _ := monitor.NewWebhooks(&other, st.Settings())
	if _, _, err := w2.Load(ctx); err == nil {
		t.Fatal("another key opened the webhook")
	}

	if err := w.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := w.Load(ctx); ok {
		t.Fatal("still set after delete")
	}
}

func TestDeliveryStatusRoundTrip(t *testing.T) {
	ctx := context.Background()
	cfg, st := testStore(t)
	w, _ := monitor.NewWebhooks(cfg, st.Settings())
	if _, ok, _ := w.Status(ctx); ok {
		t.Fatal("status before any send")
	}
	at := time.Date(2026, 9, 26, 10, 41, 0, 0, time.UTC)
	if err := w.SetStatus(ctx, monitor.DeliveryStatus{At: at, OK: false, Error: "receiver answered 503"}); err != nil {
		t.Fatal(err)
	}
	s, ok, err := w.Status(ctx)
	if err != nil || !ok || s.OK || !s.At.Equal(at) || s.Error != "receiver answered 503" {
		t.Fatalf("status: %+v ok=%v err=%v", s, ok, err)
	}
}
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./internal/monitor/`
Expected: FAIL, package does not exist / `undefined: monitor.NewWebhooks`.

- [ ] **Step 7: Write `internal/monitor/webhook.go`**

```go
// Package monitor wires the poller, the state machine and the notifier to the store. It
// owns what is due, what each observation means, what gets recorded, and what gets sent.
package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

const (
	webhookKey       = "alert_webhook_enc"
	webhookStatusKey = "alert_webhook_status"
	webhookLabel     = "kypulse:setting:alert_webhook"
)

// Webhooks stores the admin's webhook sealed under the deployment key, and the last
// delivery result in the clear (it holds no secret).
type Webhooks struct {
	Settings store.SettingsStore
	Sealer   recoveryclient.Sealer
}

func NewWebhooks(cfg *config.Config, s store.SettingsStore) (*Webhooks, error) {
	sealer, err := recoveryclient.NewAESGCMSealer(cfg.Security.EncryptionKey, webhookLabel)
	if err != nil {
		return nil, err
	}
	return &Webhooks{Settings: s, Sealer: sealer}, nil
}

func (w *Webhooks) Save(ctx context.Context, c notify.Config) error {
	plain, err := json.Marshal(c)
	if err != nil {
		return err
	}
	sealed, err := w.Sealer.Seal(plain)
	if err != nil {
		return err
	}
	return w.Settings.SetSetting(ctx, webhookKey, sealed)
}

// Load returns the webhook and whether one is set.
func (w *Webhooks) Load(ctx context.Context) (notify.Config, bool, error) {
	sealed, err := w.Settings.GetSetting(ctx, webhookKey)
	if errors.Is(err, store.ErrNotFound) {
		return notify.Config{}, false, nil
	}
	if err != nil {
		return notify.Config{}, false, err
	}
	plain, err := w.Sealer.Open(sealed)
	if err != nil {
		return notify.Config{}, false, err
	}
	var c notify.Config
	if err := json.Unmarshal(plain, &c); err != nil {
		return notify.Config{}, false, err
	}
	return c, true, nil
}

func (w *Webhooks) Delete(ctx context.Context) error {
	if err := w.Settings.DeleteSetting(ctx, webhookKey); err != nil {
		return err
	}
	return w.Settings.DeleteSetting(ctx, webhookStatusKey)
}

// DeliveryStatus is the last send's outcome, for the alert bar's "alerts not being delivered".
type DeliveryStatus struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
}

func (w *Webhooks) SetStatus(ctx context.Context, s DeliveryStatus) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return w.Settings.SetSetting(ctx, webhookStatusKey, string(b))
}

func (w *Webhooks) Status(ctx context.Context) (DeliveryStatus, bool, error) {
	raw, err := w.Settings.GetSetting(ctx, webhookStatusKey)
	if errors.Is(err, store.ErrNotFound) {
		return DeliveryStatus{}, false, nil
	}
	if err != nil {
		return DeliveryStatus{}, false, err
	}
	var s DeliveryStatus
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return DeliveryStatus{}, false, err
	}
	return s, true, nil
}
```

- [ ] **Step 8: Run the monitor tests**

Run: `go test -count=1 ./internal/monitor/`
Expected: PASS.

- [ ] **Step 9: Keep sealed rows out of `/api/settings`.** In `internal/api/settings_handlers.go`, the admin `extra_settings` loop currently drops keys with prefix `kyrecovery_token`. Change it to drop any key with prefix `kyrecovery_token` **or** suffix `_enc`, with the comment: `// Sealed rows (suffix _enc) never leave the process in either form; the kyrecovery prefix covers its legacy plaintext spelling.` Add to `TestSettingsExposureByRole` in `internal/api/authz_test.go` a seeded `alert_webhook_enc` = `"sealed-webhook-blob"` and an assertion that `extra` has no `alert_webhook_enc` key, beside the existing `kyrecovery_token_enc` assertion.

Run: `go test ./internal/api/ -run TestSettingsExposureByRole`. Expected: PASS.

- [ ] **Step 10: Docs and compose.** `docker-compose.yml` environment gains, after the backup block:

```yaml
      # Alerts. The webhook itself is set in the UI and stored sealed; this only admits a
      # plain-http receiver on your own network (off: HTTPS required).
      - KYPULSE_ALERT_ALLOW_HTTP=${KYPULSE_ALERT_ALLOW_HTTP:-false}
      # Concurrent health polls (1..32).
      - KYPULSE_POLL_WORKERS=${KYPULSE_POLL_WORKERS:-4}
```

  `internal/config/AGENTS.md` Local Contracts adds: "- `KYPULSE_ALERT_ALLOW_HTTP` (default false) admits a plain-http webhook URL; health targets may always be plain http. `KYPULSE_POLL_WORKERS` (default 4, 1..32) bounds concurrent polls." `README.md` "Run" section adds one sentence: "Watched apps and the alert webhook are set in the UI (admins) or through `/api/targets` and `/api/alerts/webhook`; `KYPULSE_ALERT_ALLOW_HTTP` and `KYPULSE_POLL_WORKERS` are the only alerting variables."

- [ ] **Step 11: Commit**

```bash
git add internal/config internal/monitor internal/api/settings_handlers.go internal/api/authz_test.go docker-compose.yml README.md
git commit -m "config: alert and poll settings; sealed webhook storage"
```

---

### Task 7: `internal/monitor` — the service

**Files:**
- Create: `internal/monitor/service.go`, `internal/monitor/service_test.go`, `internal/monitor/AGENTS.md`

**Interfaces:**
- Consumes: `store.TargetStore`, `store.AuditStore`, `poller.Poller/Target/Observation/Result`, `alerts.Next/Decide/Track`, `notify.Notifier/Config/Message`, `Webhooks` (Task 6), `logging.Logger`.
- Produces:

```go
// Service is the monitoring loop's brain. One per process.
type Service struct {
	Store    store.Store
	Webhooks *Webhooks
	Notifier *notify.Notifier // Post is an *egress.Client with AllowHTTP = cfg.Alerts.AllowHTTP
	Logger   *logging.Logger
	AppURL   string // for the link in messages: AppURL + "/#/apps/" + target ID
	Now      func() time.Time // time.Now; tests inject
}

func (s *Service) Due(ctx context.Context, now time.Time) ([]poller.Target, error)
func (s *Service) Observe(ctx context.Context, o poller.Observation)
// Silence sets the target's silence: d > 0 for a duration, untilFixed for the flag, both
// zero/false to clear. It rewrites only the track's silence fields.
func (s *Service) Silence(ctx context.Context, id string, d time.Duration, untilFixed bool) error
// SendTest delivers a test message to the stored webhook and records the delivery status.
func (s *Service) SendTest(ctx context.Context) error
// Track decodes a target's track; the API uses it to show silences.
func Track(t *store.Target) alerts.Track
```

  Declared logging vocabulary (in `service.go`, package level; none of these names exist in ky-primitives):

```go
var (
	evStateChanged   = logging.DeclareEvent("target_state_changed", "watched app changed state", slog.LevelWarn)
	evAlertSent      = logging.DeclareEvent("alert_sent", "alert delivered to the webhook", slog.LevelInfo)
	evAlertFailed    = logging.DeclareEvent("alert_send_failed", "alert could not be delivered", slog.LevelWarn)
	evPollStoreError = logging.DeclareEvent("poll_store_error", "poll result could not be stored", slog.LevelError)
	fState           = logging.DeclareString("state")
	fPrevious        = logging.DeclareString("previous")
)
```

  (`target_id`, `reason_code`, `error_kind` and `count` already exist in the library: use `logging.TargetID`, `logging.ReasonCode`, `logging.Err`.)

- [ ] **Step 1: Write the failing tests** at `internal/monitor/service_test.go` (same package `monitor_test`, reusing `testStore` from `webhook_test.go`):

```go
package monitor_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

type fakePoster struct {
	sent  []notify.Message // decoded from the generic preset body
	codes []int
	calls int
}

func (f *fakePoster) Post(_ context.Context, _ string, _ string, body []byte, _ map[string]string) (*egress.Response, error) {
	f.calls++
	var m notify.Message
	_ = json.Unmarshal(body, &m)
	f.sent = append(f.sent, m)
	code := 200
	if f.calls-1 < len(f.codes) {
		code = f.codes[f.calls-1]
	}
	if code < 0 {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &egress.Response{StatusCode: code}, nil
}

func newService(t *testing.T) (*monitor.Service, store.Store, *fakePoster, *time.Time) {
	t.Helper()
	cfg, st := testStore(t)
	w, _ := monitor.NewWebhooks(cfg, st.Settings())
	if err := w.Save(context.Background(), notify.Config{Preset: notify.Generic, URL: "https://hooks.lan/x"}); err != nil {
		t.Fatal(err)
	}
	lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	poster := &fakePoster{}
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	svc := &monitor.Service{Store: st, Webhooks: w, Logger: lg, AppURL: "https://pulse.lan",
		Notifier: &notify.Notifier{Post: poster, Backoff: []time.Duration{time.Millisecond}},
		Now:      func() time.Time { return now }}
	return svc, st, poster, &now
}

func observe(svc *monitor.Service, id string, state poller.State, at time.Time) {
	svc.Observe(context.Background(), poller.Observation{Target: poller.Target{ID: id, Name: "KyVault"},
		Result: poller.Result{State: state, Cause: "refused"}, At: at, Latency: 12 * time.Millisecond})
}

func TestDueHonoursIntervalAndEnabled(t *testing.T) {
	svc, st, _, now := newService(t)
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "a", URL: "https://a/", IntervalSec: 30, Enabled: true})
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "b", Name: "b", URL: "https://b/", IntervalSec: 30, Enabled: false})
	due, err := svc.Due(ctx, *now)
	if err != nil || len(due) != 1 || due[0].ID != "a" || due[0].Interval != 30*time.Second {
		t.Fatalf("never polled: %+v %v", due, err)
	}
	observe(svc, "a", poller.OK, *now)
	if due, _ = svc.Due(ctx, now.Add(29*time.Second)); len(due) != 0 {
		t.Fatalf("inside the interval: %+v", due)
	}
	if due, _ = svc.Due(ctx, now.Add(30*time.Second)); len(due) != 1 {
		t.Fatalf("at the interval: %+v", due)
	}
}

func TestObserveRecordsStateTransitionsAndSends(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})

	observe(svc, "a", poller.OK, *now)
	tg, _ := st.Targets().GetTarget(ctx, "a")
	if tg.State != "ok" || tg.LastPolledAt == nil || tg.LastLatencyMS != 12 {
		t.Fatalf("after first ok: %+v", tg)
	}
	if poster.calls != 0 {
		t.Fatal("a fresh target coming up must not notify")
	}
	for i := 1; i <= 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	tg, _ = st.Targets().GetTarget(ctx, "a")
	if tg.State != "down" || tg.Cause != "refused" {
		t.Fatalf("after three downs: %+v", tg)
	}
	events, total, _ := st.Targets().ListEvents(ctx, "a", 0, 10)
	if total != 2 || events[0].ToState != "down" || !events[0].Notified || events[0].NotifyError != "" {
		t.Fatalf("events: %d %+v", total, events)
	}
	if poster.calls != 1 || poster.sent[0].App != "KyVault" || poster.sent[0].State != "down" || poster.sent[0].Previous != "ok" || poster.sent[0].Reason != "refused" || poster.sent[0].URL != "https://pulse.lan/#/apps/a" {
		t.Fatalf("sent: %d %+v", poster.calls, poster.sent)
	}
	st1, ok, _ := svc.Webhooks.Status(ctx)
	if !ok || !st1.OK {
		t.Fatalf("delivery status: %+v", st1)
	}

	// An hour later, still down: one reminder, recorded as an event.
	observe(svc, "a", poller.Down, now.Add(2*time.Hour))
	events, total, _ = st.Targets().ListEvents(ctx, "a", 0, 10)
	if total != 3 || !events[0].Reminder || poster.calls != 2 || !poster.sent[1].Reminder {
		t.Fatalf("reminder: %d %+v calls=%d", total, events[0], poster.calls)
	}
}

func TestObserveRecordsAFailedSend(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	poster.codes = []int{503, 503}
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})
	for i := 0; i < 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	events, _, _ := st.Targets().ListEvents(ctx, "a", 0, 1)
	if events[0].Notified || events[0].NotifyError == "" {
		t.Fatalf("failed send not recorded: %+v", events[0])
	}
	s, ok, _ := svc.Webhooks.Status(ctx)
	if !ok || s.OK || s.Error == "" {
		t.Fatalf("delivery status: %+v", s)
	}
	tg, _ := st.Targets().GetTarget(ctx, "a")
	if tg.State != "down" {
		t.Fatal("a failed send must not stop the state from being recorded")
	}
}

func TestSilenceStopsSendsAndUntilFixedClears(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})
	observe(svc, "a", poller.OK, *now)
	if err := svc.Silence(ctx, "a", 0, true); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	tg, _ := st.Targets().GetTarget(ctx, "a")
	if tg.State != "down" || poster.calls != 0 || !monitor.Track(tg).UntilFixed {
		t.Fatalf("silenced down: state=%s calls=%d track=%+v", tg.State, poster.calls, monitor.Track(tg))
	}
	observe(svc, "a", poller.OK, now.Add(4*30*time.Second))
	observe(svc, "a", poller.OK, now.Add(5*30*time.Second))
	tg, _ = st.Targets().GetTarget(ctx, "a")
	if tg.State != "ok" || poster.calls != 1 || poster.sent[0].State != "ok" || monitor.Track(tg).UntilFixed {
		t.Fatalf("recovery: state=%s calls=%d track=%+v", tg.State, poster.calls, monitor.Track(tg))
	}
	if err := svc.Silence(ctx, "a", time.Hour, false); err != nil {
		t.Fatal(err)
	}
	if tr := monitor.Track(func() *store.Target { x, _ := st.Targets().GetTarget(ctx, "a"); return x }()); !tr.SilencedUntil.Equal(now.Add(time.Hour)) {
		t.Fatalf("timed silence: %+v", tr)
	}
	if err := svc.Silence(ctx, "missing", time.Hour, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing target: %v", err)
	}
}

func TestSendTestRecordsStatus(t *testing.T) {
	svc, _, poster, _ := newService(t)
	if err := svc.SendTest(context.Background()); err != nil || poster.calls != 1 || !poster.sent[0].Test {
		t.Fatalf("test send: %v calls=%d %+v", err, poster.calls, poster.sent)
	}
	s, ok, _ := svc.Webhooks.Status(context.Background())
	if !ok || !s.OK {
		t.Fatalf("status: %+v", s)
	}
}

func TestObserveIgnoresADeletedTarget(t *testing.T) {
	svc, _, poster, now := newService(t)
	observe(svc, "gone", poller.Down, *now) // must not panic or send
	if poster.calls != 0 {
		t.Fatal("sent for a target that does not exist")
	}
}
```

  Add `"encoding/json"` to the imports (the fake decodes the body); the `alerts` import is unused in this file — drop it.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/monitor/ -run 'TestDue|TestObserve|TestSilence|TestSendTest'`
Expected: FAIL, `undefined: monitor.Service`.

- [ ] **Step 3: Write `internal/monitor/service.go`**

```go
package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/alerts"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

var (
	evStateChanged   = logging.DeclareEvent("target_state_changed", "watched app changed state", slog.LevelWarn)
	evAlertSent      = logging.DeclareEvent("alert_sent", "alert delivered to the webhook", slog.LevelInfo)
	evAlertFailed    = logging.DeclareEvent("alert_send_failed", "alert could not be delivered", slog.LevelWarn)
	evPollStoreError = logging.DeclareEvent("poll_store_error", "poll result could not be stored", slog.LevelError)
	fState           = logging.DeclareString("state")
	fPrevious        = logging.DeclareString("previous")
)

// sendBudget bounds one delivery, retries included, after the poll's own context is gone.
const sendBudget = 60 * time.Second

// ErrNoWebhook is SendTest's answer when nothing is configured; the API maps it to 412.
var ErrNoWebhook = errors.New("monitor: no webhook is configured")

// Service is the monitoring loop's brain. One per process.
type Service struct {
	Store    store.Store
	Webhooks *Webhooks
	Notifier *notify.Notifier
	Logger   *logging.Logger
	AppURL   string
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Track decodes a target's stored track. A target that has never been polled is pending.
func Track(t *store.Target) alerts.Track {
	var tr alerts.Track
	if t.TrackJSON != "" {
		_ = json.Unmarshal([]byte(t.TrackJSON), &tr)
	}
	if tr.State == "" {
		tr.State, tr.Since = alerts.Pending, t.CreatedAt
	}
	return tr
}

// Due lists enabled targets whose interval has elapsed since their last poll. Computed here,
// not in SQL, so both dialects share one rule and the list is small anyway.
func (s *Service) Due(ctx context.Context, now time.Time) ([]poller.Target, error) {
	all, err := s.Store.Targets().ListTargets(ctx)
	if err != nil {
		return nil, err
	}
	var due []poller.Target
	for _, t := range all {
		if !t.Enabled {
			continue
		}
		interval := time.Duration(t.IntervalSec) * time.Second
		if t.LastPolledAt != nil && now.Before(t.LastPolledAt.Add(interval)) {
			continue
		}
		due = append(due, poller.Target{ID: t.ID, Name: t.Name, URL: t.URL, Interval: interval})
	}
	return due, nil
}

// Observe applies one poll: state machine, persistence, event, webhook. The store is written
// before anything is sent, so a failed send never loses the state change.
func (s *Service) Observe(ctx context.Context, o poller.Observation) {
	tg, err := s.Store.Targets().GetTarget(ctx, o.Target.ID)
	if err != nil {
		return // deleted while in flight
	}
	track, tr := alerts.Next(Track(tg), o.Result, o.At)
	decision, track := alerts.Decide(track, tr, o.At)

	trackJSON, _ := json.Marshal(track)
	resultJSON, _ := json.Marshal(o.Result)
	err = s.Store.Targets().RecordPoll(ctx, tg.ID, store.PollUpdate{
		PolledAt: o.At, LatencyMS: o.Latency.Milliseconds(), LastResult: string(resultJSON),
		TrackJSON: string(trackJSON), State: string(track.State), StateSince: track.Since, Cause: track.Cause,
	})
	if err != nil {
		s.Logger.Log(ctx, evPollStoreError, logging.TargetID(tg.ID), logging.Err(err))
		return
	}
	if tr == nil && !decision.Send {
		return
	}
	ev := &store.TargetEvent{TargetID: tg.ID, At: o.At, FromState: string(track.State), ToState: string(track.State), Cause: track.Cause, Reminder: decision.Reminder}
	if tr != nil {
		ev.FromState, ev.ToState, ev.Cause = string(tr.From), string(tr.To), tr.Cause
		s.Logger.Log(ctx, evStateChanged, logging.TargetID(tg.ID), fState(string(tr.To)), fPrevious(string(tr.From)), logging.ReasonCode(tr.Cause))
	}
	if err := s.Store.Targets().RecordEvent(ctx, ev); err != nil {
		s.Logger.Log(ctx, evPollStoreError, logging.TargetID(tg.ID), logging.Err(err))
		return
	}
	if !decision.Send {
		return
	}
	msg := notify.Message{App: tg.Name, State: ev.ToState, Previous: ev.FromState, Reason: ev.Cause,
		Time: o.At, URL: s.AppURL + "/#/apps/" + tg.ID, Reminder: decision.Reminder}
	s.deliver(ctx, tg.ID, ev.ID, msg)
}

// deliver sends one message on a context detached from the poll, records the outcome on the
// event and in the delivery status, and audits it. It never returns an error: the send's
// result is data, not a failure of the observation.
func (s *Service) deliver(ctx context.Context, targetID string, eventID int64, msg notify.Message) {
	cfg, ok, err := s.Webhooks.Load(ctx)
	if err != nil || !ok {
		return // no webhook configured: nothing to send, nothing to record
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendBudget)
	defer cancel()
	sendErr := s.Notifier.Send(sendCtx, cfg, msg)
	status := DeliveryStatus{At: s.now(), OK: sendErr == nil}
	action, details := "alert.sent", "target=" + targetID + " state=" + msg.State
	if sendErr != nil {
		status.Error = recoveryclient.AuditSafe(sendErr.Error())
		action, details = "alert.send_failed", details+" error="+status.Error
		s.Logger.Log(ctx, evAlertFailed, logging.TargetID(targetID), fState(msg.State), logging.Err(sendErr))
	} else {
		s.Logger.Log(ctx, evAlertSent, logging.TargetID(targetID), fState(msg.State))
	}
	_ = s.Webhooks.SetStatus(sendCtx, status)
	if eventID != 0 {
		_ = s.Store.Targets().SetEventNotified(sendCtx, eventID, sendErr == nil, status.Error)
	}
	_ = s.Store.Audit().LogAudit(sendCtx, &store.AuditRecord{UserID: "system", Action: action, Resource: targetID, Details: details})
}

// Silence edits only the track's silence fields, so a poll landing in between loses nothing.
func (s *Service) Silence(ctx context.Context, id string, d time.Duration, untilFixed bool) error {
	tg, err := s.Store.Targets().GetTarget(ctx, id)
	if err != nil {
		return err
	}
	track := Track(tg)
	track.UntilFixed = untilFixed
	track.SilencedUntil = time.Time{}
	if d > 0 {
		track.SilencedUntil = s.now().Add(d)
	}
	b, _ := json.Marshal(track)
	return s.Store.Targets().SetTrack(ctx, id, string(b))
}

// SendTest delivers a test message and records the delivery status. The error is returned
// as well, so the admin's request can say what went wrong.
func (s *Service) SendTest(ctx context.Context) error {
	cfg, ok, err := s.Webhooks.Load(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoWebhook
	}
	msg := notify.Message{App: "kyPulse", State: "ok", Previous: "ok", Time: s.now(), URL: s.AppURL, Test: true}
	sendErr := s.Notifier.Send(ctx, cfg, msg)
	status := DeliveryStatus{At: s.now(), OK: sendErr == nil}
	if sendErr != nil {
		status.Error = recoveryclient.AuditSafe(sendErr.Error())
	}
	_ = s.Webhooks.SetStatus(ctx, status)
	return sendErr
}
```

- [ ] **Step 4: Run the monitor tests under race**

Run: `gofmt -l internal/monitor; go vet ./internal/monitor/ && go test -race -count=3 ./internal/monitor/`
Expected: PASS. If `TestObserveRecordsStateTransitionsAndSends` fails on the reminder assertion, check that `Decide` receives `o.At` (not `time.Now()`): reminders are measured against the observation time.

- [ ] **Step 5: Write `internal/monitor/AGENTS.md`**

```markdown
# Monitor

## Purpose
Glues the poller, the alerts state machine and the notifier to the store: what is due, what an observation means, what is recorded, what is sent.

## Ownership
Owns `Service` (Due, Observe, Silence, SendTest), `Webhooks` (sealed webhook config and delivery status), the `alerts.Track`/`poller.Result` JSON stored on target rows, and the monitoring log events (`target_state_changed`, `alert_sent`, `alert_send_failed`, `poll_store_error`).

## Local Contracts
- Observe writes the poll result and the transition before sending; a failed send is recorded on the event and in `alert_webhook_status`, never lost.
- Sends run on a context detached from the poll with a 60 s budget; every send and failure is an audit row (`alert.sent`, `alert.send_failed`, actor `system`).
- The webhook is sealed under `kypulse:setting:alert_webhook`; `alert_webhook_status` is plaintext and holds no secret.
- Messages link to `AppURL/#/apps/<id>`; step 2c makes that route exist.
- Due is computed in Go from `ListTargets`, not in dialect-specific SQL.

## Verification
- `go test -race ./internal/monitor/`

## Child DOX Index
None.
```

- [ ] **Step 6: Commit**

```bash
git add internal/monitor
git commit -m "monitor: due targets, observations, transitions, delivery"
```

---

### Task 8: API routes

**Files:**
- Create: `internal/api/monitor_handlers.go`, `internal/api/monitor_test.go`
- Modify: `internal/api/server.go` (struct, `NewServer`, `routes()`, a `requireSession` wrapper), `internal/api/authz_test.go` (extend the admin table), `internal/api/AGENTS.md`

**Interfaces:**
- Consumes: `monitor.Service`, `monitor.Webhooks`, `monitor.Track`, `monitor.ErrNoWebhook`, `store.TargetStore`, `egress.ValidateURL`, `notify.Validate`, `notify.ErrBadPreset`, `egress.ErrScheme`, `egress.ErrRefusedAddress`.
- Produces: `NewServer(cfg, st, lg, mon *monitor.Service) *Server` and these routes:

| Method | Path | Auth | Handler | Response |
|---|---|---|---|---|
| GET | `/api/status` | session | `handleStatus` | `{checked_at, total, ok, degraded, down, pending, problems:[{id,name,state,since,cause}], webhook:{configured, last:{at,ok,error}}}` |
| GET | `/api/targets` | session | `handleListTargets` | `{targets:[target + silenced_until, until_fixed, basic]}` |
| GET | `/api/targets/{id}` | session | `handleGetTarget` | `{target, last_result, events:[last 20]}` |
| POST | `/api/targets` | admin | `handleCreateTarget` | 201 `{target}`; 400 on validation; 409 duplicate name |
| PUT | `/api/targets/{id}` | admin | `handleUpdateTarget` | `{target}` |
| DELETE | `/api/targets/{id}` | admin | `handleDeleteTarget` | 204 |
| POST | `/api/targets/{id}/silence` | admin | `handleSilence` | body `{"for":"1h"|"8h"|"until_fixed"|"off"}`; `{silenced_until, until_fixed}` |
| GET | `/api/alerts` | session | `handleListAlerts` | `{events:[...], total}` with `?target=&offset=&limit=` (limit ≤ 200, default 50) |
| GET | `/api/alerts/webhook` | admin | `handleGetWebhook` | `{configured, preset, url, has_token, last:{...}}` — never the token |
| PUT | `/api/alerts/webhook` | admin | `handleSetWebhook` | body `{preset,url,token}`; an empty `token` keeps the stored one; 400 on `notify.Validate` failure with the egress error text |
| DELETE | `/api/alerts/webhook` | admin | `handleDeleteWebhook` | 204 |
| POST | `/api/alerts/webhook/test` | admin | `handleTestWebhook` | `{ok:true}`; 412 no webhook; 502 with `{error}` when delivery failed |

- [ ] **Step 1: Write the failing tests** at `internal/api/monitor_test.go` (package `api_test`; reuse `setupTestServer`, `loginAs`, `do` and add a `doJSON` helper):

```go
package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func doJSON(t *testing.T, srv *api.Server, method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
		req.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "test-csrf"})
		req.Header.Set(auth.HeaderCSRF, "test-csrf")
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func decodeMap(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %d %s: %v", w.Code, w.Body.String(), err)
	}
	return out
}

func TestTargetCRUDAndValidation(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")

	w := doJSON(t, srv, "POST", "/api/targets", admin, map[string]any{"name": "KyVault", "url": "http://vault.lan/healthz", "interval_sec": 30})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	id := decodeMap(t, w)["target"].(map[string]any)["id"].(string)

	for _, bad := range []map[string]any{
		{"name": "KyVault", "url": "http://other.lan/healthz"},           // duplicate name
		{"name": "x", "url": "http://127.0.0.1/healthz"},                  // loopback
		{"name": "x", "url": "ftp://vault.lan/"},                          // scheme
		{"name": "x", "url": "http://vault.lan/", "interval_sec": 5},      // below the floor
		{"name": "bad name!", "url": "http://vault.lan/"},                 // name pattern
		{"name": "", "url": "http://vault.lan/"},                          // empty name
	} {
		w := doJSON(t, srv, "POST", "/api/targets", admin, bad)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusConflict {
			t.Errorf("%v: got %d, want 400/409: %s", bad, w.Code, w.Body.String())
		}
	}

	w = doJSON(t, srv, "PUT", "/api/targets/"+id, admin, map[string]any{"name": "KyVault prod", "url": "http://vault.lan/healthz", "interval_sec": 60, "enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	got, _ := st.Targets().GetTarget(context.Background(), id)
	if got.Name != "KyVault prod" || got.IntervalSec != 60 || got.Enabled {
		t.Fatalf("stored: %+v", got)
	}

	viewer := loginAs(t, srv, st, "bob", "viewer")
	list := decodeMap(t, do(t, srv, "GET", "/api/targets", viewer))
	if n := len(list["targets"].([]any)); n != 1 {
		t.Fatalf("viewer list: %v", list)
	}
	detail := decodeMap(t, do(t, srv, "GET", "/api/targets/"+id, viewer))
	if detail["target"].(map[string]any)["state"] != "pending" {
		t.Fatalf("detail: %v", detail)
	}
	if w := do(t, srv, "GET", "/api/targets/nope", viewer); w.Code != http.StatusNotFound {
		t.Fatalf("missing: %d", w.Code)
	}

	w = doJSON(t, srv, "POST", "/api/targets/"+id+"/silence", admin, map[string]string{"for": "8h"})
	if w.Code != http.StatusOK || decodeMap(t, w)["silenced_until"] == nil {
		t.Fatalf("silence: %d %s", w.Code, w.Body.String())
	}
	if w := doJSON(t, srv, "POST", "/api/targets/"+id+"/silence", admin, map[string]string{"for": "forever"}); w.Code != http.StatusBadRequest {
		t.Fatalf("bad silence: %d", w.Code)
	}

	if w := do(t, srv, "DELETE", "/api/targets/"+id, admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, "DELETE", "/api/targets/"+id, admin); w.Code != http.StatusNotFound {
		t.Fatalf("delete again: %d", w.Code)
	}
}

func TestStatusSummary(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	viewer := loginAs(t, srv, st, "bob", "viewer")
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "A", URL: "http://a/", IntervalSec: 30, Enabled: true, State: "down", Cause: "refused"})
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "b", Name: "B", URL: "http://b/", IntervalSec: 30, Enabled: true, State: "ok"})
	s := decodeMap(t, do(t, srv, "GET", "/api/status", viewer))
	if s["total"] != float64(2) || s["down"] != float64(1) || s["ok"] != float64(1) {
		t.Fatalf("summary: %v", s)
	}
	problems := s["problems"].([]any)
	if len(problems) != 1 || problems[0].(map[string]any)["name"] != "A" {
		t.Fatalf("problems: %v", problems)
	}
	if s["webhook"].(map[string]any)["configured"] != false {
		t.Fatalf("webhook: %v", s["webhook"])
	}
	if w := do(t, srv, "GET", "/api/status", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status: %d", w.Code)
	}
}

func TestWebhookTokenIsWriteOnly(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")
	w := doJSON(t, srv, "PUT", "/api/alerts/webhook", admin, map[string]string{"preset": "gotify", "url": "https://gotify.lan", "token": "gk-secret"})
	if w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/alerts/webhook", "/api/settings", "/api/status"} {
		w := do(t, srv, "GET", path, admin)
		if strings.Contains(w.Body.String(), "gk-secret") {
			t.Errorf("%s leaks the token: %s", path, w.Body.String())
		}
	}
	got := decodeMap(t, do(t, srv, "GET", "/api/alerts/webhook", admin))
	if got["configured"] != true || got["has_token"] != true || got["preset"] != "gotify" || got["url"] != "https://gotify.lan" {
		t.Fatalf("get: %v", got)
	}
	// An empty token on update keeps the stored one.
	doJSON(t, srv, "PUT", "/api/alerts/webhook", admin, map[string]string{"preset": "gotify", "url": "https://gotify2.lan", "token": ""})
	got = decodeMap(t, do(t, srv, "GET", "/api/alerts/webhook", admin))
	if got["url"] != "https://gotify2.lan" || got["has_token"] != true {
		t.Fatalf("after update: %v", got)
	}
	for _, bad := range []map[string]string{
		{"preset": "slack", "url": "https://x.lan"},
		{"preset": "ntfy", "url": "http://ntfy.lan/t"}, // http without KYPULSE_ALERT_ALLOW_HTTP
		{"preset": "ntfy", "url": "https://127.0.0.1/t"},
	} {
		if w := doJSON(t, srv, "PUT", "/api/alerts/webhook", admin, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%v: got %d", bad, w.Code)
		}
	}
	if w := do(t, srv, "DELETE", "/api/alerts/webhook", admin); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	if w := doJSON(t, srv, "POST", "/api/alerts/webhook/test", admin, nil); w.Code != http.StatusPreconditionFailed {
		t.Fatalf("test without webhook: %d %s", w.Code, w.Body.String())
	}
}

func TestViewerCannotWriteMonitoring(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	viewer := loginAs(t, srv, st, "bob", "viewer")
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/targets"}, {"PUT", "/api/targets/x"}, {"DELETE", "/api/targets/x"}, {"POST", "/api/targets/x/silence"},
		{"GET", "/api/alerts/webhook"}, {"PUT", "/api/alerts/webhook"}, {"DELETE", "/api/alerts/webhook"}, {"POST", "/api/alerts/webhook/test"},
	} {
		if w := do(t, srv, c.method, c.path, viewer); w.Code != http.StatusForbidden {
			t.Errorf("%s %s viewer: got %d", c.method, c.path, w.Code)
		}
		if w := do(t, srv, c.method, c.path, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: got %d", c.method, c.path, w.Code)
		}
	}
	for _, path := range []string{"/api/targets", "/api/alerts", "/api/status"} {
		if w := do(t, srv, "GET", path, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s anonymous: got %d", path, w.Code)
		}
	}
}
```

  Also extend the table in `TestPrivilegedEndpointsRequireAdmin` (`internal/api/authz_test.go`) with `{"POST", "/api/targets"}`, `{"GET", "/api/alerts/webhook"}`, `{"PUT", "/api/alerts/webhook"}`, `{"POST", "/api/alerts/webhook/test"}`.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/api/ -run 'TestTarget|TestStatus|TestWebhook|TestViewerCannot'`
Expected: FAIL (the routes fall through to the SPA and answer 200 HTML, or `setupTestServer` fails to compile once `NewServer` changes — either is the expected RED).

- [ ] **Step 3: Wire the service into the server.** In `internal/api/server.go`: add field `monitor *monitor.Service` to `Server`; change the constructor to `func NewServer(cfg *config.Config, st store.Store, lg *logging.Logger, mon *monitor.Service) *Server` and set the field. Update the three call sites: `setupTestServer` in `api_test.go` builds a real service —

```go
	webhooks, err := monitor.NewWebhooks(cfg, st.Settings())
	if err != nil {
		t.Fatalf("webhooks: %v", err)
	}
	mon := &monitor.Service{Store: st, Webhooks: webhooks, Logger: lg, AppURL: cfg.Server.AppURL,
		Notifier: &notify.Notifier{Post: egress.New(egress.Options{AllowHTTP: cfg.Alerts.AllowHTTP}), Backoff: nil}}
	srv := api.NewServer(cfg, st, lg, mon)
```

  and `backup_test.go`'s call passes `mon` the same way (factor a `newTestMonitor(t, cfg, st, lg)` helper in `api_test.go` and use it in both). `cmd/server/main.go` is Task 9.

  Add a session wrapper next to `requireAdmin`:

```go
// requireSession admits any signed-in user: viewers read status, targets and alerts.
func (s *Server) requireSession(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, _, err := s.sessions.AuthenticateRequest(r); err != nil {
			if errors.Is(err, auth.ErrPasswordChangeRequired) {
				s.writeJSON(w, http.StatusForbidden, map[string]string{"error": "Change your password before continuing", "code": "password_change_required"})
			} else {
				s.writeError(w, http.StatusUnauthorized, "Authentication required")
			}
			return
		}
		h(w, r)
	}
}
```

  And in `routes()`, after the backup block:

```go
	// Monitoring. Reads are for any session (viewers see status and alerts); writes and the
	// webhook are admin-only. The webhook token is write-only end to end.
	s.mux.HandleFunc("GET /api/status", s.requireSession(s.handleStatus))
	s.mux.HandleFunc("GET /api/targets", s.requireSession(s.handleListTargets))
	s.mux.HandleFunc("GET /api/targets/{id}", s.requireSession(s.handleGetTarget))
	s.mux.HandleFunc("POST /api/targets", s.requireAdmin(s.handleCreateTarget))
	s.mux.HandleFunc("PUT /api/targets/{id}", s.requireAdmin(s.handleUpdateTarget))
	s.mux.HandleFunc("DELETE /api/targets/{id}", s.requireAdmin(s.handleDeleteTarget))
	s.mux.HandleFunc("POST /api/targets/{id}/silence", s.requireAdmin(s.handleSilence))
	s.mux.HandleFunc("GET /api/alerts", s.requireSession(s.handleListAlerts))
	s.mux.HandleFunc("GET /api/alerts/webhook", s.requireAdmin(s.handleGetWebhook))
	s.mux.HandleFunc("PUT /api/alerts/webhook", s.requireAdmin(s.handleSetWebhook))
	s.mux.HandleFunc("DELETE /api/alerts/webhook", s.requireAdmin(s.handleDeleteWebhook))
	s.mux.HandleFunc("POST /api/alerts/webhook/test", s.tracked(s.requireAdmin(s.handleTestWebhook)))
```

- [ ] **Step 4: Write `internal/api/monitor_handlers.go`.** Requirements, with the shapes from the route table:

- Validation for create/update (a pure `validateTargetInput(in targetInput, allowHTTP bool) error` at the top of the file): `name` matches `^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$`; `url` passes `egress.ValidateURL(url, true)` (health targets may be plain http); `interval_sec` defaults to 30 when 0, must be 10..3600; `container` ≤ 128 chars. Input struct:

```go
type targetInput struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	IntervalSec int    `json:"interval_sec"`
	Enabled     *bool  `json:"enabled"` // nil on create means true
	Container   string `json:"container"`
}
```

- IDs are `"tgt_" + crypto.RandomHex(8)`.
- Responses render a target as `store.Target` plus `silenced_until`, `until_fixed` (from `monitor.Track`) and `basic` (from the decoded `LastResult`); write a `targetView(t *store.Target) map[string]any` helper used by list, get, create and update.
- Errors: `store.ErrAlreadyExists` → 409 "A target with that name exists"; `store.ErrNotFound` → 404; validation → 400 with the message; `egress.ErrRefusedAddress`/`egress.ErrScheme` → 400 with `err.Error()`.
- `handleSilence` maps `for`: `"1h"` → `Silence(id, time.Hour, false)`, `"8h"` → 8 h, `"until_fixed"` → `(0, true)`, `"off"` → `(0, false)`, anything else 400.
- `handleListAlerts`: `target`, `offset` (≥0), `limit` (1..200, default 50) query params → `ListEvents`.
- `handleStatus`: from `ListTargets` count states, list problems (state `down` or `degraded`, sorted down first then by name), `checked_at` = the newest `LastPolledAt` (null if none), and `webhook` from `Webhooks.Load` (configured only) and `Webhooks.Status`.
- `handleGetWebhook`: `Load` → `{configured:false}` or `{configured:true, preset, url, has_token: token != "", last: status-or-null}`.
- `handleSetWebhook`: decode `{preset,url,token}` (body capped at 4 KiB like `handleSetSchedule`); if `token == ""` and a config is stored, keep the stored token; `notify.Validate(cfg, s.config.Alerts.AllowHTTP)` → 400 with `err.Error()`; `Save`; audit `admin.webhook_set` with details `preset=<p> url=<u> allow_http=<bool>` (never the token); reply like `handleGetWebhook`.
- `handleDeleteWebhook`: `Delete`, audit `admin.webhook_delete`, 204.
- `handleTestWebhook`: `s.monitor.SendTest(r.Context())`; `monitor.ErrNoWebhook` → 412 "No webhook is configured"; other error → 502 `{error: AuditSafe(err)}`; success `{ok:true}`; audit `admin.webhook_test` with `outcome=success|failure`.
- Every admin write audits with `s.actorID(r)` (existing helper in `backup_handlers.go`) as `UserID`, `s.requestIP(r)` as `IPAddress`, the target ID as `Resource`, and a details string built with the existing `auditValue` quoting: actions `admin.target_create`, `admin.target_update`, `admin.target_delete`, `admin.target_silence`.
- No `log.Printf`; nothing to log here beyond audit rows.

- [ ] **Step 5: Run the API tests**

Run: `gofmt -l internal/api; go vet ./internal/api/ && go test -race -count=1 ./internal/api/`
Expected: PASS, including the extended `TestPrivilegedEndpointsRequireAdmin`.

- [ ] **Step 6: DOX.** `internal/api/AGENTS.md`: Purpose adds "monitoring routes"; add a "Monitoring" table with the twelve routes from the Interfaces block (method, path, auth, response one-liner); Local Contracts adds "- Viewer reads use `s.requireSession`; monitoring writes and the webhook use `s.requireAdmin`. The webhook token is write-only: never in `GET /api/alerts/webhook`, `/api/settings` (sealed `_enc` rows are filtered by suffix) or `/api/status`. Audit actions: `admin.target_create|update|delete|silence`, `admin.webhook_set|delete|test`; `alert.sent|send_failed` are written by `internal/monitor` with actor `system`."

- [ ] **Step 7: Commit**

```bash
git add internal/api
git commit -m "api: targets, alerts, status and webhook routes"
```

---

### Task 9: Wire the loop into the process

**Files:**
- Modify: `cmd/server/main.go`, `cmd/server/backuploop_test.go` (rename the wait test if its name no longer fits), `scripts/smoke-test.sh`, `AGENTS.md` (root), `docs/RESTORE.md`
- Create: `cmd/server/monitor.go`

**Interfaces:**
- Consumes: `monitor.Service`, `monitor.NewWebhooks`, `poller.Poller`, `egress.New`, `notify.Notifier`, `notify.DefaultBackoff`, `api.NewServer(cfg, st, lg, mon)`.
- Produces: `func newMonitor(cfg *config.Config, st store.Store, lg *logging.Logger) (*monitor.Service, *poller.Poller, error)` and `func monitorLoop(ctx context.Context, p *poller.Poller, done chan<- struct{})` in `cmd/server/monitor.go`.

- [ ] **Step 1: Write `cmd/server/monitor.go`**

```go
package main

import (
	"context"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// pollTick is how often due targets are looked for; intervals are per target (>= 10 s).
const pollTick = 5 * time.Second

// newMonitor builds the service and the poller that feeds it. Health polls use an egress
// client that admits plain http; the webhook client admits it only by opt-in.
func newMonitor(cfg *config.Config, st store.Store, lg *logging.Logger) (*monitor.Service, *poller.Poller, error) {
	webhooks, err := monitor.NewWebhooks(cfg, st.Settings())
	if err != nil {
		return nil, nil, err
	}
	svc := &monitor.Service{
		Store: st, Webhooks: webhooks, Logger: lg, AppURL: cfg.Server.AppURL,
		Notifier: &notify.Notifier{Post: egress.New(egress.Options{AllowHTTP: cfg.Alerts.AllowHTTP}), Backoff: notify.DefaultBackoff},
	}
	p := &poller.Poller{
		Get:     egress.New(egress.Options{AllowHTTP: true}),
		Workers: cfg.Poll.Workers,
		Due:     svc.Due,
		Observe: svc.Observe,
	}
	return svc, p, nil
}

// monitorLoop runs the poller until ctx ends and closes done once in-flight polls have
// finished, so runServer can close the store behind it.
func monitorLoop(ctx context.Context, p *poller.Poller, done chan<- struct{}) {
	defer close(done)
	p.Run(ctx, pollTick)
}
```

- [ ] **Step 2: Wire `runServer`.** In `cmd/server/main.go`:
  - After the bootstrap-admin block and before `api.NewServer`: `mon, pl, err := newMonitor(cfg, st, lg); if err != nil { fatal("Failed to build the monitor: %v", err) }`, then `srv := api.NewServer(cfg, st, lg, mon)`.
  - After `go backupLoop(...)`: `monitorDone := make(chan struct{}); go monitorLoop(ctx, pl, monitorDone)`.
  - In the shutdown sequence, `waitForBackupWork(waitCtx, backupDone, srv.WaitDetached)` becomes `waitForBackupWork(waitCtx, backupDone, monitorDone, srv.WaitDetached)`: extend the function's signature with `monitorDone <-chan struct{}` and add a third concurrent wait on it with the same `ctx` bound, logging `[KYPULSE] abandoning polls still running after %s` on expiry. The polls are bounded by egress's 5 s timeout plus the 60 s send budget, so they finish long before the 17-minute budget.
  - If `cfg.Alerts.AllowHTTP`, log once at startup, mirroring the backup opt-in line: `log.Printf("[ALERTS] KYPULSE_ALERT_ALLOW_HTTP is on: a plain-http webhook receiver is admitted (loopback and link-local remain refused)")`.
  - Update `cmd/server/backuploop_test.go`: `TestWaitForBackupWorkIsBoundedInBothPhases` and `TestWaitForBackupWorkWaitsBothAtOnce` pass a closed `monitorDone` channel; add `TestWaitForBackupWorkAlsoWaitsForTheMonitor` that leaves `monitorDone` open with the others closed and asserts the wait returns only at the ctx deadline.

- [ ] **Step 3: Smoke test.** In `scripts/smoke-test.sh`, inside the section that already logs in as admin and holds a session cookie (search for the CSRF/login helper used by the backup checks), add:

```bash
check "viewer-tier status needs a session" "$(status "$BASE/api/status")" "401"
check "admin creates a target" "$(status -X POST -H 'Content-Type: application/json' -b "$COOKIES" -H "X-CSRF-Token: $CSRF" -d '{"name":"self","url":"'"$BASE"'/healthz","interval_sec":10}' "$BASE/api/targets")" "201"
contains "targets list shows it" "$(curl -s -b "$COOKIES" "$BASE/api/targets")" '"name":"self"'
check "webhook must be https without the opt-in" "$(status -X PUT -H 'Content-Type: application/json' -b "$COOKIES" -H "X-CSRF-Token: $CSRF" -d '{"preset":"ntfy","url":"http://ntfy.lan/t"}' "$BASE/api/alerts/webhook")" "400"
```

  Use the script's actual variable names for the cookie jar and CSRF token (read the file; the backup checks show them). Note the self-target's URL is `127.0.0.1`, which egress refuses: that is fine for the smoke test (it exercises create/list, not polling) — but to prove the loop runs end to end, add after a short sleep: `contains "self target was polled and refused" "$(curl -s -b "$COOKIES" "$BASE/api/targets")" '"cause":"address_refused"'` with `sleep 12` before it (interval 10 s + tick 5 s worst case is 15 s; use `sleep 16`).

- [ ] **Step 4: Full verification**

```bash
gofmt -l $(git ls-files '*.go'); go vet ./... && go test -race -count=1 ./...
shellcheck scripts/*.sh && go build -o kypulse ./cmd/server && scripts/smoke-test.sh
cd web && npm test && npm run build && cd .. && git status --short web/dist   # must be empty: no UI change
```

Expected: everything green; `web/dist` unchanged.

- [ ] **Step 5: DOX and docs.** Root `AGENTS.md`: Ownership adds "- `internal/egress`, `internal/poller`, `internal/alerts`, `internal/notify`, `internal/monitor`: the monitoring backend; each has its own AGENTS.md." and the Child DOX Index gains five lines. Work Guidance's egress sentence drops "(planned in step 2b)". `docs/RESTORE.md`: one sentence in the "what a capsule holds" area: "Watched apps, their alert history and the sealed webhook are in the database and restore with it; the webhook opens only under the same encryption key."

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "monitor loop: poll watched apps and deliver alerts"
```

- [ ] **Step 7: Push and open the PR. Ask Yoshi first.**

```bash
git push -u origin feat/monitoring-backend
gh pr create --base master --title "monitoring backend: targets, poller, alerts, webhook" --body-file - <<'EOF'
Step 2b of the kyPulse design (`docs/superpowers/specs/2026-09-26-kypulse-design.md` §2).

- `internal/egress`: the one outbound client — LAN allowed, loopback/link-local/reserved refused at dial time, no redirects, 64 KiB body cap.
- `internal/poller`: the spec's response normaliser (fixtures are the suite's real answers) and a bounded worker pool that drains on shutdown.
- `internal/alerts`: 3/2/2 thresholds, hourly reminders, silences; pure.
- `internal/notify`: ntfy, Gotify, Discord and generic presets; tokens in headers only; retries on transport/429/5xx.
- `internal/monitor`: due targets, observations, transitions, delivery, sealed webhook storage and delivery status.
- Store: `targets` and `target_events` (migration 5). API: `/api/status`, `/api/targets*`, `/api/alerts*`; viewers read, admins write; webhook token write-only end to end.
- No UI in this step; step 2c builds the tabs on these routes.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
```

Register the PR with `link_pull_request` and drive CI to green with the `pull-request` skill.
