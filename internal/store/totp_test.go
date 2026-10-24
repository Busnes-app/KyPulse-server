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
		cur, err := st.Users().GetUserByID(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Sessions().CreateSession(ctx, &store.Session{TokenHash: hash, UserID: u.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, cur.Credentials()); err != nil {
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
	if err := st.Users().EnableTOTP(ctx, u.ID, "pw", "a", `["h"]`, "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("enable a replaced secret: %v", err)
	}

	newSession("keep")
	newSession("other")
	if err := st.Sessions().CreateMFAChallenge(ctx, &store.MFAChallenge{TokenHash: "ch", UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}, "pw"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "pw", "b", `["h"]`, "keep", "192.0.2.1"); err != nil {
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
	if err := st.Users().EnableTOTP(ctx, u.ID, "pw", "b", `["x"]`, "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("enable twice: %v", err)
	}
	got, _ := st.Users().GetUserByID(ctx, u.ID)
	if !got.TOTPEnabled || got.TOTPSecretEnc != "b" || got.RecoveryCodesHash != `["h"]` {
		t.Fatalf("enabled state changed: %+v", got)
	}

	newSession("other2")
	if err := st.Users().DisableTOTP(ctx, u.ID, "pw", "keep", ""); err != nil {
		t.Fatal(err)
	}
	if !alive("keep") || alive("other2") {
		t.Fatalf("disable: keep=%v other=%v", alive("keep"), alive("other2"))
	}
	if err := st.Users().DisableTOTP(ctx, u.ID, "pw", "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
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

// A session is granted only for the TOTP state its sign-in checked, and a transition only under
// the password the handler checked and while the acting session still exists.
func TestTOTPStateBindsGrants(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u := &store.User{ID: "bind", Username: "bind", PasswordHash: "pw", Role: "viewer", Status: "active", SSOProvider: "local"}
	if err := st.Users().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	session := func(hash string, c store.Credentials) error {
		return st.Sessions().CreateSession(ctx, &store.Session{TokenHash: hash, UserID: u.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, c)
	}
	passwordOnly := store.Credentials{PasswordHash: "pw"}
	if err := session("keep", passwordOnly); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().SetTOTPSecret(ctx, u.ID, "", "s1"); err != nil {
		t.Fatal(err)
	}
	// A pending secret does not change what a password-only sign-in needs.
	if err := session("pending", passwordOnly); err != nil {
		t.Fatalf("password sign-in while enrolment is pending: %v", err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "changed", "s1", `["h"]`, "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("enable under a stale password: %v", err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "pw", "s1", `["h"]`, "gone", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("enable from a revoked session: %v", err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "pw", "s1", `["h"]`, "keep", ""); err != nil {
		t.Fatal(err)
	}
	// A password sign-in that read the account before TOTP came on gets nothing after.
	if err := session("late", passwordOnly); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("password-only session after enable: %v", err)
	}
	if err := session("mfa-stale", store.Credentials{PasswordHash: "pw", TOTPEnabled: true, TOTPSecretEnc: "s0"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("session for another secret: %v", err)
	}
	withTOTP := store.Credentials{PasswordHash: "pw", TOTPEnabled: true, TOTPSecretEnc: "s1"}
	if err := session("mfa", withTOTP); err != nil {
		t.Fatalf("session after the second factor: %v", err)
	}
	if err := st.Users().DisableTOTP(ctx, u.ID, "changed", "keep", ""); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("disable under a stale password: %v", err)
	}
	if err := st.Users().DisableTOTP(ctx, u.ID, "pw", "keep", ""); err != nil {
		t.Fatal(err)
	}
	if err := session("mfa-late", withTOTP); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second-factor session after disable: %v", err)
	}
}

// The operator reset turns TOTP off: a lost or hostile second factor must not outlive it.
func TestResetAdminPasswordClearsTOTP(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u := &store.User{ID: "adm", Username: "adm", PasswordHash: "pw", Role: "admin", Status: "active", SSOProvider: "local"}
	if err := st.Users().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.Sessions().CreateSession(ctx, &store.Session{TokenHash: "keep", UserID: u.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, u.Credentials()); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().SetTOTPSecret(ctx, u.ID, "", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "pw", "s1", `["h"]`, "keep", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().ResetAdminPassword(ctx, u.ID, "new"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Users().GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TOTPEnabled || got.TOTPSecretEnc != "" || got.RecoveryCodesHash != "[]" {
		t.Fatalf("TOTP survived the operator reset: %+v", got)
	}
}

// An operator reset between a confirm's authentication and its write wins: the confirm, holding
// the old password hash and a revoked session, fails and TOTP stays off. An expired acting
// session cannot carry a transition either.
func TestTOTPTransitionLosesToOperatorReset(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	u := &store.User{ID: "race", Username: "race", PasswordHash: "pw", Role: "admin", Status: "active", SSOProvider: "local"}
	if err := st.Users().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.Sessions().CreateSession(ctx, &store.Session{TokenHash: "stolen", UserID: u.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}, u.Credentials()); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().SetTOTPSecret(ctx, u.ID, "", "attacker"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().ResetAdminPassword(ctx, u.ID, "new"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "pw", "attacker", `["h"]`, "stolen", ""); err == nil {
		t.Fatal("confirm committed after the operator reset")
	}
	got, err := st.Users().GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TOTPEnabled || got.TOTPSecretEnc != "" {
		t.Fatalf("TOTP state after reset and late confirm: %+v", got)
	}

	// The reset password, with a pending secret again, but the acting session has expired.
	got.PasswordHash = "new"
	if err := st.Sessions().CreateSession(ctx, &store.Session{TokenHash: "old", UserID: u.ID, CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}, got.Credentials()); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().SetTOTPSecret(ctx, u.ID, "", "s1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().EnableTOTP(ctx, u.ID, "new", "s1", `["h"]`, "old", ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("enable from an expired session: %v", err)
	}
}
