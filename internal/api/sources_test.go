package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/auth"
)

func TestLogSourceBoundaryRefusalsAudited(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "auditadmin", "admin")
	viewer := loginAs(t, srv, st, "auditviewer", "viewer")
	const marker = "credential-and-body-marker"
	const sourceToken = "source-bearer-token"
	hash := sha256.Sum256([]byte(sourceToken))
	if err := st.Sources().CreateCode(context.Background(), "audit-source-code", "", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Sources().Claim(context.Background(), "audit-source-code", hex.EncodeToString(hash[:]), "host"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		method, path, action, reason string
		cookie                       *http.Cookie
		bearer, body                 string
		csrf                         bool
		status                       int
	}{
		{"POST", "/api/log-sources/pairing", "admin.log_pairing", "authentication", nil, "", marker, false, 401},
		{"GET", "/api/log-sources", "admin.log_source_list", "authentication", nil, marker, "", false, 401},
		{"DELETE", "/api/log-sources/unknown", "admin.log_source_revoke", "authentication", nil, "", marker, false, 401},
		{"POST", "/api/log-sources/pairing", "admin.log_pairing", "role", viewer, "", marker, true, 403},
		{"GET", "/api/log-sources", "admin.log_source_list", "role", viewer, "", "", true, 403},
		{"DELETE", "/api/log-sources/unknown", "admin.log_source_revoke", "role", viewer, "", marker, true, 403},
		{"POST", "/api/log-sources/pairing", "admin.log_pairing", "authentication", nil, marker, marker, false, 401},
		{"GET", "/api/log-sources", "admin.log_source_list", "authentication", nil, sourceToken, "", false, 401},
		{"DELETE", "/api/log-sources/unknown", "admin.log_source_revoke", "csrf", admin, "", marker, false, 403},
		{"POST", "/api/log-sources/pairing", "admin.log_pairing", "csrf", admin, "", marker, false, 403},
		{"POST", "/api/log-sources/claim", "log_source.claim", "csrf", viewer, "", marker, false, 403},
	}
	for _, tc := range tests {
		t.Run(tc.method+tc.path+tc.reason, func(t *testing.T) {
			_, before, err := st.Audit().ListAuditRecords(context.Background(), 0, 200)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.cookie != nil {
				r.AddCookie(tc.cookie)
			}
			if tc.csrf {
				r.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "test-csrf"})
				r.Header.Set(auth.HeaderCSRF, "test-csrf")
			}
			if tc.bearer != "" {
				r.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d", w.Code, tc.status)
			}
			records, after, err := st.Audit().ListAuditRecords(context.Background(), 0, 200)
			if err != nil {
				t.Fatal(err)
			}
			if after != before+1 {
				t.Fatalf("audit count %d -> %d", before, after)
			}
			row := records[0]
			if row.Action != tc.action || row.Details != "outcome=refused reason="+tc.reason {
				t.Fatalf("audit action=%q details=%q", row.Action, row.Details)
			}
			if strings.Contains(row.Resource+row.Details, marker) {
				t.Fatalf("credential/body leaked in audit")
			}
		})
	}
	_, before, err := st.Audit().ListAuditRecords(context.Background(), 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/log-sources", nil)
	r.AddCookie(admin)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	_, after, err := st.Audit().ListAuditRecords(context.Background(), 0, 200)
	if err != nil || w.Code != 200 || after != before {
		t.Fatalf("successful list status=%d audit %d->%d err=%v", w.Code, before, after, err)
	}
}

func TestLogSourcePairingRoutes(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "sourceadmin", "admin")
	viewer := loginAs(t, srv, st, "sourceviewer", "viewer")
	call := func(method, path, body string, cookie *http.Cookie, bearer string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
			r.AddCookie(&http.Cookie{Name: auth.CSRFCookieName, Value: "test-csrf"})
			r.Header.Set(auth.HeaderCSRF, "test-csrf")
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("cache header %q", w.Header().Get("Cache-Control"))
		}
		return w
	}
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/api/log-sources/pairing", `{}`},
		{"GET", "/api/log-sources", ""},
		{"DELETE", "/api/log-sources/unknown", ""},
	} {
		for _, cookie := range []*http.Cookie{nil, viewer} {
			w := call(tc.method, tc.path, tc.body, cookie, "")
			want := http.StatusUnauthorized
			if cookie != nil {
				want = http.StatusForbidden
			}
			if w.Code != want {
				t.Fatalf("%s %s: got %d want %d", tc.method, tc.path, w.Code, want)
			}
		}
		if w := call(tc.method, tc.path, tc.body, nil, "source-token"); w.Code != http.StatusUnauthorized {
			t.Fatalf("bearer admin route=%d", w.Code)
		}
	}
	if w := call("POST", "/api/log-sources/pairing", `null`, admin, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("null=%d", w.Code)
	}
	csrfReq := httptest.NewRequest("POST", "/api/log-sources/pairing", strings.NewReader(`{}`))
	csrfReq.AddCookie(admin)
	csrfW := httptest.NewRecorder()
	srv.ServeHTTP(csrfW, csrfReq)
	if csrfW.Code != http.StatusForbidden {
		t.Fatalf("pairing CSRF=%d", csrfW.Code)
	}
	if w := call("POST", "/api/log-sources/pairing", `{} {}`, admin, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("two objects=%d", w.Code)
	}
	pair := call("POST", "/api/log-sources/pairing", `{}`, admin, "")
	if pair.Code != 200 {
		t.Fatalf("pair=%d %s", pair.Code, pair.Body.String())
	}
	var paired struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(pair.Body.Bytes(), &paired); err != nil || len(paired.Code) != 6 {
		t.Fatalf("pair=%s err=%v", pair.Body.String(), err)
	}
	claimBody, _ := json.Marshal(map[string]string{"pairing_code": paired.Code, "name": "host_1"})
	claim := call("POST", "/api/log-sources/claim", string(claimBody), nil, "")
	if claim.Code != 200 {
		t.Fatalf("claim=%d %s", claim.Code, claim.Body.String())
	}
	var claimed struct {
		Token  string `json:"token"`
		Source struct {
			ID string `json:"id"`
		} `json:"source"`
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &claimed); err != nil || len(claimed.Token) != 64 {
		t.Fatalf("claim=%s err=%v", claim.Body.String(), err)
	}
	for _, tc := range []struct {
		cookie *http.Cookie
		bearer string
	}{{admin, ""}, {viewer, ""}, {nil, claimed.Token}} {
		fresh := call("POST", "/api/log-sources/pairing", `{}`, admin, "")
		if fresh.Code != 200 {
			t.Fatalf("fresh pairing=%d", fresh.Code)
		}
		var p struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(fresh.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(map[string]string{"pairing_code": p.Code, "name": "host_2"})
		if w := call("POST", "/api/log-sources/claim", string(body), tc.cookie, tc.bearer); w.Code != 200 {
			t.Fatalf("claim authority=%d %s", w.Code, w.Body.String())
		}
	}
	if w := call("GET", "/api/log-sources", "", nil, claimed.Token); w.Code != http.StatusUnauthorized {
		t.Fatalf("real bearer on admin route=%d", w.Code)
	}
	if w := call("POST", "/api/log-sources/claim", string(claimBody), nil, ""); w.Code != http.StatusForbidden {
		t.Fatalf("replay=%d", w.Code)
	}
	list := call("GET", "/api/log-sources", "", admin, "")
	if list.Code != 200 || bytes.Contains(list.Body.Bytes(), []byte(claimed.Token)) || bytes.Contains(list.Body.Bytes(), []byte(paired.Code)) {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
	if w := call("DELETE", "/api/log-sources/"+claimed.Source.ID, "", admin, ""); w.Code != 200 {
		t.Fatalf("revoke=%d", w.Code)
	}
	if w := call("DELETE", "/api/log-sources/"+claimed.Source.ID, "", admin, ""); w.Code != 404 {
		t.Fatalf("revoke again=%d", w.Code)
	}
}

func TestLogSourceClaimLimiterIgnoresForgedProxy(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	for i := 0; i < 6; i++ {
		r := httptest.NewRequest("POST", "/api/log-sources/claim", strings.NewReader(`{"pairing_code":"000000","name":"host"}`))
		r.RemoteAddr = "198.51.100.9:1234"
		r.Header.Set("X-Forwarded-For", "203.0.113."+string(rune('0'+i)))
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		want := http.StatusForbidden
		if i == 5 {
			want = http.StatusTooManyRequests
			if w.Header().Get("Retry-After") == "" {
				t.Fatal("no Retry-After")
			}
		}
		if w.Code != want {
			t.Fatalf("attempt %d=%d", i+1, w.Code)
		}
	}
}

func TestLogSourceClaimGlobalLimiter(t *testing.T) {
	srv, _, _ := setupTestServer(t)
	for i := 0; i < 31; i++ {
		r := httptest.NewRequest("POST", "/api/log-sources/claim", strings.NewReader(`{"pairing_code":"000000","name":"host"}`))
		r.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", i+1)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		want := http.StatusForbidden
		if i == 30 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("attempt %d=%d", i+1, w.Code)
		}
	}
}
