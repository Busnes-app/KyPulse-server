package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
	"github.com/google/uuid"
)

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), testdb.Config(t))
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}

	t.Cleanup(func() {
		_ = st.Close()
	})

	return st
}

func TestUserStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	userID := uuid.NewString()
	user := &store.User{
		ID:           userID,
		Username:     "alice",
		Email:        "alice@busnes.app",
		DisplayName:  "Alice Admin",
		PasswordHash: "argon2id$mockedhash",
		Role:         "admin",
		Status:       "active",
		SSOProvider:  "local",
	}

	// 1. Create
	if err := st.Users().CreateUser(ctx, user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Duplicate should fail
	if err := st.Users().CreateUser(ctx, user); err != store.ErrAlreadyExists {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	// 2. GetByID
	got, err := st.Users().GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserByID error: %v", err)
	}
	if got.Username != "alice" || got.DisplayName != "Alice Admin" {
		t.Errorf("unexpected user data: %+v", got)
	}

	// 3. GetByUsername (case-insensitive)
	gotByU, err := st.Users().GetUserByUsername(ctx, "ALICE")
	if err != nil {
		t.Fatalf("GetUserByUsername error: %v", err)
	}
	if gotByU.ID != userID {
		t.Errorf("expected ID %s, got %s", userID, gotByU.ID)
	}

	// 4. GetByEmail
	gotByE, err := st.Users().GetUserByEmail(ctx, "alice@busnes.app")
	if err != nil {
		t.Fatalf("GetUserByEmail error: %v", err)
	}
	if gotByE.ID != userID {
		t.Errorf("expected ID %s, got %s", userID, gotByE.ID)
	}

	// 5. Update
	user.DisplayName = "Alice Operations"
	user.Role = store.RoleViewer
	if err := st.Users().UpdateUser(ctx, user); err != nil {
		t.Fatalf("UpdateUser error: %v", err)
	}
	gotUpdated, _ := st.Users().GetUserByID(ctx, userID)
	if gotUpdated.DisplayName != "Alice Operations" || gotUpdated.Role != store.RoleViewer {
		t.Errorf("update not reflected: %+v", gotUpdated)
	}

	// 6. List & Count
	users, count, err := st.Users().ListUsers(ctx, 0, 10, "alice")
	if err != nil {
		t.Fatalf("ListUsers error: %v", err)
	}
	if count != 1 || len(users) != 1 {
		t.Errorf("expected 1 user, got count=%d len=%d", count, len(users))
	}

	// 7. Delete
	if err := st.Users().DeleteUser(ctx, userID); err != nil {
		t.Fatalf("DeleteUser error: %v", err)
	}
	if _, err := st.Users().GetUserByID(ctx, userID); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after deletion, got %v", err)
	}
}

func TestSessionStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	userID := uuid.NewString()
	user := &store.User{
		ID:       userID,
		Username: "bob",
		Role:     store.RoleViewer,
		Status:   "active",
	}
	_ = st.Users().CreateUser(ctx, user)

	tokenHash := "mockhash12345"
	sess := &store.Session{
		TokenHash: tokenHash,
		UserID:    userID,
		UserAgent: "Mozilla/5.0 BusnesApp",
		IPAddress: "127.0.0.1",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(1 * time.Hour),
	}

	if err := st.Sessions().CreateSession(ctx, sess, user.Credentials()); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	got, err := st.Sessions().GetSession(ctx, tokenHash)
	if err != nil {
		t.Fatalf("GetSession error: %v", err)
	}
	if got.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, got.UserID)
	}

	if err := st.Sessions().DeleteSession(ctx, tokenHash); err != nil {
		t.Fatalf("DeleteSession error: %v", err)
	}
	if _, err := st.Sessions().GetSession(ctx, tokenHash); err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestAuditAndSettings(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// Audit log
	rec := &store.AuditRecord{
		UserID:    "user-1",
		Action:    "auth.login",
		Resource:  "session",
		Details:   `{"method":"totp"}`,
		IPAddress: "127.0.0.1",
	}
	if err := st.Audit().LogAudit(ctx, rec); err != nil {
		t.Fatalf("LogAudit error: %v", err)
	}

	records, count, err := st.Audit().ListAuditRecords(ctx, 0, 10)
	if err != nil || count != 1 || len(records) != 1 {
		t.Fatalf("ListAuditRecords failed: count=%d, err=%v", count, err)
	}

	// Settings
	if err := st.Settings().SetSetting(ctx, "theme_default", "patina"); err != nil {
		t.Fatalf("SetSetting error: %v", err)
	}
	val, err := st.Settings().GetSetting(ctx, "theme_default")
	if err != nil || val != "patina" {
		t.Fatalf("GetSetting failed: val=%s, err=%v", val, err)
	}
}

func TestSpendTOTPCounterRefusesReplay(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	u := &store.User{ID: "usr_t", Username: "t", Role: store.RoleViewer, Status: "active", SSOProvider: "local"}
	if err := st.Users().CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.Users().SpendTOTPCounter(ctx, u.ID, 100); err != nil {
		t.Fatalf("first spend: %v", err)
	}
	if err := st.Users().SpendTOTPCounter(ctx, u.ID, 100); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("replay: got %v, want ErrAlreadyExists", err)
	}
	if err := st.Users().SpendTOTPCounter(ctx, u.ID, 99); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("older counter: got %v, want ErrAlreadyExists", err)
	}
	if err := st.Users().SpendTOTPCounter(ctx, u.ID, 101); err != nil {
		t.Fatalf("next counter: %v", err)
	}
	got, _ := st.Users().GetUserByID(ctx, u.ID)
	if got.TOTPLastCounter != 101 {
		t.Fatalf("stored counter %d, want 101", got.TOTPLastCounter)
	}
}

func TestDeleteSettingIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	if err := st.Settings().DeleteSetting(ctx, "never"); err != nil {
		t.Fatal(err)
	}
	_ = st.Settings().SetSetting(ctx, "k", "v")
	_ = st.Settings().DeleteSetting(ctx, "k")
	if _, err := st.Settings().GetSetting(ctx, "k"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUserRoleIsAdminOrViewer(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	for _, role := range []string{"user", "manager", "", "Admin"} {
		err := st.Users().CreateUser(ctx, &store.User{ID: "usr_" + role, Username: "u" + role, Role: role, Status: "active"})
		if !errors.Is(err, store.ErrInvalidRole) {
			t.Errorf("CreateUser with role %q: got %v, want ErrInvalidRole", role, err)
		}
	}

	viewer := &store.User{ID: "usr_v", Username: "v", Role: store.RoleViewer, Status: "active"}
	if err := st.Users().CreateUser(ctx, viewer); err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	viewer.Role = "manager"
	if err := st.Users().UpdateUser(ctx, viewer); !errors.Is(err, store.ErrInvalidRole) {
		t.Errorf("UpdateUser to manager: got %v, want ErrInvalidRole", err)
	}
	viewer.Role = store.RoleAdmin
	if err := st.Users().UpdateUser(ctx, viewer); err != nil {
		t.Errorf("UpdateUser to admin: %v", err)
	}
}

// Replicas starting together on one Postgres database must not race each other's migrations.
func TestConcurrentOpenMigratesOnce(t *testing.T) {
	cfg := testdb.Config(t)
	if cfg.Driver != "postgres" {
		t.Skip("the migration lock is Postgres-only")
	}
	errs := make(chan error, 4)
	for range 4 {
		go func() {
			st, err := store.Open(context.Background(), cfg)
			if err == nil {
				_ = st.Close()
			}
			errs <- err
		}()
	}
	for range 4 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestEndSSOSessionFencesTheSID(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	user := &store.User{ID: uuid.NewString(), Username: "carol", Role: store.RoleViewer, Status: "active"}
	if err := st.Users().CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	create := func(hash, sid string) error {
		now := time.Now().UTC()
		return st.Sessions().CreateSession(ctx, &store.Session{TokenHash: hash, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), SSOSID: sid}, user.Credentials())
	}
	if err := create("ended", "sid-1"); err != nil {
		t.Fatal(err)
	}
	if err := create("kept", "sid-2"); err != nil {
		t.Fatal(err)
	}
	if err := st.Sessions().EndSSOSession(ctx, "sid-1", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Sessions().GetSession(ctx, "ended"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("ended session: %v", err)
	}
	if _, err := st.Sessions().GetSession(ctx, "kept"); err != nil {
		t.Fatalf("other sid's session: %v", err)
	}
	// A login for a fenced sid fails and leaves no session row behind.
	if err := create("late", "sid-1"); !errors.Is(err, store.ErrSSOSessionEnded) {
		t.Fatalf("fenced create: %v", err)
	}
	if _, err := st.Sessions().GetSession(ctx, "late"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("fenced create left its row: %v", err)
	}
	// An expired fence no longer refuses.
	if err := st.Sessions().EndSSOSession(ctx, "sid-3", time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := create("after", "sid-3"); err != nil {
		t.Fatalf("expired fence: %v", err)
	}
}

func TestEndSSOSubjectFencesTheSubject(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	user := &store.User{ID: uuid.NewString(), Username: "dave", Role: store.RoleViewer, Status: "active", SSOProvider: "kysignon", SSOSubject: "dave"}
	if err := st.Users().CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	create := func(hash string, issued time.Time) error {
		return st.Sessions().CreateSession(ctx, &store.Session{TokenHash: hash, UserID: user.ID, CreatedAt: issued, ExpiresAt: now.Add(time.Hour), SSOSID: hash}, user.Credentials())
	}
	if err := create("old", now.Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := create("new", now); err != nil {
		t.Fatal(err)
	}
	if err := st.Sessions().EndSSOSubject(ctx, "kysignon", "dave", now.Add(-time.Minute), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Sessions().GetSession(ctx, "old"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("session issued before the cutoff: %v", err)
	}
	if _, err := st.Sessions().GetSession(ctx, "new"); err != nil {
		t.Fatalf("session issued after the cutoff: %v", err)
	}
	// An earlier cutoff never moves the fence back.
	if err := st.Sessions().EndSSOSubject(ctx, "kysignon", "dave", now.Add(-5*time.Minute), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := create("late", now.Add(-90*time.Second)); !errors.Is(err, store.ErrSSOSessionEnded) {
		t.Fatalf("fenced create: %v", err)
	}
	if _, err := st.Sessions().GetSession(ctx, "late"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("fenced create left its row: %v", err)
	}
	if err := create("fresh", now); err != nil {
		t.Fatalf("login after the cutoff: %v", err)
	}
}
