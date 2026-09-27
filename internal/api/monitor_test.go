package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/auth"
	"github.com/Busnes-app/kypulse-server/internal/egress"
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
		{"name": "KyVault", "url": "http://other.lan/healthz"},       // duplicate name
		{"name": "x", "url": "http://127.0.0.1/healthz"},             // loopback
		{"name": "x", "url": "ftp://vault.lan/"},                     // scheme
		{"name": "x", "url": "http://vault.lan/", "interval_sec": 5}, // below the floor
		{"name": "bad name!", "url": "http://vault.lan/"},            // name pattern
		{"name": "", "url": "http://vault.lan/"},                     // empty name
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
	// A rename that omits interval_sec keeps the stored interval.
	if w := doJSON(t, srv, "PUT", "/api/targets/"+id, admin, map[string]any{"name": "KyVault", "url": "http://vault.lan/healthz"}); w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}
	if got, _ = st.Targets().GetTarget(context.Background(), id); got.Name != "KyVault" || got.IntervalSec != 60 {
		t.Fatalf("after rename: %+v", got)
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
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "c", Name: "C", URL: "http://c/", IntervalSec: 30, Enabled: false, State: "down", Cause: "refused"})
	s := decodeMap(t, do(t, srv, "GET", "/api/status", viewer))
	if s["total"] != float64(3) || s["down"] != float64(1) || s["ok"] != float64(1) || s["paused"] != float64(1) {
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

// A webhook row that exists but cannot be opened (rotated deployment key, corrupt store row)
// must still report configured:true: delivery is broken, not absent.
func TestStatusUnreadableWebhookIsConfigured(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")
	viewer := loginAs(t, srv, st, "bob", "viewer")
	ctx := context.Background()

	if w := doJSON(t, srv, "PUT", "/api/alerts/webhook", admin, map[string]string{"preset": "ntfy", "url": "https://ntfy.sh/kypulse"}); w.Code != http.StatusOK {
		t.Fatalf("save webhook: %d %s", w.Code, w.Body.String())
	}
	// Corrupt the sealed row directly so Webhooks.Load fails (mirrors
	// TestObserveRecordsAnUnreadableWebhook in internal/monitor).
	if err := st.Settings().SetSetting(ctx, "alert_webhook_enc", "not a sealed value"); err != nil {
		t.Fatalf("corrupt webhook row: %v", err)
	}

	s := decodeMap(t, do(t, srv, "GET", "/api/status", viewer))
	if s["webhook"].(map[string]any)["configured"] != true {
		t.Fatalf("webhook: %v", s["webhook"])
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
	put := func(body map[string]any) map[string]any {
		t.Helper()
		if w := doJSON(t, srv, "PUT", "/api/alerts/webhook", admin, body); w.Code != http.StatusOK {
			t.Fatalf("put %v: %d %s", body, w.Code, w.Body.String())
		}
		return decodeMap(t, do(t, srv, "GET", "/api/alerts/webhook", admin))
	}
	// Same preset and host, empty token: the stored token stays.
	if got := put(map[string]any{"preset": "gotify", "url": "https://gotify.lan/other", "token": ""}); got["has_token"] != true {
		t.Fatalf("same host: %v", got)
	}
	// A different host must not inherit it.
	if got := put(map[string]any{"preset": "gotify", "url": "https://gotify2.lan", "token": ""}); got["url"] != "https://gotify2.lan" || got["has_token"] != false {
		t.Fatalf("changed host: %v", got)
	}
	put(map[string]any{"preset": "gotify", "url": "https://gotify2.lan", "token": "gk-2"})
	if got := put(map[string]any{"preset": "gotify", "url": "https://gotify2.lan", "clear_token": true}); got["has_token"] != false {
		t.Fatalf("clear_token: %v", got)
	}
	rows, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 50)
	sets := 0
	for _, row := range rows {
		if row.Action == "admin.webhook_set" {
			sets++
		}
		if row.Action == "admin.webhook_set" && (strings.Contains(row.Details, "https://") || !strings.Contains(row.Details, `host="gotify`)) {
			t.Errorf("webhook_set audit must name the host only: %q", row.Details)
		}
	}
	if sets < 5 {
		t.Fatalf("webhook_set audit rows: %d", sets)
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

type leakyPoster struct{}

func (leakyPoster) Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error) {
	return nil, &url.Error{Op: "Post", URL: "https://secret-hook-host.example/t0ken", Err: errors.New("dial tcp: connection refused")}
}

func TestWebhookTestFailureNeverEchoesTheURL(t *testing.T) {
	srv, st, _ := setupTestServerWith(t, leakyPoster{})
	admin := loginAs(t, srv, st, "alice", "admin")
	if w := doJSON(t, srv, "PUT", "/api/alerts/webhook", admin, map[string]string{"preset": "ntfy", "url": "https://secret-hook-host.example/t0ken"}); w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	w := doJSON(t, srv, "POST", "/api/alerts/webhook/test", admin, nil)
	if w.Code != http.StatusBadGateway || strings.Contains(w.Body.String(), "secret-hook-host") || decodeMap(t, w)["error"] != "refused" {
		t.Fatalf("test send: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/status", "/api/alerts"} {
		if body := do(t, srv, "GET", path, admin).Body.String(); strings.Contains(body, "t0ken") {
			t.Errorf("%s carries the webhook URL: %s", path, body)
		}
	}
}
