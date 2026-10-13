package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

func TestInstallerMonitoringCheckOnReadOnlyConnection(t *testing.T) {
	s := newLogTestStore(t)
	s.db.SetMaxOpenConns(1)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte(strings.Repeat("ab", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	b := config.InstallerBundle{Common: applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530"}, Monitoring: &config.InstallerMonitoring{Targets: []config.InstallerTarget{{Key: "identity", Name: "KyIdentity", URL: "https://identity.example/healthz"}}, Sources: []config.InstallerSource{{Key: "offboard", Name: "offboard", Target: "identity", TokenFile: file, OffboardingAlerts: true}}}}
	if err := b.PrepareMonitoring(&config.Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyInstaller(ctx, b, false); err != nil {
		t.Fatal(err)
	}
	guard, reset := "PRAGMA query_only=ON", "PRAGMA query_only=OFF"
	if s.driver == "postgres" {
		guard, reset = "SET default_transaction_read_only=ON", "SET default_transaction_read_only=OFF"
	}
	if _, err := s.db.ExecContext(ctx, guard); err != nil {
		t.Fatal(err)
	}
	defer s.db.ExecContext(ctx, reset)
	if report, err := s.ApplyInstaller(ctx, b, true); err != nil || len(report.Present) != 2 || len(report.Created) != 0 {
		t.Fatalf("inspection attempted a write: %+v %v", report, err)
	}
}
