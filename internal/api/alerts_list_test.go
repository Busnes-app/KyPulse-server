package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

// /api/alerts pages by id: next_before_id leads to the older page, and 0 means no more.
func TestListAlertsKeysetPages(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	viewer := loginAs(t, srv, st, "vera", "viewer")
	ctx := context.Background()
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "a", URL: "https://a/", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if err := st.Targets().RecordEvent(ctx, &store.TargetEvent{TargetID: "a", At: time.Now(), FromState: "ok", ToState: "down"}); err != nil {
			t.Fatalf("event %d: %v", i, err)
		}
	}
	type page struct {
		Events []store.TargetEvent `json:"events"`
		Next   int64               `json:"next_before_id"`
	}
	get := func(path string) page {
		w := do(t, srv, "GET", path, viewer)
		var p page
		if err := json.Unmarshal(w.Body.Bytes(), &p); w.Code != 200 || err != nil {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
		return p
	}
	first := get("/api/alerts?limit=2")
	if len(first.Events) != 2 || first.Next != first.Events[1].ID || first.Events[0].ID <= first.Events[1].ID {
		t.Fatalf("first page: %+v", first)
	}
	second := get(fmt.Sprintf("/api/alerts?limit=2&before_id=%d", first.Next))
	if len(second.Events) != 1 || second.Next != 0 || second.Events[0].ID >= first.Next {
		t.Fatalf("second page: %+v", second)
	}
}
