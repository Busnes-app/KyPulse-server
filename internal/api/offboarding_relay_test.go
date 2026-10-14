package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

type offboardingPoster struct{ messages chan notify.Message }

func (p *offboardingPoster) Post(_ context.Context, _ string, _ string, body []byte, _ map[string]string) (*egress.Response, error) {
	var m notify.Message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	p.messages <- m
	return &egress.Response{StatusCode: 200}, nil
}

func TestOffboardFailureRelayNative(t *testing.T) {
	installer := os.Getenv("KYQ_TEST_INSTALLER_RELAY_ROOT")
	if installer == "" {
		t.Skip("explicit isolated installer failure fixture")
	}
	srv, st, cfg := setupTestServer(t)
	ctx := context.Background()
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "bridge", Name: "Offboarding bridge", URL: "https://bridge.example/healthz", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.Sources().CreateOffboardingCode(ctx, "code", "bridge", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	const token = "native-offboarding-alert-fixture"
	hash := sha256.Sum256([]byte(token))
	source, err := st.Sources().Claim(ctx, "code", hex.EncodeToString(hash[:]), "bridge")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var lastBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(body)))
			if r.Header.Get("Ky-Log-Sequence") == "2" {
				mu.Lock()
				lastBody = body
				mu.Unlock()
			}
		}
		srv.ServeHTTP(w, r)
	}))
	defer server.Close()
	material, _ := json.Marshal(struct {
		Token       string
		Certificate []byte
	}{token, server.Certificate().Raw})
	commandCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, "go", "test", "-race", "-count=1", "./internal/offboard", "-run", "^TestNativePulseFailureRelay$")
	command.Dir = installer
	command.Env = append(os.Environ(), "GOWORK=off", "KYQ_TEST_PULSE_FAILURE_RELAY_URL="+server.URL, "KYQ_TEST_PULSE_RELAY_MATERIAL="+string(material))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native failure relay: %v\n%s", err, output)
	}
	events, err := st.Targets().ListEvents(ctx, "bridge", 0, 10)
	total := len(events)
	if err != nil || total != 1 || events[0].Cause != "offboarding_failed" || events[0].NotifyError != "pending" {
		t.Fatalf("native durable failure: %+v total=%d %v", events, total, err)
	}
	// Start a fresh monitoring service after ingestion: delivery depends on durable state.
	webhooks, err := monitor.NewWebhooks(cfg, st.Settings())
	if err != nil {
		t.Fatal(err)
	}
	if err := webhooks.Save(ctx, notify.Config{Preset: notify.Generic, URL: "https://hooks.example/x"}); err != nil {
		t.Fatal(err)
	}
	lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	poster := &offboardingPoster{messages: make(chan notify.Message, 8)}
	svc := &monitor.Service{Store: st, Webhooks: webhooks, Logger: lg, AppURL: "https://pulse.example", Notifier: &notify.Notifier{Post: poster}}
	runCtx, stop := context.WithCancel(ctx)
	svc.Start(runCtx)
	defer func() { stop(); svc.Drain() }()
	select {
	case m := <-poster.messages:
		if m.Reason != "offboarding_failed" || m.App != "Offboarding bridge" || m.URL != "https://pulse.example/#/apps/bridge" {
			t.Fatalf("native failure webhook: %+v", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("native persisted failure did not notify")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		events, err = st.Targets().ListEvents(ctx, "bridge", 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 1 && events[0].Notified {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native webhook outcome was not recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	body := append([]byte(nil), lastBody...)
	mu.Unlock()
	r := httptest.NewRequest("POST", "/api/ingest/logs", strings.NewReader(string(body)))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/x-ndjson")
	r.Header.Set("Ky-Log-Sequence", "2")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("native replay refused: %d %s", w.Code, w.Body)
	}
	time.Sleep(1100 * time.Millisecond)
	stop()
	svc.Drain()
	select {
	case <-poster.messages:
		t.Fatal("receipt replay duplicated webhook")
	default:
	}
	rows, err := st.Logs().List(ctx, store.LogFilter{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("native rows: %d %v", len(rows), err)
	}
	activity, err := st.Logs().ListActivity(ctx, store.ActivityFilter{})
	if err != nil || len(activity) != 2 {
		t.Fatalf("native activity: %d %v", len(activity), err)
	}
	for _, a := range activity {
		if a.SourceID != source.ID || a.TargetID != "bridge" || a.Actor != "kyoffboard" || strings.Contains(a.Target, "private") {
			t.Fatalf("native failure scope: %+v", a)
		}
	}
	if events, err := st.Targets().ListEvents(ctx, "bridge", 0, 10); err != nil || len(events) != 1 || !events[0].Notified {
		t.Fatalf("native replay changed alert: %+v %v", events, err)
	}
}
