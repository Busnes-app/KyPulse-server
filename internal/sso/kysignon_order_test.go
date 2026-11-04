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
