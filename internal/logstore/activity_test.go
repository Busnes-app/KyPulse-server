package logstore

import (
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestFailedSignInBurstPolicy(t *testing.T) {
	for _, tc := range []struct {
		action, outcome string
		want            bool
	}{{"auth.login", "failure", true}, {"sign-in", "failure", true}, {"auth.login", "success", false}, {"delete.login.failure", "failure", false}, {"auth.login", "", false}} {
		if got := FailedSignIn(store.Activity{Action: tc.action, Outcome: tc.outcome}); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
	now := time.Now()
	for _, tc := range []struct {
		count    int
		duration time.Duration
		want     bool
	}{{4, 5 * time.Minute, false}, {5, 5 * time.Minute, true}, {5, 5*time.Minute + time.Microsecond, false}} {
		if got := IsBurst(tc.count, now, now.Add(tc.duration)); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}
