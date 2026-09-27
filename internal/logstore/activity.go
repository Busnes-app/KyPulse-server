package logstore

import (
	"slices"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

// Sign-in actions are exact parsed audit vocabulary, never a text search.
func SignInBurstRule() store.ActivityBurstRule {
	return store.ActivityBurstRule{Actions: []string{"auth.login", "sign-in"}, Outcome: "failure", Minimum: 5, Window: 5 * time.Minute}
}
func FailedSignIn(a store.Activity) bool {
	rule := SignInBurstRule()
	return a.Outcome == rule.Outcome && slices.Contains(rule.Actions, a.Action)
}
func IsBurst(count int, from, to time.Time) bool {
	rule := SignInBurstRule()
	return count >= rule.Minimum && !to.Before(from) && to.Sub(from) <= rule.Window
}
