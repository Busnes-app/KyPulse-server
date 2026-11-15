package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/suitedir"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kypost"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

type kypostAnswer struct {
	code  int
	reply string
	calls int
}

func (k *kypostAnswer) Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error) {
	k.calls++
	return &egress.Response{StatusCode: k.code, Body: []byte(k.reply)}, nil
}

func pairedLink(k *kypostAnswer) *kypost.Link {
	l := &kypost.Link{Post: k, Lookup: func(context.Context) (suitedir.Peer, error) { return suitedir.Peer{}, errors.New("unused") }}
	l.Set(&kypost.Peer{URL: "https://mail.example", Secret: []byte(strings.Repeat("s", 32))})
	return l
}

func TestKyPostChannelHiddenWithoutKyIdentity(t *testing.T) {
	srv, st, _ := setupTestServer(t)
	admin := loginAs(t, srv, st, "alice", "admin")
	body := decodeMap(t, do(t, srv, "GET", "/api/alerts/kypost", admin))
	if body["available"] != false || body["reason"] != "no_kyidentity" {
		t.Fatalf("got %v", body)
	}
	if w := doJSON(t, srv, "PUT", "/api/alerts/kypost", admin, map[string]any{"owner_id": "usr_alice", "recipients": []string{"a@example.com"}}); w.Code != http.StatusConflict {
		t.Fatalf("put while unavailable: %d", w.Code)
	}
}

func TestKyPostChannelSetTestAndRefusal(t *testing.T) {
	k := &kypostAnswer{code: 409, reply: `{"error":"x","code":"recipient_unconfirmed"}`}
	srv, st, _ := setupTestServerLinked(t, nil, nil, pairedLink(k))
	ctx := context.Background()
	admin := loginAs(t, srv, st, "alice", "admin")
	if err := st.Users().CreateUser(ctx, &store.User{ID: "u_ops", Username: "ops", Role: "viewer", Status: "active", SSOProvider: "kysignon", SSOSubject: "sub-ops"}); err != nil {
		t.Fatal(err)
	}
	body := decodeMap(t, do(t, srv, "GET", "/api/alerts/kypost", admin))
	owners := body["owners"].([]any)
	if body["available"] != true || body["paired"] != true || len(owners) != 1 || owners[0].(map[string]any)["id"] != "u_ops" {
		t.Fatalf("got %v", body)
	}
	if w := doJSON(t, srv, "POST", "/api/alerts/kypost/test", admin, nil); w.Code != http.StatusPreconditionFailed {
		t.Fatalf("test before setup: %d", w.Code)
	}
	for _, bad := range []map[string]any{
		{"owner_id": "usr_alice", "recipients": []string{"a@example.com"}}, // a local account
		{"owner_id": "nobody", "recipients": []string{"a@example.com"}},
		{"owner_id": "u_ops", "recipients": []string{}},
		{"owner_id": "u_ops", "recipients": []string{"Ops <a@example.com>"}},
		{"owner_id": "u_ops", "recipients": []string{"a@example.com\r\nBcc: b@example.com"}},
	} {
		if w := doJSON(t, srv, "PUT", "/api/alerts/kypost", admin, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%v: %d %s", bad, w.Code, w.Body.String())
		}
	}
	w := doJSON(t, srv, "PUT", "/api/alerts/kypost", admin, map[string]any{"owner_id": "u_ops", "recipients": []string{"Ops@Example.com"}})
	if w.Code != http.StatusOK || decodeMap(t, w)["recipients"].([]any)[0] != "ops@example.com" {
		t.Fatalf("put: %d %s", w.Code, w.Body.String())
	}
	w = doJSON(t, srv, "POST", "/api/alerts/kypost/test", admin, nil)
	if w.Code != http.StatusBadGateway || decodeMap(t, w)["error"] != "recipient_unconfirmed" || k.calls != 1 {
		t.Fatalf("test: %d %s calls=%d", w.Code, w.Body.String(), k.calls)
	}
	last := decodeMap(t, do(t, srv, "GET", "/api/alerts/kypost", admin))["last"].(map[string]any)
	if last["ok"] != false || last["error"] != "recipient_unconfirmed" {
		t.Fatalf("last %v", last)
	}
	k.code, k.reply = 202, `{"outcome":"sent"}`
	if w := doJSON(t, srv, "POST", "/api/alerts/kypost/test", admin, nil); w.Code != http.StatusOK {
		t.Fatalf("test ok: %d", w.Code)
	}
	recs, _, _ := st.Audit().ListAuditRecords(ctx, 0, 50)
	for _, r := range recs {
		if strings.Contains(r.Details, "@") || strings.Contains(r.Resource, "@") {
			t.Fatalf("an address reached the audit: %+v", r)
		}
	}
	// A viewer cannot read or change the channel.
	viewer := loginAs(t, srv, st, "bob", "viewer")
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		if w := doJSON(t, srv, m, "/api/alerts/kypost", viewer, map[string]any{}); w.Code != http.StatusForbidden {
			t.Errorf("viewer %s: %d", m, w.Code)
		}
	}
	if w := doJSON(t, srv, "DELETE", "/api/alerts/kypost", admin, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", w.Code)
	}
	if decodeMap(t, do(t, srv, "GET", "/api/alerts/kypost", admin))["configured"] != false {
		t.Fatal("still configured")
	}
}

func TestKyPostChannelRecipientsNeverReachSettings(t *testing.T) {
	srv, st, _ := setupTestServerLinked(t, nil, nil, pairedLink(&kypostAnswer{code: 202}))
	admin := loginAs(t, srv, st, "alice", "admin")
	_ = st.Users().CreateUser(context.Background(), &store.User{ID: "u_ops", Username: "ops", Role: "admin", Status: "active", SSOProvider: "kysignon", SSOSubject: "s"})
	doJSON(t, srv, "PUT", "/api/alerts/kypost", admin, map[string]any{"owner_id": "u_ops", "recipients": []string{"secret-person@example.com"}})
	if w := do(t, srv, "GET", "/api/settings", admin); strings.Contains(w.Body.String(), "secret-person") {
		t.Fatal("recipients leaked into /api/settings")
	}
}
