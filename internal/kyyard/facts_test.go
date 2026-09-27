package kyyard_test

import (
	"testing"

	"github.com/Busnes-app/kypulse-server/internal/kyyard"
)

func TestParseStatus(t *testing.T) {
	cases := []struct {
		status string
		health string
		exit   int // -1 = nil
	}{
		{"Up 3 hours (healthy)", "healthy", -1},
		{"Up 2 minutes (unhealthy)", "unhealthy", -1},
		{"Up 10 seconds (health: starting)", "starting", -1},
		{"Up 5 days", "none", -1},
		{"Exited (137) 2 hours ago", "none", 137},
		{"Exited (0) 3 days ago", "none", 0},
		{"Restarting (1) 5 seconds ago", "none", 1},
		{"", "none", -1},
	}
	for _, c := range cases {
		h, code := kyyard.ParseStatus(c.status)
		if h != c.health || (c.exit == -1 && code != nil) || (c.exit != -1 && (code == nil || *code != c.exit)) {
			t.Errorf("%q: got %s %v", c.status, h, code)
		}
	}
	if kyyard.LinkFor("ep_1", "kyvault") != "ep_1/kyvault" {
		t.Fatal("link format")
	}
}
