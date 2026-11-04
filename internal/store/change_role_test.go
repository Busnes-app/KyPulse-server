package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/google/uuid"
)

func newUser(name string) *store.User {
	return &store.User{ID: uuid.NewString(), Username: name, Role: store.RoleViewer, Status: "active", SSOProvider: "local"}
}

// ChangeRole never undoes a deactivation that committed after the caller read the user.
func TestChangeRoleRefusesAMovedRow(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u := newUser("ann")
	if err := st.Users().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().ChangeRole(ctx, u.ID, store.RoleViewer, store.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().ChangeRole(ctx, u.ID, store.RoleViewer, store.RoleAdmin); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("stale from-role: got %v", err)
	}
	got, _ := st.Users().GetUserByID(ctx, u.ID)
	got.Status = "inactive"
	if err := st.Users().UpdateUser(ctx, got); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().ChangeRole(ctx, u.ID, store.RoleAdmin, store.RoleViewer); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("inactive user: got %v", err)
	}
	if got, _ := st.Users().GetUserByID(ctx, u.ID); got.Status != "inactive" {
		t.Fatal("ChangeRole reactivated a deactivated user")
	}
}
