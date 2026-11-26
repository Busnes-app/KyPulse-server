package egress

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Busnes-app/ky-primitives/pqtls"
)

func TestOutboundPQTLS(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	srv.StartTLS()
	defer srv.Close()

	t.Run("Get", func(t *testing.T) {
		c := handshakeClient(srv, "10.1.2.3")
		defer c.http.CloseIdleConnections()

		resp, err := c.Get(context.Background(), "https://example.com/healthz")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if resp.TLS == nil {
			t.Fatal("Get returned nil TLS state")
		}
		pqtls.RequireHybrid(t, *resp.TLS)
	})

	t.Run("Handshake", func(t *testing.T) {
		c := New(Options{})
		pool := x509.NewCertPool()
		pool.AddCert(srv.Certificate())
		c.tls.RootCAs = pool

		state, err := c.Handshake(context.Background(), srv.Listener.Addr().String(), "example.com", true)
		if err != nil {
			t.Fatalf("Handshake: %v", err)
		}
		pqtls.RequireHybrid(t, *state)
	})
}
