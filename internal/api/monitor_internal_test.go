package api

import (
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/notify"
)

func TestSavedWebhookTokenRequiresSameReceiver(t *testing.T) {
	old := notify.Config{Preset: notify.Generic, URL: "https://hook.lan:8443/old", Token: "saved-secret"}
	for _, tc := range []struct {
		preset string
		url    string
		want   bool
	}{
		{"generic", "https://hook.lan:8443/new", true},
		{"generic", "http://hook.lan:8443/old", false},
		{"generic", "https://other.lan:8443/old", false},
		{"generic", "https://hook.lan:443/old", false},
		{"ntfy", "https://hook.lan:8443/old", false},
	} {
		if got := sameReceiver(old, tc.preset, tc.url); got != tc.want {
			t.Errorf("%s %s: token reuse=%v, want %v", tc.preset, tc.url, got, tc.want)
		}
	}
}
