package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

// TOTP state moves only by compare-and-swap: pending while off, enabled only for the pending
// secret checked, disabled only while on. Enable and disable keep the acting session, end the
// rest and every MFA challenge, and chain an audit row, all in one transaction.
func TestTOTPTransitionsAreCompareAndSwap(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u := &store.User{ID: "mfa", Username: "mfa", PasswordHash: "pw", Role: "viewer", Status: "active", SSOProvider: "local"}
	if err := st.Users().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	newSession := func(hash string) {
		t.Helper()
		if err := st.Sessions().CreateSession(ctx, &store.Session{TokenHash: hash, UserID: u.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, "pw"); err != nil {
			t.Fatal(err)
		}
	}
	alive := func(hash string) bool {
		_, err := st.Sessions().GetSession(ctx, hash)
		return err == nil
	}
	countAudit := func(action string) int {
		recs, _, err := st.Audit().ListAuditRecords(ctx, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, r := range recs {
			if r.Action == action && r.UserID == u.ID {
				n++
			}
		}
		return n
	}

	if err := st.Users().SetTOTPSecret(ctx, u.ID, "stale", "a"); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("set from a stale secret: %v", err)
	}
	if err := st.Users().SetTOTPSecret(ctx, u.ID, "", "a"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().SetTOTPSecret(ctx, u.ID, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "a", `["h"]`, "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("enable a replaced secret: %v", err)
	}

	newSession("keep")
	newSession("other")
	if err := st.Sessions().CreateMFAChallenge(ctx, &store.MFAChallenge{TokenHash: "ch", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}, "pw"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "b", `["h"]`, "keep", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	if !alive("keep") || alive("other") {
		t.Fatalf("enable: keep=%v other=%v", alive("keep"), alive("other"))
	}
	if _, _, err := st.Sessions().ConsumeMFAChallenge(ctx, "ch"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("challenge survived enable: %v", err)
	}
	if countAudit("auth.mfa_enabled") != 1 {
		t.Fatal("no auth.mfa_enabled audit row")
	}

	if err := st.Users().SetTOTPSecret(ctx, u.ID, "b", "c"); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("pending write over an enabled secret: %v", err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "b", `["x"]`, "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("enable twice: %v", err)
	}
	got, _ := st.Users().GetUserByID(ctx, u.ID)
	if !got.TOTPEnabled || got.TOTPSecretEnc != "b" || got.RecoveryCodesHash != `["h"]` {
		t.Fatalf("enabled state changed: %+v", got)
	}

	newSession("other2")
	if err := st.Users().DisableTOTP(ctx, u.ID, "keep", ""); err != nil {
		t.Fatal(err)
	}
	if !alive("keep") || alive("other2") {
		t.Fatalf("disable: keep=%v other=%v", alive("keep"), alive("other2"))
	}
	if err := st.Users().DisableTOTP(ctx, u.ID, "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("disable twice: %v", err)
	}
	got, _ = st.Users().GetUserByID(ctx, u.ID)
	if got.TOTPEnabled || got.TOTPSecretEnc != "" || got.RecoveryCodesHash != "[]" {
		t.Fatalf("disabled state: %+v", got)
	}
	if countAudit("auth.mfa_disabled") != 1 {
		t.Fatal("no auth.mfa_disabled audit row")
	}
	if _, err := st.Audit().VerifyChain(ctx); err != nil {
		t.Fatalf("audit chain after TOTP transitions: %v", err)
	}
}
