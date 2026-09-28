package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func TestKyYardGuardedLogClientHasSeparateCap(t *testing.T) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	var ip string
	for _, address := range addresses {
		n, ok := address.(*net.IPNet)
		if ok && n.IP.To4() != nil && n.IP.IsPrivate() && !n.IP.IsLoopback() {
			ip = n.IP.String()
			break
		}
	}
	if ip == "" {
		t.Skip("no private IPv4 for production egress guard")
	}
	body := strings.Repeat("2026-09-27T12:00:00Z "+strings.Repeat("x", 4096)+"\n", 1000)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing bearer")
		}
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, body)
	}))
	listener, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		t.Fatal(err)
	}
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	ctx := context.Background()
	cfg := &config.Config{Database: testdb.Config(t)}
	cfg.Security.EncryptionKey = make([]byte, 32)
	cfg.KyYard.AllowHTTP = true
	st, err := store.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	lg, _ := logging.New(logging.Config{App: "test", Out: io.Discard})
	s, err := newKyYard(cfg, st, lg)
	if err != nil {
		t.Fatal(err)
	}
	c := &kyyard.Client{HTTP: s.HTTP, LogHTTP: s.LogHTTP, Config: kyyard.Config{URL: server.URL, OrganizationID: "org", Token: "token"}}
	page, err := c.Logs(ctx, "ep", "ct", "")
	if err != nil || len(page.Lines) != 1000 {
		t.Fatalf("32 MiB log client refused >2 MiB: rows=%d %v", len(page.Lines), err)
	}
	if _, err := c.Endpoints(ctx); kyyard.Reason(err) != "body_too_large" {
		t.Fatalf("inventory cap changed: %v", err)
	}
	// The production log client still refuses loopback, rather than bypassing the guard.
	c.Config.URL = "http://127.0.0.1:1"
	if _, err := c.Logs(ctx, "ep", "ct", ""); kyyard.Reason(err) != "address_refused" {
		t.Fatalf("loopback guard: %v", err)
	}
}
