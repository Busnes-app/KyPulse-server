package monitor_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func pendingOffboarding(t *testing.T, st store.Store) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "bridge", Name: "Offboarding bridge", URL: "https://bridge.example/healthz", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.Sources().CreateOffboardingCode(ctx, "code", "bridge", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	source, err := st.Sources().Claim(ctx, "code", "token", "bridge")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	batch := store.LogBatch{Logs: []store.LogLine{{App: "kyoffboard", Time: now, ReceivedAt: now, Raw: "audit"}}, Activity: []store.Activity{{App: "kyoffboard", Actor: "kyoffboard", Action: "offboarding.failed", Outcome: "failed", Target: "bookstack:" + strings.Repeat("ab", 32), Time: now, ReceivedAt: now}}}
	if err := st.Logs().AppendSequenced(ctx, source.ID, batch, store.LogReceipt{Sequence: 1, Digest: strings.Repeat("ab", 32)}, 1<<20); err != nil {
		t.Fatal(err)
	}
	e, err := st.Targets().NextOffboardingAlert(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return e.ID
}

func waitOffboarding(t *testing.T, st store.Store, reason string) *store.TargetEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		events, _, err := st.Targets().ListEvents(context.Background(), "bridge", 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 1 && events[0].NotifyError == reason {
			return events[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("offboarding alert did not reach %q", reason)
	return nil
}

func TestOffboardingDurableDeliveryAndCancellationResume(t *testing.T) {
	cfg, st := testStore(t)
	webhooks, err := monitor.NewWebhooks(cfg, st.Settings())
	if err != nil {
		t.Fatal(err)
	}
	if err := webhooks.Save(context.Background(), notify.Config{Preset: notify.Generic, URL: "https://hooks.example/x"}); err != nil {
		t.Fatal(err)
	}
	id := pendingOffboarding(t, st)
	lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	blocked := &ctxPoster{entered: make(chan struct{}, 1)}
	first := &monitor.Service{Store: st, Webhooks: webhooks, Logger: lg, AppURL: "https://pulse.example", Notifier: &notify.Notifier{Post: blocked}, ShutdownGrace: 20 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	first.Start(ctx)
	t.Cleanup(func() { cancel(); first.Drain() })
	select {
	case <-blocked.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("persisted alert was not delivered")
	}
	cancel()
	first.Drain()
	if e := waitOffboarding(t, st, "pending"); e.ID != id || e.Notified {
		t.Fatal("shutdown lost pending alert")
	}
	poster := &fakePoster{}
	second := &monitor.Service{Store: st, Webhooks: webhooks, Logger: lg, AppURL: "https://pulse.example", Notifier: &notify.Notifier{Post: poster}}
	ctx, cancelSecond := context.WithCancel(context.Background())
	second.Start(ctx)
	t.Cleanup(func() { cancelSecond(); second.Drain() })
	e := waitOffboarding(t, st, "")
	if !e.Notified || e.ID != id {
		t.Fatal("restart did not deliver the original alert")
	}
	// Another scan must not resend a recorded success.
	time.Sleep(1100 * time.Millisecond)
	cancelSecond()
	second.Drain()
	poster.mu.Lock()
	defer poster.mu.Unlock()
	if poster.calls != 1 || len(poster.sent) != 1 || poster.sent[0].Reason != "offboarding_failed" || poster.sent[0].App != "Offboarding bridge" || poster.sent[0].URL != "https://pulse.example/#/apps/bridge" {
		t.Fatalf("webhook: calls=%d %+v", poster.calls, poster.sent)
	}
	rows, _, err := st.Audit().ListAuditRecords(context.Background(), 0, 10)
	if err != nil || len(rows) != 2 || rows[0].Action != "alert.sent" || rows[1].Action != "alert.send_failed" {
		t.Fatalf("delivery audit: %+v %v", rows, err)
	}
}

func TestOffboardingDeliveryHonorsSettingsAndSafeFailure(t *testing.T) {
	for _, mode := range []string{"silenced", "paused", "unconfigured", "unreadable", "rejected_403"} {
		t.Run(mode, func(t *testing.T) {
			cfg, st := testStore(t)
			webhooks, err := monitor.NewWebhooks(cfg, st.Settings())
			if err != nil {
				t.Fatal(err)
			}
			if mode != "unconfigured" {
				if err := webhooks.Save(context.Background(), notify.Config{Preset: notify.Generic, URL: "https://hooks.example/PRIVATE"}); err != nil {
					t.Fatal(err)
				}
			}
			pendingOffboarding(t, st)
			if mode == "silenced" {
				until := time.Now().Add(time.Hour)
				if err := st.Targets().SetSilence(context.Background(), "bridge", &until, false); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "paused" {
				target, err := st.Targets().GetTarget(context.Background(), "bridge")
				if err != nil {
					t.Fatal(err)
				}
				target.Enabled = false
				if err := st.Targets().UpdateTarget(context.Background(), target); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unreadable" {
				webhooks.Sealer, err = recoveryclient.NewAESGCMSealer([]byte(strings.Repeat("z", 32)), "kypulse:setting:alert_webhook")
				if err != nil {
					t.Fatal(err)
				}
			}
			poster := &fakePoster{codes: []int{403}}
			lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
			svc := &monitor.Service{Store: st, Webhooks: webhooks, Logger: lg, Notifier: &notify.Notifier{Post: poster}}
			ctx, cancel := context.WithCancel(context.Background())
			svc.Start(ctx)
			t.Cleanup(func() { cancel(); svc.Drain() })
			e := waitOffboarding(t, st, mode)
			if e.Notified {
				t.Fatal("suppressed/failed webhook marked delivered")
			}
			cancel()
			svc.Drain()
			poster.mu.Lock()
			defer poster.mu.Unlock()
			want := 0
			if mode == "rejected_403" {
				want = 1
			}
			if poster.calls != want {
				t.Fatalf("webhook calls=%d want=%d", poster.calls, want)
			}
			rows, _, err := st.Audit().ListAuditRecords(context.Background(), 0, 10)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if strings.Contains(row.Details, "PRIVATE") {
					t.Fatal("webhook credential leaked")
				}
			}
		})
	}
}
