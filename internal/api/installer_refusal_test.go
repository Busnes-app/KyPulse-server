package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/api"
)

// The installer CLI shows only a generic refusal; the server log carries the reason.
func TestInstallerRefusalIsLogged(t *testing.T) {
	_, st, cfg := setupTestServer(t)
	var buf bytes.Buffer
	lg, err := logging.New(logging.Config{App: "kypulse", Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	srv := api.NewServer(cfg, st, lg, newTestMonitor(t, cfg, st, lg), newTestKyYard(t, cfg, st, lg, nil))
	b := applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530",
		Backup: &applysetup.Backup{Directory: "/var/backups/kypulse"}}
	raw, _ := json.Marshal(b)
	if _, err := srv.ApplyInstaller(context.Background(), raw, true); err == nil {
		t.Fatal("check of an absent configuration passed")
	}
	if out := buf.String(); !strings.Contains(out, `"event":"installer_setup_refused"`) || !strings.Contains(out, "installer configuration is absent") {
		t.Fatalf("refusal reason not logged:\n%s", out)
	}
}
