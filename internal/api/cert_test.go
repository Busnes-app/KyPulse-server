package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestTLSTargetValidation(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")
	for _, bad := range []map[string]any{
		{"name": "e1", "url": "tls://edge.lan"},                                             // no port
		{"name": "e2", "url": "tls://edge.lan:0"},                                           // bad port
		{"name": "e3", "url": "tls://127.0.0.1:443"},                                        // loopback without permission
		{"name": "e4", "url": "tls://[fe80::1]:443"},                                        // link-local without permission
		{"name": "e5", "url": "tls://169.254.169.254:443", "allow_local": true},             // metadata, even when allowed
		{"name": "e6", "url": "tls://edge.lan:443", "sni": "10.0.0.1"},                      // SNI must be a name
		{"name": "e7", "url": "tls://edge.lan:443", "sni": "bad_name!"},                     // SNI pattern
		{"name": "e8", "url": "tls://edge.lan:443", "sni": strings.Repeat("a.", 130) + "a"}, // oversize SNI
		{"name": "e9", "url": "https://app.lan/", "sni": "app.lan"},                         // SNI only on tls://
		{"name": "e10", "url": "https://app.lan/", "allow_local": true},                     // local only on tls://
	} {
		if w := doJSON(t, srv, "POST", "/api/targets", admin, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%v: got %d %s", bad, w.Code, w.Body.String())
		}
	}
	w := doJSON(t, srv, "POST", "/api/targets", admin, map[string]any{"name": "Edge", "url": "tls://127.0.0.1:8443", "sni": "edge.lan", "allow_local": true})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	id := decodeMap(t, w)["target"].(map[string]any)["id"].(string)
	got, _ := st.Targets().GetTarget(context.Background(), id)
	if got.SNI != "edge.lan" || !got.AllowLocal {
		t.Fatalf("stored %+v", got)
	}
	// Dropping the local permission while the address is loopback is refused.
	if w := doJSON(t, srv, "PUT", "/api/targets/"+id, admin, map[string]any{"name": "Edge", "url": "tls://127.0.0.1:8443"}); w.Code != http.StatusBadRequest {
		t.Fatalf("update without permission: %d", w.Code)
	}
	recs, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
	var audited bool
	for _, r := range recs {
		if r.Action == "admin.target_create" && strings.Contains(r.Details, "allow_local=true") {
			audited = true
		}
	}
	if !audited {
		t.Fatal("the local permission was not audited")
	}
}

func TestCertificatesListSortsBySoonestExpiry(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, tc := range []struct {
		id   string
		days int
	}{{"late", 80}, {"none", 0}, {"soon", 3}, {"mid", 15}} {
		res := `{"state":"ok"}`
		if tc.days != 0 {
			res = `{"state":"ok","cert":{"not_after":"` + now.Add(time.Duration(tc.days)*24*time.Hour).Format(time.RFC3339) + `","issuer":"CA","name_match":true}}`
		}
		if err := st.Targets().CreateTarget(ctx, &store.Target{ID: tc.id, Name: tc.id, URL: "https://" + tc.id + ".lan/", Enabled: true, LastResult: res}); err != nil {
			t.Fatal(err)
		}
	}
	viewer := loginAs(t, srv, st, "bob", "viewer")
	body := decodeMap(t, do(t, srv, "GET", "/api/certificates", viewer))
	var order []string
	for _, c := range body["certificates"].([]any) {
		order = append(order, c.(map[string]any)["id"].(string))
	}
	if strings.Join(order, ",") != "soon,mid,late" {
		t.Fatalf("order %v", order)
	}
	if body["warn_days"].(float64) != 21 || body["critical_days"].(float64) != 7 {
		t.Fatalf("defaults %v", body)
	}
	first := body["certificates"].([]any)[0].(map[string]any)
	if d := first["cert_days_left"].(float64); d != 2 && d != 3 {
		t.Fatalf("days left %v", d)
	}
	if do(t, srv, "GET", "/api/certificates", nil).Code != http.StatusUnauthorized {
		t.Fatal("anonymous read")
	}
}

func TestCertThresholds(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")
	for _, bad := range []map[string]any{{"warn_days": 7, "critical_days": 7}, {"warn_days": 500, "critical_days": 7}, {"warn_days": 21, "critical_days": 0}, {"warn_days": "x"}} {
		if w := doJSON(t, srv, "PUT", "/api/certificates/thresholds", admin, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, w.Code)
		}
	}
	if w := doJSON(t, srv, "PUT", "/api/certificates/thresholds", admin, map[string]any{"warn_days": 30, "critical_days": 10}); w.Code != http.StatusOK {
		t.Fatalf("set: %d %s", w.Code, w.Body.String())
	}
	body := decodeMap(t, do(t, srv, "GET", "/api/certificates", admin))
	if body["warn_days"].(float64) != 30 || body["critical_days"].(float64) != 10 {
		t.Fatalf("got %v", body)
	}
}
