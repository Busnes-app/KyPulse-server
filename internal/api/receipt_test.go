package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestIngestSequencedReceipt(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	token, source := pairedLogSource(t, st)
	body := "{\"line\":\"{\\\"seq\\\":1,\\\"hash\\\":\\\"audit\\\",\\\"action\\\":\\\"offboard\\\",\\\"fields\\\":[]}\"}\n"
	digest := sha256.Sum256([]byte(body))
	send := func(body string, sequences ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/ingest/logs", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/x-ndjson")
		for _, seq := range sequences {
			r.Header.Add("Ky-Log-Sequence", seq)
		}
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	for range 2 {
		w := send(body, "1")
		if w.Code != 204 || w.Header().Get("Ky-Log-Receipt") != "ky.log-ingest/1" || w.Header().Get("Ky-Log-Sequence") != "1" || w.Header().Get("Ky-Log-Digest") != hex.EncodeToString(digest[:]) {
			t.Fatalf("receipt: %d %v %s", w.Code, w.Header(), w.Body)
		}
	}
	ctx := context.Background()
	rows, err := st.Logs().List(ctx, store.LogFilter{})
	if err != nil || len(rows) != 1 || rows[0].SourceID != source.ID {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	activity, err := st.Logs().ListActivity(ctx, store.ActivityFilter{})
	if err != nil || len(activity) != 1 {
		t.Fatalf("activity: %+v %v", activity, err)
	}
	for _, tc := range []struct {
		body   string
		seq    []string
		status int
	}{
		{" " + body, []string{"1"}, 409}, {body, []string{"3"}, 409}, {body, nil, 409},
		{body, []string{"0"}, 400}, {body, []string{"01"}, 400}, {body, []string{"+1"}, 400},
		{body, []string{"9223372036854775808"}, 400}, {body, []string{"1", "1"}, 400},
		{"{bad}\n", []string{"2"}, 400}, {body, []string{"2"}, 204}, {body, []string{"1"}, 409},
	} {
		w := send(tc.body, tc.seq...)
		if w.Code != tc.status {
			t.Fatalf("sequence %v: %d want %d: %s", tc.seq, w.Code, tc.status, w.Body)
		}
		if w.Code != 204 && w.Header().Get("Ky-Log-Receipt") != "" {
			t.Fatal("refusal carried success receipt")
		}
	}
	if err := st.Sources().Revoke(ctx, source.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := send(body, "2"); w.Code != 401 || w.Header().Get("Ky-Log-Receipt") != "" {
		t.Fatalf("revoked replay: %d %v", w.Code, w.Header())
	}
}
