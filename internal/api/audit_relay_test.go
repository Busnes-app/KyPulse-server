package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestIngestReceiptCapabilitySourceBoundary(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, source := pairedLogSource(t, st)
	request := func(headers http.Header) *httptest.ResponseRecorder {
		r := httptest.NewRequest("OPTIONS", "/api/ingest/logs", nil)
		r.Header = headers
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	w := request(http.Header{"Authorization": {"Bearer " + token}})
	if w.Code != 204 || w.Header().Get("Ky-Log-Receipt") != "ky.log-ingest/1" || w.Header().Get("Ky-Log-Source") != source.ID || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("capability: %d %v", w.Code, w.Header())
	}
	for _, headers := range []http.Header{{}, {"Authorization": {"Bearer " + token, "Bearer " + token}}, {"Authorization": {"Bearer invalid"}}} {
		w := request(headers)
		if w.Code != 401 || w.Header().Get("Ky-Log-Receipt") != "" {
			t.Fatalf("capability refusal: %d %v", w.Code, w.Header())
		}
	}
	w = request(http.Header{"Origin": {"https://foreign.example"}, "Access-Control-Request-Method": {"POST"}})
	if w.Code != 403 || w.Header().Get("Ky-Log-Source") != "" {
		t.Fatal("CORS exposed source identity")
	}
	if rows, err := st.Logs().List(context.Background(), store.LogFilter{}); err != nil || len(rows) != 0 {
		t.Fatal("capability wrote logs", err)
	}
	if err := st.Sources().Revoke(context.Background(), source.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := request(http.Header{"Authorization": {"Bearer " + token}}); w.Code != 401 || w.Header().Get("Ky-Log-Source") != "" {
		t.Fatal("revoked capability accepted")
	}
}

func TestOffboardAuditRelayNative(t *testing.T) {
	installer := os.Getenv("KYQ_TEST_INSTALLER_RELAY_ROOT")
	if installer == "" {
		t.Skip("explicit isolated installer fixture")
	}
	srv, st, _ := setupTestServer(t)
	token, source := pairedLogSource(t, st)
	var posts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			srv.ServeHTTP(w, r)
			return
		}
		n := posts.Add(1)
		if n != 1 {
			srv.ServeHTTP(w, r)
			return
		}
		// Commit through the real API/store, then lose the reply at the socket boundary.
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, r)
		if recorder.Code != 204 {
			t.Error("native initial batch refused", recorder.Code)
		}
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	defer server.Close()
	material, _ := json.Marshal(struct {
		Token       string
		Certificate []byte
	}{token, server.Certificate().Raw})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "test", "-race", "-count=1", "./internal/offboard", "-run", "^TestNativePulseAuditRelay$")
	command.Dir = installer
	command.Env = append(os.Environ(), "GOWORK=off", "KYQ_TEST_PULSE_RELAY_URL="+server.URL, "KYQ_TEST_PULSE_RELAY_MATERIAL="+string(material))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native relay fixture failed: %v\n%s", err, output)
	}
	rows, err := st.Logs().List(context.Background(), store.LogFilter{})
	if err != nil || len(rows) != 2 || posts.Load() != 3 {
		t.Fatalf("native replay rows=%d posts=%d err=%v", len(rows), posts.Load(), err)
	}
	activity, err := st.Logs().ListActivity(context.Background(), store.ActivityFilter{})
	if err != nil || len(activity) != 2 {
		t.Fatalf("native activity=%+v %v", activity, err)
	}
	for _, row := range rows {
		if row.SourceID != source.ID || row.Source != "sender" || row.App != "kyoffboard" {
			t.Fatal("native source attribution not retained")
		}
	}
	seen := map[string]bool{}
	for _, event := range activity {
		seen[event.Action] = true
		if event.SourceID != source.ID || event.Actor != "kyoffboard" {
			t.Fatal("native audit projection not recognized")
		}
	}
	if !seen["offboarding.started"] || !seen["offboarding.success"] {
		t.Fatal("native activity phases missing")
	}
}
