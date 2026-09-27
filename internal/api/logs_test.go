package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/api"
	"github.com/Busnes-app/kypulse-server/internal/ingest"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func pairedLogSource(t *testing.T, st store.Store) (string, store.LogSource) {
	t.Helper()
	token := "log-token-" + strings.ReplaceAll(t.Name(), "/", "-")
	hash := sha256.Sum256([]byte(token))
	code := "log-code-" + token
	if err := st.Sources().CreateCode(context.Background(), code, "", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	source, err := st.Sources().Claim(context.Background(), code, hex.EncodeToString(hash[:]), "sender")
	if err != nil {
		t.Fatal(err)
	}
	return token, source
}

func ingestRequest(srv *api.Server, token string, body []byte, chunked bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/ingest/logs", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/x-ndjson; charset=utf-8")
	if chunked {
		r.ContentLength = -1
		r.TransferEncoding = []string{"chunked"}
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	return w
}

func TestIngestBatchAndLimits(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, source := pairedLogSource(t, st)
	for _, chunked := range []bool{false, true} {
		body := []byte("{\"line\":\"ok\",\"padding\":\"" + strings.Repeat("x", ingest.MaxRequestBytes-len("{\"line\":\"ok\",\"padding\":\"\"}\n")) + "\"}\n")
		if len(body) != ingest.MaxRequestBytes {
			t.Fatal(len(body))
		}
		if w := ingestRequest(srv, token, body, chunked); w.Code != 204 {
			t.Fatalf("exact size chunked=%v: %d %s", chunked, w.Code, w.Body.String())
		}
		if w := ingestRequest(srv, token, append(body, 'x'), chunked); w.Code != 413 {
			t.Fatalf("oversize chunked=%v: %d", chunked, w.Code)
		}
	}
	before, _ := st.Logs().List(context.Background(), store.LogFilter{})
	activityBefore, _ := st.Logs().ListActivity(context.Background(), store.ActivityFilter{})
	auditLine := `{"seq":1,"hash":"h","fields":[],"action":"sign-in","user_id":"alice"}`
	first, _ := json.Marshal(map[string]string{"line": auditLine})
	if w := ingestRequest(srv, token, append(first, []byte("\n{\"line\":7}\n")...), false); w.Code != 400 {
		t.Fatalf("malformed status=%d", w.Code)
	}
	after, _ := st.Logs().List(context.Background(), store.LogFilter{})
	if len(after) != len(before) {
		t.Fatal("malformed batch persisted rows")
	}
	activityAfter, _ := st.Logs().ListActivity(context.Background(), store.ActivityFilter{})
	if len(activityAfter) != len(activityBefore) {
		t.Fatal("malformed batch persisted activity")
	}
	if w := ingestRequest(srv, token, []byte("{\"line\":\"plain\",\"source\":\"forged\",\"target_id\":\"forged\"}\n"), false); w.Code != 204 {
		t.Fatalf("forged fields status=%d", w.Code)
	}
	rows, _ := st.Logs().List(context.Background(), store.LogFilter{Limit: 1})
	if rows[0].Source != "sender" || rows[0].SourceID != source.ID || rows[0].TargetID != "" {
		t.Fatalf("forged attribution: %+v", rows[0])
	}
	if w := ingestRequest(srv, token, []byte("{\"line\":\""+strings.Repeat("x", ingest.MaxLineBytes+1)+"\"}\n"), false); w.Code != 204 {
		t.Fatalf("line truncation status=%d", w.Code)
	}
	rows, _ = st.Logs().List(context.Background(), store.LogFilter{Limit: 1})
	if !rows[0].Truncated || len(rows[0].Raw) != ingest.MaxLineBytes {
		t.Fatalf("line not bounded: %+v", rows[0])
	}
}

func TestIngestRateLimitAndRevocation(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, source := pairedLogSource(t, st)
	for i := 0; i < 60; i++ {
		if w := ingestRequest(srv, token, []byte("{\"line\":\"ok\"}\n"), false); w.Code != 204 {
			t.Fatalf("request %d: %d", i, w.Code)
		}
	}
	if w := ingestRequest(srv, token, []byte("{\"line\":\"ok\"}\n"), false); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("61st: %d retry=%q", w.Code, w.Header().Get("Retry-After"))
	}
	if err := st.Sources().Revoke(context.Background(), source.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := ingestRequest(srv, token, []byte("{\"line\":\"ok\"}\n"), false); w.Code != 401 {
		t.Fatalf("revoked: %d", w.Code)
	}
}

type revokeOnRead struct {
	once   bool
	revoke func()
	body   io.Reader
}

func (r *revokeOnRead) Read(p []byte) (int, error) {
	if !r.once {
		r.once = true
		r.revoke()
	}
	return r.body.Read(p)
}

func TestIngestRevokedAfterAuthentication(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, source := pairedLogSource(t, st)
	reader := &revokeOnRead{body: strings.NewReader("{\"line\":\"too late\"}\n"), revoke: func() {
		if err := st.Sources().Revoke(context.Background(), source.ID, time.Now()); err != nil {
			t.Fatal(err)
		}
	}}
	r := httptest.NewRequest("POST", "/api/ingest/logs", reader)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/x-ndjson")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("post-auth revoke status=%d", w.Code)
	}
	rows, err := st.Logs().List(context.Background(), store.LogFilter{})
	if err != nil || len(rows) != 0 {
		t.Fatalf("revoked ingest stored %d rows: %v", len(rows), err)
	}
}

func TestIngestMediaAndBearerBoundary(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, _ := pairedLogSource(t, st)
	for _, tc := range []struct {
		bearer, media, encoding string
		want                    int
	}{
		{"Bearer wrong", "application/x-ndjson", "", 401},
		{"Basic " + token, "application/x-ndjson", "", 401},
		{"Bearer " + token, "application/json", "", 415},
		{"Bearer " + token, "application/x-ndjson", "gzip", 415},
	} {
		r := httptest.NewRequest("POST", "/api/ingest/logs", strings.NewReader("{\"line\":\"ok\"}\n"))
		r.Header.Set("Authorization", tc.bearer)
		r.Header.Set("Content-Type", tc.media)
		if tc.encoding != "" {
			r.Header.Set("Content-Encoding", tc.encoding)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Errorf("%+v: %d", tc, w.Code)
		}
	}
}

func TestLogReadsAreAdminOnlyAndSanitized(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "logsadmin", "admin")
	viewer := loginAs(t, srv, st, "logsviewer", "viewer")
	now := time.Now().UTC()
	for i := 0; i < 201; i++ {
		raw := fmt.Sprintf("entry-%03d", i)
		if i == 0 {
			raw = "literal %_ [31mred\x7f"
		}
		line := store.LogLine{Time: now, ReceivedAt: now, App: "app", Message: raw, Raw: raw}
		if err := st.Logs().Append(context.Background(), "", store.LogBatch{Logs: []store.LogLine{line}}, 1<<30); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/api/logs?target_id=valid", "/api/activity?target_id=valid"} {
		if w := do(t, srv, "GET", path, viewer); w.Code != 403 {
			t.Fatalf("viewer %s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/api/logs?unknown=x", "/api/logs?limit=201", "/api/logs?before_id=-1", "/api/logs?from=bad", "/api/logs?text=%00", "/api/activity?actor=%00", "/api/logs?from=2025-01-02T00:00:00Z&to=2025-01-01T00:00:00Z", "/api/activity?text=x"} {
		if w := do(t, srv, "GET", path, admin); w.Code != 400 {
			t.Errorf("invalid %s: %d", path, w.Code)
		}
	}
	w := do(t, srv, "GET", "/api/logs?limit=200", admin)
	var page struct {
		Items []store.LogLine `json:"items"`
		Next  int64           `json:"next_before_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 200 || page.Next != page.Items[199].ID {
		t.Fatalf("first page: %d %+v %v", w.Code, page, err)
	}
	if err := st.Logs().Append(context.Background(), "", store.LogBatch{Logs: []store.LogLine{{Time: now, ReceivedAt: now, Message: "new", Raw: "new"}}}, 1<<30); err != nil {
		t.Fatal(err)
	}
	w = do(t, srv, "GET", fmt.Sprintf("/api/logs?before_id=%d&limit=200", page.Next), admin)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || page.Next != 0 {
		t.Fatalf("second page: %d %+v %v", w.Code, page, err)
	}
	w = do(t, srv, "GET", "/api/logs?text=%25%5F", admin)
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Items) != 1 || strings.ContainsAny(page.Items[0].Raw, "\x1b\x7f") {
		t.Fatalf("literal/sanitize: %d %+v %v", w.Code, page, err)
	}
}

func TestLogRouteAuthenticationDenials(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, _ := pairedLogSource(t, st)
	for _, path := range []string{"/api/logs", "/api/activity"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("bearer admitted to %s: %d", path, w.Code)
		}
	}
	for _, action := range []string{"admin.log_list", "admin.activity_list"} {
		rows, _, err := st.Audit().ListAuditRecords(context.Background(), 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			if row.Action == action && row.Details == "outcome=refused reason=authentication" {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing audit denial %s", action)
		}
	}
}

func TestIngestNULNormalizesForBothBackends(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, _ := pairedLogSource(t, st)
	inner := map[string]any{"app": "a\x00b", "message": "m\x00n", "seq": 1, "hash": "h", "fields": []string{}, "action": "act\x00ion", "user_id": "u\x00ser"}
	raw, _ := json.Marshal(inner)
	for _, line := range []string{"plain\x00line", string(raw)} {
		wire, _ := json.Marshal(map[string]string{"line": line})
		if w := ingestRequest(srv, token, append(wire, '\n'), false); w.Code != 204 {
			t.Fatalf("NUL ingest status=%d %s", w.Code, w.Body.String())
		}
	}
	logs, err := st.Logs().List(context.Background(), store.LogFilter{})
	if err != nil || len(logs) != 2 {
		t.Fatalf("logs=%d err=%v", len(logs), err)
	}
	activity, err := st.Logs().ListActivity(context.Background(), store.ActivityFilter{})
	if err != nil || len(activity) != 1 {
		t.Fatalf("activity=%d err=%v", len(activity), err)
	}
	for _, value := range []string{logs[0].App, logs[0].Message, logs[1].Raw, activity[0].Actor, activity[0].Action} {
		if strings.ContainsRune(value, 0) {
			t.Fatalf("stored NUL in %q", value)
		}
	}
}
