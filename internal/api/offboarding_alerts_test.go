package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestOffboardingAlertPairingAndIngestBoundary(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	ctx := context.Background()
	target := &store.Target{ID: "bridge", Name: "Offboarding bridge", URL: "https://bridge.example/healthz", Enabled: true}
	if err := st.Targets().CreateTarget(ctx, target); err != nil {
		t.Fatal(err)
	}
	admin := loginAs(t, srv, st, "alert-admin", "admin")
	viewer := loginAs(t, srv, st, "alert-viewer", "viewer")
	call := func(path, body string, cookie *http.Cookie, bearer, sequence string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
			r.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "csrf"})
			r.Header.Set(auth.HeaderCSRF, "csrf")
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		if path == "/api/ingest/logs" {
			r.Header.Set("Content-Type", "application/x-ndjson")
		}
		if sequence != "" {
			r.Header.Set("Ky-Log-Sequence", sequence)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	const intent = `{"target_id":"bridge","offboarding_alerts":true}`
	for _, cookie := range []*http.Cookie{nil, viewer} {
		w := call("/api/log-sources/pairing", intent, cookie, "source-token", "")
		if w.Code != 401 && w.Code != 403 {
			t.Fatalf("non-admin granted alerts: %d", w.Code)
		}
	}
	for _, body := range []string{`{"offboarding_alerts":true}`, `{"target_id":"missing","offboarding_alerts":true}`} {
		if w := call("/api/log-sources/pairing", body, admin, "", ""); w.Code != 400 {
			t.Fatalf("missing target: %d %s", w.Code, w.Body)
		}
	}
	w := call("/api/log-sources/pairing", intent, admin, "", "")
	var pairing struct {
		Code string `json:"code"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &pairing) != nil || pairing.Code == "" {
		t.Fatalf("pairing: %d %s", w.Code, w.Body)
	}
	w = call("/api/log-sources/claim", `{"pairing_code":"`+pairing.Code+`","name":"bridge","target_id":"forged","offboarding_alerts":false}`, nil, "", "")
	var claim struct {
		Token  string          `json:"token"`
		Source store.LogSource `json:"source"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &claim) != nil || !claim.Source.OffboardingAlerts || claim.Source.TargetID != target.ID {
		t.Fatalf("claim changed admin scope: %d %s", w.Code, w.Body)
	}
	capability := httptest.NewRequest("OPTIONS", "/api/ingest/logs", nil)
	capability.Header.Set("Authorization", "Bearer "+claim.Token)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, capability)
	if w.Code != 204 || w.Header().Get("Ky-Log-Alerts") != "ky.offboarding-alerts/1" {
		t.Fatalf("alert capability missing: %d %v", w.Code, w.Header())
	}
	line, _ := json.Marshal(map[string]any{"app": "kyoffboard", "seq": 2, "hash": "audit", "fields": []string{}, "user_id": "kyoffboard", "resource": "bookstack:" + strings.Repeat("ab", 32), "action": "offboarding.failed", "outcome": "failed"})
	wire, _ := json.Marshal(map[string]string{"line": string(line)})
	body := string(wire) + "\n"
	if w := call("/api/ingest/logs", body, nil, claim.Token, ""); w.Code != 409 {
		t.Fatalf("unsequenced alert accepted: %d", w.Code)
	}
	if w := call("/api/ingest/logs", `{"line":"unstructured"}`+"\n", nil, claim.Token, "1"); w.Code != 400 || w.Header().Get("Ky-Log-Receipt") != "" {
		t.Fatalf("invalid audit accepted: %d", w.Code)
	}
	for range 2 {
		if w := call("/api/ingest/logs", body, nil, claim.Token, "1"); w.Code != 204 {
			t.Fatalf("ingest: %d %s", w.Code, w.Body)
		}
	}
	events, err := st.Targets().ListEvents(ctx, "", 0, 10)
	total := len(events)
	if err != nil || total != 1 || events[0].TargetID != target.ID || events[0].Cause != "offboarding_failed" || events[0].NotifyError != "pending" {
		t.Fatalf("native alerts: %+v %d %v", events, total, err)
	}
	// Existing session-authorized Alerts endpoint exposes the persisted failure.
	r := httptest.NewRequest("GET", "/api/alerts", nil)
	r.AddCookie(viewer)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "offboarding_failed") {
		t.Fatalf("failure absent from Alerts: %d %s", w.Code, w.Body)
	}
	if err := st.Targets().DeleteTarget(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, capability)
	if w.Code != 204 || w.Header().Get("Ky-Log-Alerts") != "" {
		t.Fatalf("deleted target retained alert capability: %d %v", w.Code, w.Header())
	}
	if w := call("/api/ingest/logs", body, nil, claim.Token, "2"); w.Code != 409 || w.Header().Get("Ky-Log-Receipt") != "" {
		t.Fatalf("lost scope acknowledged: %d", w.Code)
	}
}
