package sender

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

func TestPairPreflightsAndPersists(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	called := 0
	token := hex.EncodeToString(make([]byte, 32))
	h := postFunc(func(body []byte) (*egress.Response, error) {
		called++
		if !strings.Contains(string(body), `"pairing_code":"123456"`) {
			t.Fatalf("body=%s", body)
		}
		return &egress.Response{StatusCode: 200, Body: []byte(`{"token":"` + token + `","source":{"id":"source-1"}}`)}, nil
	})
	state, err := Pair(context.Background(), h, dir, "https://example.com", "123456", "host-a")
	if err != nil || state.SourceID != "source-1" {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	loaded, secret, err := LoadState(dir)
	if err != nil || loaded.SourceID != "source-1" || len(secret) != 32 {
		t.Fatalf("loaded=%+v secret=%d err=%v", loaded, len(secret), err)
	}
	if _, err := Pair(context.Background(), h, dir, "https://example.com", "123456", "host-a"); err == nil || called != 1 {
		t.Fatalf("repaired: %v calls=%d", err, called)
	}
}

func TestPairRejectsUnsafeStateBeforeClaim(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("bad"), 0644); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err := Pair(context.Background(), postFunc(func([]byte) (*egress.Response, error) { called = true; return nil, nil }), dir, "https://example.com", "123456", "host")
	if err == nil || called {
		t.Fatalf("unsafe token: %v called=%v", err, called)
	}
}

func TestPairSaveFailureNamesRevocation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	token := hex.EncodeToString(make([]byte, 32))
	_, err := Pair(context.Background(), postFunc(func([]byte) (*egress.Response, error) {
		if err := os.Mkdir(filepath.Join(dir, "positions.json"), 0700); err != nil {
			t.Fatal(err)
		}
		return &egress.Response{StatusCode: 200, Body: []byte(`{"token":"` + token + `","source":{"id":"source-1"}}`)}, nil
	}), dir, "https://example.com", "123456", "host")
	if err == nil || !strings.Contains(err.Error(), "revoke the source and pair again") || strings.Contains(err.Error(), token) {
		t.Fatalf("failure=%v", err)
	}
}

// Browser regressions supply the exact --url emitted by the source UI. Exercise
// Pair's real URL preflight without claiming a source or assuming test-host TLS.
func TestPairScreenCommandOrigin(t *testing.T) {
	origin := os.Getenv("KYPULSE_TEST_SCREEN_PAIR_ORIGIN")
	if origin == "" {
		origin = "https://pulse.example.com:8443"
	}
	called := false
	h := postFunc(func([]byte) (*egress.Response, error) {
		called = true
		return &egress.Response{StatusCode: 200, Body: []byte(`{"token":"` + strings.Repeat("ab", 32) + `","source":{"id":"screen-source"}}`)}, nil
	})
	state, err := Pair(context.Background(), h, filepath.Join(t.TempDir(), "state"), origin, "123456", "screen-source")
	if err != nil || !called || state.URL != origin {
		t.Fatalf("screen origin %q: state=%+v called=%v err=%v", origin, state, called, err)
	}
}
