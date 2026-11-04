package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

// Q4 A: KyIdentity is the authority for an SSO user's role on every sign-in; a change
// ends the user's other sessions. Installer-owned identities keep the directory's role.
func TestSSOSignInAppliesKyIdentityRole(t *testing.T) {
	idp := newFakeIdP(t)
	t.Setenv("KYPULSE_KYSIGNON_ISSUER", idp.URL)
	t.Setenv("KYPULSE_KYSIGNON_CLIENT_ID", "pulse")
	t.Setenv("KYPULSE_SSO_AUTO_PROVISION", "true")
	srv, st, _ := setupTestServer(t)
	ctx := context.Background()
	role := func(sub string) string {
		u, err := st.Users().GetUserBySSO(ctx, "kysignon", sub)
		if err != nil {
			t.Fatal(err)
		}
		return u.Role
	}

	idp.role = "admin"
	first := sessionCookie(t, ssoLogin(srv, idp, "alice", "sid-1"))
	same := sessionCookie(t, ssoLogin(srv, idp, "alice", "sid-2"))
	if role("alice") != store.RoleAdmin || !signedIn(srv, first) {
		t.Fatal("admin not provisioned")
	}
	if !signedIn(srv, first) || !signedIn(srv, same) {
		t.Fatal("an unchanged role ended a session")
	}

	idp.role = "viewer"
	demoted := sessionCookie(t, ssoLogin(srv, idp, "alice", "sid-3"))
	if role("alice") != store.RoleViewer {
		t.Fatal("demotion in KyIdentity not applied at sign-in")
	}
	if signedIn(srv, first) || signedIn(srv, same) {
		t.Fatal("a session from before the role change survived")
	}
	if !signedIn(srv, demoted) {
		t.Fatal("the sign-in that applied the change has no session")
	}

	// An installer-owned identity keeps its role whatever the ID token says.
	owned := &store.User{ID: "usr_owned", Username: "owner", Role: store.RoleAdmin, Status: "active", SSOProvider: "kysignon", SSOSubject: "owner-sub"}
	if err := st.Users().CreateUser(ctx, owned); err != nil {
		t.Fatal(err)
	}
	if err := st.Settings().SetSetting(ctx, store.InstallerUserKey(idp.URL, "owner-sub"), `{}`); err != nil {
		t.Fatal(err)
	}
	if w := ssoLogin(srv, idp, "owner-sub", "sid-4"); !signedIn(srv, sessionCookie(t, w)) || role("owner-sub") != store.RoleAdmin {
		t.Fatal("sign-in changed an installer-owned identity's role")
	}
}

// KyIdentity sends the per-app `roles` claim to every client and the IdP-wide `role` only to
// legacy ones. `roles` decides; without it the legacy `role` does; with neither the stored
// role stays and no session ends. A malformed `roles` fails the sign-in.
func TestSSOSignInReadsPerAppRoles(t *testing.T) {
	idp := newFakeIdP(t)
	t.Setenv("KYPULSE_KYSIGNON_ISSUER", idp.URL)
	t.Setenv("KYPULSE_KYSIGNON_CLIENT_ID", "pulse")
	t.Setenv("KYPULSE_SSO_AUTO_PROVISION", "true")
	srv, st, _ := setupTestServer(t)
	role := func() string {
		u, err := st.Users().GetUserBySSO(context.Background(), "kysignon", "ivy")
		if err != nil {
			t.Fatal(err)
		}
		return u.Role
	}

	idp.roles = []string{"editor", "kypulse.admin"}
	first := sessionCookie(t, ssoLogin(srv, idp, "ivy", "s1"))
	if role() != store.RoleAdmin {
		t.Fatalf("provisioned as %q, want admin from roles", role())
	}
	// An IdP-wide admin without this app's admin role is a viewer here.
	idp.roles, idp.role = []string{}, "admin"
	second := sessionCookie(t, ssoLogin(srv, idp, "ivy", "s2"))
	if role() != store.RoleViewer || signedIn(srv, first) || !signedIn(srv, second) {
		t.Fatalf("roles without admin: role %q, old session kept %v", role(), signedIn(srv, first))
	}
	// No roles claim: the legacy role applies.
	idp.roles = nil
	third := sessionCookie(t, ssoLogin(srv, idp, "ivy", "s3"))
	if role() != store.RoleAdmin || signedIn(srv, second) {
		t.Fatalf("legacy role: %q, old session kept %v", role(), signedIn(srv, second))
	}
	// Neither claim: unchanged, and no session ends.
	idp.role = ""
	ssoLogin(srv, idp, "ivy", "s4")
	if role() != store.RoleAdmin || !signedIn(srv, third) {
		t.Fatalf("no role claims: role %q, other session kept %v", role(), signedIn(srv, third))
	}
	// A malformed roles claim fails the sign-in and changes nothing.
	idp.role = "viewer"
	for _, bad := range []any{"admin", map[string]any{"admin": true}, []any{1}, []any{nil}, []any{nil, "admin"}} {
		idp.roles = bad
		if w := ssoLogin(srv, idp, "ivy", "s5"); w.Code != http.StatusUnauthorized || role() != store.RoleAdmin || !signedIn(srv, third) {
			t.Fatalf("roles %v: status %d, role %q, other session kept %v", bad, w.Code, role(), signedIn(srv, third))
		}
	}
}
