package api_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
)

func TestInstallerMonitoringPrivateIPCAndNativeAlertSource(t *testing.T) {
	ctx := context.Background()
	srv, st, cfg := setupTestServer(t)
	file := filepath.Join(t.TempDir(), "token")
	token := strings.Repeat("ab", 32)
	if err := os.WriteFile(file, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	b := config.InstallerBundle{Common: applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530"}, Monitoring: &config.InstallerMonitoring{Targets: []config.InstallerTarget{{Key: "identity", Name: "KyIdentity", URL: "https://identity.example/healthz"}}, Sources: []config.InstallerSource{{Key: "offboard", Name: "offboard", Target: "identity", TokenFile: file, OffboardingAlerts: true}}}}
	raw, _ := json.Marshal(b)
	socket, err := config.InstallerSocket(cfg.Database.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	closeSetup, err := applysetup.StartLocal(ctx, socket, srv.ApplyInstaller, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeSetup()
	for i, check := range []bool{false, true, false} {
		report, err := applysetup.CallLocal(ctx, socket, raw, check)
		if err != nil || len(report.Pending) != 0 {
			t.Fatalf("IPC enrollment: %+v %v", report, err)
		}
		if i == 0 && len(report.Created) != 2 || i > 0 && (len(report.Created) != 0 || len(report.Present) != 2) {
			t.Fatal("enrollment did not converge", report)
		}
		encoded, _ := json.Marshal(report)
		if strings.Contains(string(encoded), token) {
			t.Fatal("credential in report")
		}
	}
	sources, err := st.Sources().List(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatalf("native sources: %+v %v", sources, err)
	}
	request := httptest.NewRequest("OPTIONS", "/api/ingest/logs", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, request)
	if w.Code != 204 || w.Header().Get("Ky-Log-Source") != sources[0].ID || w.Header().Get("Ky-Log-Alerts") != "ky.offboarding-alerts/1" {
		t.Fatalf("native enrolled authority: %d %v", w.Code, w.Header())
	}
	line, _ := json.Marshal(map[string]any{"app": "kyoffboard", "seq": 2, "hash": "audit", "fields": []string{}, "user_id": "kyoffboard", "resource": "bookstack:" + strings.Repeat("ab", 32), "action": "offboarding.failed", "outcome": "failed"})
	body, _ := json.Marshal(map[string]string{"line": string(line)})
	request = httptest.NewRequest("POST", "/api/ingest/logs", strings.NewReader(string(body)+"\n"))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/x-ndjson")
	request.Header.Set("Ky-Log-Sequence", "1")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, request)
	if w.Code != 204 {
		t.Fatalf("enrolled ingest: %d %s", w.Code, w.Body)
	}
	events, err := st.Targets().ListEvents(ctx, "", 0, 10)
	total := len(events)
	if err != nil || total != 1 || events[0].TargetID != sources[0].TargetID || events[0].Cause != "offboarding_failed" {
		t.Fatalf("enrolled failure alerts: %+v %v", events, err)
	}
	if report, err := applysetup.CallLocal(ctx, socket, raw, true); err != nil || len(report.Present) != 2 {
		t.Fatal("ingestion drifted enrollment", report, err)
	}
}
