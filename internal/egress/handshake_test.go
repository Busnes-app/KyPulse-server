package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateHostPort(t *testing.T) {
	for _, tc := range []struct {
		in    string
		local bool
		want  error
	}{
		{"edge.lan:443", false, nil},
		{"10.0.0.5:8443", false, nil},
		{"edge.lan", false, ErrScheme},
		{"edge.lan:0", false, ErrScheme},
		{"edge.lan:99999", false, ErrScheme},
		{"edge.lan:https", false, ErrScheme},
		{":443", false, ErrScheme},
		{"user@edge.lan:443", false, ErrScheme},
		{"edge.lan/x:443", false, ErrScheme},
		{"127.0.0.1:443", false, ErrRefusedAddress},
		{"[::1]:443", false, ErrRefusedAddress},
		{"169.254.10.1:443", false, ErrRefusedAddress},
		{"127.0.0.1:443", true, nil},
		{"169.254.10.1:443", true, nil},
		{"169.254.169.254:443", true, ErrRefusedAddress},
		{"169.254.170.2:443", true, ErrRefusedAddress},
		{"[fd00:ec2::254]:443", true, ErrRefusedAddress},
		{"0.0.0.0:443", true, ErrRefusedAddress},
		{"224.0.0.1:443", true, ErrRefusedAddress},
	} {
		if err := ValidateHostPort(tc.in, tc.local); !errors.Is(err, tc.want) {
			t.Errorf("%s local=%v: got %v want %v", tc.in, tc.local, err, tc.want)
		}
	}
}

// handshakeClient resolves every name to ip and dials srv, so the guard judges ip.
func handshakeClient(srv *httptest.Server, ip string) *Client {
	c := newClient(Options{},
		func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP(ip)}, nil },
		func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
		})
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	c.tls.RootCAs = pool
	return c
}

func TestHandshakeGuardsTheResolvedAddress(t *testing.T) {
	var requests int
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer srv.Close()
	for _, tc := range []struct {
		ip    string
		local bool
		ok    bool
	}{
		{"10.1.2.3", false, true},
		{"127.0.0.1", false, false},
		{"fe80::1", false, false},
		{"127.0.0.1", true, true},
		{"169.254.169.254", true, false},
	} {
		// httptest's certificate names example.com and 127.0.0.1.
		state, err := handshakeClient(srv, tc.ip).Handshake(context.Background(), "example.com:443", "", tc.local)
		if tc.ok && (err != nil || len(state.PeerCertificates) == 0) {
			t.Fatalf("%s local=%v: %v", tc.ip, tc.local, err)
		}
		if !tc.ok && !errors.Is(err, ErrRefusedAddress) {
			t.Fatalf("%s local=%v: want refusal, got %v", tc.ip, tc.local, err)
		}
	}
	if requests != 0 {
		t.Fatalf("a handshake sent %d HTTP requests", requests)
	}
}

func TestHandshakeReturnsTheChainOnAFailedVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	c := handshakeClient(srv, "10.1.2.3")
	_, err := c.Handshake(context.Background(), "example.com:443", "wrong.example", false)
	var verr *tls.CertificateVerificationError
	if !errors.As(err, &verr) || len(verr.UnverifiedCertificates) == 0 {
		t.Fatalf("got %v", err)
	}
}

func TestGetCarriesTheHandshake(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	c := handshakeClient(srv, "10.1.2.3")
	resp, err := c.Get(context.Background(), "https://example.com/healthz")
	if err != nil || resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		t.Fatalf("got %+v %v", resp, err)
	}
}
