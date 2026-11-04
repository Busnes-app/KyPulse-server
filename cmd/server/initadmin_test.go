package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

// init-admin resets an admin, refuses to promote a viewer without -promote, and never
// touches an account that signs in through an identity provider.
func TestInitAdminPromotesOnlyWhenAsked(t *testing.T) {
	ctx := context.Background()
	st := openBootstrapStore(t)
	for _, u := range []*store.User{
		{ID: "usr_view", Username: "bob", Role: store.RoleViewer, Status: "active", SSOProvider: "local", PasswordHash: "old"},
		{ID: "usr_sso", Username: "sue", Role: store.RoleViewer, Status: "active", SSOProvider: "kysignon", SSOSubject: "s1"},
	} {
		if err := st.Users().CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := initAdmin(ctx, st, "bob", "new", false); err == nil || !strings.Contains(err.Error(), "-promote") {
		t.Fatalf("viewer without -promote: %v", err)
	}
	if u, _ := st.Users().GetUserByID(ctx, "usr_view"); u.Role != store.RoleViewer || u.PasswordHash != "old" {
		t.Fatalf("refused promotion changed the account: %+v", u)
	}
	if _, err := initAdmin(ctx, st, "sue", "new", true); err == nil {
		t.Fatal("SSO account was reset")
	}
	if msg, err := initAdmin(ctx, st, "bob", "new", true); err != nil || !strings.Contains(msg, "promoted") {
		t.Fatalf("promote: %q %v", msg, err)
	}
	if u, _ := st.Users().GetUserByID(ctx, "usr_view"); u.Role != store.RoleAdmin || u.PasswordHash != "new" {
		t.Fatalf("promotion not applied: %+v", u)
	}
	if msg, err := initAdmin(ctx, st, "bob", "newer", false); err != nil || !strings.Contains(msg, "reset") {
		t.Fatalf("admin reset: %q %v", msg, err)
	}
	if msg, err := initAdmin(ctx, st, "fresh", "pw", false); err != nil || !strings.Contains(msg, "created") {
		t.Fatalf("create: %q %v", msg, err)
	}
}
