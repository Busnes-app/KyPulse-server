package sso

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/crypto"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

// A stale delivery that passed the window check but waited behind the apply lock until its
// window closed must be refused, not allowed to prune the newer fence and apply.
func TestSyncWebhookRechecksTheWindowUnderTheLock(t *testing.T) {
	st, err := store.Open(context.Background(), testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const secret = "webhook-secret-999"
	k := NewKySignOnClient(config.SSOConfig{KySignOnHMACSecret: secret}, st)
	base := time.Now().Truncate(time.Second)
	send := func(p KySignOnSyncPayload) error {
		body, _ := json.Marshal(p)
		return k.HandleSyncWebhook(context.Background(), body, crypto.ComputeHMACSHA256(body, secret))
	}
	k.now = func() time.Time { return base.Add(time.Second) }
	demoted := KySignOnSyncPayload{Event: "user.updated", ID: "ext-race", Username: "ann", Role: "viewer", Status: "active", Timestamp: base.Add(time.Second).Unix()}
	if err := send(demoted); err != nil {
		t.Fatal(err)
	}
	// The promotion is checked at base+299s, then gets the lock at base+302s.
	calls := 0
	k.now = func() time.Time {
		calls++
		if calls == 1 {
			return base.Add(299 * time.Second)
		}
		return base.Add(302 * time.Second)
	}
	promoted := demoted
	promoted.Role, promoted.Timestamp = "admin", base.Unix()
	if err := send(promoted); err == nil {
		t.Fatal("a delivery whose window closed while it waited was applied")
	}
	if u, _ := st.Users().GetUserBySSO(context.Background(), "kysignon", "ext-race"); u.Role != store.RoleViewer {
		t.Fatalf("stale promotion applied: %q", u.Role)
	}
}

// The legacy format may end an installer-owned identity's access, but never promote,
// rename or reactivate it: the installer directory governs those.
func TestLegacySyncLeavesInstallerOwnedIdentitiesToTheDirectory(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	const secret, issuer = "webhook-secret-999", "https://identity.example.test"
	k := NewKySignOnClient(config.SSOConfig{KySignOnIssuer: issuer, KySignOnHMACSecret: secret}, st)
	owned := &store.User{ID: "usr_owned", Username: "everyday", Role: store.RoleViewer, Status: "active", SSOProvider: "kysignon", SSOSubject: "daily-sub"}
	if err := st.Users().CreateUser(ctx, owned); err != nil {
		t.Fatal(err)
	}
	if err := st.Settings().SetSetting(ctx, store.InstallerUserKey(issuer, "daily-sub"), `{}`); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Unix()
	send := func(p KySignOnSyncPayload) {
		t.Helper()
		at++
		p.ID, p.Timestamp = "daily-sub", at
		body, _ := json.Marshal(p)
		if err := k.HandleSyncWebhook(ctx, body, crypto.ComputeHMACSHA256(body, secret)); err != nil {
			t.Fatal(err)
		}
	}
	get := func() *store.User {
		u, err := st.Users().GetUserBySSO(ctx, "kysignon", "daily-sub")
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	send(KySignOnSyncPayload{Event: "user.updated", Username: "renamed", Role: "admin", Status: "active"})
	if u := get(); u.Role != store.RoleViewer || u.Username != "everyday" {
		t.Fatalf("legacy delivery changed an installer-owned identity: %+v", u)
	}
	send(KySignOnSyncPayload{Event: "user.deleted"})
	if u := get(); u.Status != "inactive" {
		t.Fatalf("legacy deletion did not end access: %q", u.Status)
	}
	send(KySignOnSyncPayload{Event: "user.updated", Username: "everyday", Role: "viewer", Status: "active"})
	if u := get(); u.Status != "inactive" {
		t.Fatal("legacy delivery reactivated an installer-owned identity")
	}
}
