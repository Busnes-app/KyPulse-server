package monitor_test

import (
	"context"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func observeCert(svc *monitor.Service, id string, c *poller.Cert, at time.Time) {
	tg, _ := svc.Store.Targets().GetTarget(context.Background(), id)
	svc.Observe(context.Background(), poller.Observation{Target: poller.Target{ID: id, Name: tg.Name, URL: tg.URL, SNI: tg.SNI},
		Result: poller.Result{State: poller.OK, Cert: c}, At: at})
	monitor.Flush(svc)
}

func certEvents(t *testing.T, st store.Store, id string) []*store.TargetEvent {
	t.Helper()
	evs, err := st.Targets().ListEvents(context.Background(), id, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	var out []*store.TargetEvent
	for _, e := range evs {
		if len(e.ToState) > 5 && e.ToState[:5] == "cert_" {
			out = append(out, e)
		}
	}
	return out
}

func TestCertificateWarnsThenGoesCriticalAndReminds(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "app", Name: "App", URL: "https://app.lan/healthz", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	day := 24 * time.Hour
	notAfter := now.Add(30 * day)
	c := &poller.Cert{NotAfter: notAfter, NameMatch: true, Issuer: "Test CA"}
	observeCert(svc, "app", c, *now)
	if len(poster.sent) != 0 || len(certEvents(t, st, "app")) != 0 {
		t.Fatalf("a fresh healthy certificate was news: %+v", poster.sent)
	}
	// 20 days left: warning, after two polls.
	at := notAfter.Add(-20 * day)
	observeCert(svc, "app", c, at)
	observeCert(svc, "app", c, at.Add(30*time.Second))
	if len(poster.sent) != 1 || poster.sent[0].State != "cert_warning" || poster.sent[0].Reason != "cert_expiring" {
		t.Fatalf("warning: %+v", poster.sent)
	}
	// Health stays ok: the certificate never moves the health state or the uptime.
	tg, _ := st.Targets().GetTarget(ctx, "app")
	if tg.State != "ok" {
		t.Fatalf("health state %s", tg.State)
	}
	if tr, _ := st.Targets().TransitionsSince(ctx, time.Time{}); len(tr) != 1 {
		t.Fatalf("certificate events leaked into transitions: %d", len(tr))
	}
	// An hour later, a reminder.
	observeCert(svc, "app", c, at.Add(time.Hour+time.Minute))
	if len(poster.sent) != 2 || !poster.sent[1].Reminder {
		t.Fatalf("reminder: %+v", poster.sent)
	}
	// Silenced: no more.
	if err := st.Targets().SetSilence(ctx, "app", nil, true); err != nil {
		t.Fatal(err)
	}
	at = notAfter.Add(-5 * day)
	observeCert(svc, "app", c, at)
	observeCert(svc, "app", c, at.Add(30*time.Second))
	if len(poster.sent) != 2 {
		t.Fatalf("sent while silenced: %+v", poster.sent[2:])
	}
	if evs := certEvents(t, st, "app"); evs[0].ToState != "cert_critical" {
		t.Fatalf("critical not recorded: %+v", evs[0])
	}
	// A renewed certificate ends the until-fixed silence.
	renewed := &poller.Cert{NotAfter: notAfter.Add(90 * day), NameMatch: true}
	observeCert(svc, "app", renewed, at.Add(time.Minute))
	observeCert(svc, "app", renewed, at.Add(2*time.Minute))
	if tg, _ := st.Targets().GetTarget(ctx, "app"); tg.UntilFixed {
		t.Fatal("renewal kept the until-fixed silence")
	}
}

func TestCertificateFailuresAndThresholds(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	if err := monitor.SetThresholds(ctx, st.Settings(), 60, 30); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][2]int{{7, 7}, {10, 20}, {400, 7}, {21, 0}} {
		if err := monitor.SetThresholds(ctx, st.Settings(), bad[0], bad[1]); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	if w, c := monitor.Thresholds(ctx, st.Settings()); w != 60 || c != 30 {
		t.Fatalf("thresholds %d %d", w, c)
	}
	for i, c := range []*poller.Cert{
		{NotAfter: now.Add(40 * 24 * time.Hour), NameMatch: true},                             // inside the custom warning
		{NotAfter: now.Add(-time.Minute), NameMatch: true},                                    // expired
		{NotAfter: now.Add(300 * 24 * time.Hour), NameMatch: false},                           // wrong name
		{NotAfter: now.Add(300 * 24 * time.Hour), NameMatch: true, Problem: "cert_untrusted"}, // bad chain
	} {
		id := string(rune('a' + i))
		if err := st.Targets().CreateTarget(ctx, &store.Target{ID: id, Name: id, URL: "tls://edge.lan:443", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		observeCert(svc, id, c, *now)
	}
	want := []string{"cert_warning/cert_expiring", "cert_failed/cert_expired", "cert_failed/cert_name_mismatch", "cert_failed/cert_untrusted"}
	if len(poster.sent) != len(want) {
		t.Fatalf("sent %+v", poster.sent)
	}
	for i, m := range poster.sent {
		if m.State+"/"+m.Reason != want[i] {
			t.Errorf("%d: got %s/%s want %s", i, m.State, m.Reason, want[i])
		}
	}
}

func TestCertificateTrackSurvivesAPollWithoutACertificate(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "e", Name: "Edge", URL: "tls://edge.lan:443", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	observeCert(svc, "e", &poller.Cert{NotAfter: now.Add(-time.Hour), NameMatch: true}, *now)
	observeCert(svc, "e", nil, now.Add(time.Hour+time.Minute))
	if len(poster.sent) != 2 || !poster.sent[1].Reminder || poster.sent[1].State != "cert_failed" {
		t.Fatalf("no reminder through a poll without a certificate: %+v", poster.sent)
	}
}

func TestEditingTheSNIDropsAPollInFlight(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	tg := &store.Target{ID: "e", Name: "Edge", URL: "tls://edge.lan:443", SNI: "old.lan", Enabled: true}
	if err := st.Targets().CreateTarget(ctx, tg); err != nil {
		t.Fatal(err)
	}
	tg.SNI = "new.lan"
	if err := st.Targets().UpdateTarget(ctx, tg); err != nil {
		t.Fatal(err)
	}
	svc.Observe(ctx, poller.Observation{Target: poller.Target{ID: "e", URL: tg.URL, SNI: "old.lan"},
		Result: poller.Result{State: poller.OK, Cert: &poller.Cert{NotAfter: now.Add(-time.Hour)}}, At: *now})
	monitor.Flush(svc)
	if got, _ := st.Targets().GetTarget(ctx, "e"); got.CertTrack != "" || len(poster.sent) != 0 {
		t.Fatalf("stale SNI poll recorded: %q %+v", got.CertTrack, poster.sent)
	}
}
