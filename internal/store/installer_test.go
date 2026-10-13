package store_test

import (
	"context"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestInstallerOwnedIdentityAtomicCheckAndDrift(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530", Admins: []applysetup.Identity{{Issuer: "https://identity.test", Subject: "owner-sub", Username: "owner-admin"}}, Everyday: []applysetup.Identity{{Issuer: "https://identity.test", Subject: "daily-sub", Username: "everyday"}}}
	if _, err := st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, true); err == nil {
		t.Fatal("missing check succeeded")
	}
	if count, err := st.Users().CountUsers(ctx); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	r, err := st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, false)
	if err != nil || len(r.Created) != 2 {
		t.Fatal(r, err)
	}
	if _, err = st.Audit().VerifyChain(ctx); err != nil {
		t.Fatal("unkeyed installer audit", err)
	}
	_, before, err := st.Audit().ListAuditRecords(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []bool{true, false} {
		r, err = st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, check)
		if err != nil || len(r.Created) != 0 || len(r.Present) != 2 {
			t.Fatal(r, err)
		}
	}
	_, after, err := st.Audit().ListAuditRecords(ctx, 0, 100)
	if err != nil || before != after {
		t.Fatal("check/resume changed audit", before, after, err)
	}
	daily, err := st.Users().GetUserBySSO(ctx, "kysignon", "daily-sub")
	if err != nil || daily.Role != store.RoleViewer || daily.PasswordHash != "" {
		t.Fatal(daily, err)
	}
	daily.Status = "inactive"
	if err = st.Users().UpdateUser(ctx, daily); err != nil {
		t.Fatal(err)
	}
	if _, err = st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, false); err == nil {
		t.Fatal("reactivated drifted directory user")
	}
}

func TestInstallerForeignCollisionRollsBack(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	foreign := &store.User{ID: "foreign", Username: "daily", Role: store.RoleViewer, Status: "active", SSOProvider: "local", PasswordHash: "retained-password"}
	if err := st.Users().CreateUser(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	b := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530", Admins: []applysetup.Identity{{Issuer: "https://identity.test", Subject: "owner-sub", Username: "owner-admin"}}, Everyday: []applysetup.Identity{{Issuer: "https://identity.test", Subject: "daily-sub", Username: "DAILY"}}}
	if _, err := st.ApplyInstaller(ctx, config.InstallerBundle{Common: b}, false); err == nil {
		t.Fatal("adopted name collision")
	}
	if count, err := st.Users().CountUsers(ctx); err != nil || count != 1 {
		t.Fatal("partial accounts persisted", count, err)
	}
	got, err := st.Users().GetUserByID(ctx, foreign.ID)
	if err != nil || got.PasswordHash != foreign.PasswordHash {
		t.Fatal("changed foreign password", err)
	}
	all, err := st.Settings().GetAllSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for k := range all {
		if len(k) >= 10 && k[:10] == "installer." {
			t.Fatal("partial ownership persisted", k)
		}
	}
}
