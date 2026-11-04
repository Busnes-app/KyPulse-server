package api_test

import (
	"context"
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
