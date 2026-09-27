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
		{"http://[fd00:ec2::254]/latest/meta-data", true, ErrRefusedAddress},
		{"http://100.100.100.200/latest/meta-data", true, ErrRefusedAddress},
		{"https://203.0.113.7/", false, ErrRefusedAddress},
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

func TestGetWithSendsHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Clone() }))
	defer srv.Close()
	c := testClient(t, srv, "10.0.0.5", Options{AllowHTTP: true})
	if _, err := c.GetWith(context.Background(), "http://vault.lan/x", map[string]string{"Authorization": "Bearer abc"}); err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer abc" || got.Get("User-Agent") != "kypulse" {
		t.Fatalf("headers: %v", got)
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
