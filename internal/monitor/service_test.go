package monitor_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/recoveryclient"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

type fakePoster struct {
	mu    sync.Mutex
	sent  []notify.Message // decoded from the generic preset body
	codes []int
	err   error // returned by every call when set
	calls int
}

func (f *fakePoster) Post(_ context.Context, _ string, _ string, body []byte, _ map[string]string) (*egress.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var m notify.Message
	_ = json.Unmarshal(body, &m)
	f.sent = append(f.sent, m)
	code := 200
	if f.calls-1 < len(f.codes) {
		code = f.codes[f.calls-1]
	}
	if code < 0 {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &egress.Response{StatusCode: code}, nil
}

func newService(t *testing.T) (*monitor.Service, store.Store, *fakePoster, *time.Time) {
	t.Helper()
	cfg, st := testStore(t)
	w, _ := monitor.NewWebhooks(cfg, st.Settings())
	if err := w.Save(context.Background(), notify.Config{Preset: notify.Generic, URL: "https://hooks.lan/x"}); err != nil {
		t.Fatal(err)
	}
	lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	poster := &fakePoster{}
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	svc := &monitor.Service{Store: st, Webhooks: w, Logger: lg, AppURL: "https://pulse.lan",
		Notifier: &notify.Notifier{Post: poster, Backoff: []time.Duration{time.Millisecond}},
		Now:      func() time.Time { return now }}
	return svc, st, poster, &now
}

func observe(svc *monitor.Service, id string, state poller.State, at time.Time) {
	svc.Observe(context.Background(), poller.Observation{Target: poller.Target{ID: id, Name: "KyVault"},
		Result: poller.Result{State: state, Cause: "refused"}, At: at, Latency: 12 * time.Millisecond})
}

func TestDueHonoursIntervalAndEnabled(t *testing.T) {
	svc, st, _, now := newService(t)
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "a", URL: "https://a/", IntervalSec: 30, Enabled: true})
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "b", Name: "b", URL: "https://b/", IntervalSec: 30, Enabled: false})
	due, err := svc.Due(ctx, *now)
	if err != nil || len(due) != 1 || due[0].ID != "a" || due[0].Interval != 30*time.Second {
		t.Fatalf("never polled: %+v %v", due, err)
	}
	observe(svc, "a", poller.OK, *now)
	if due, _ = svc.Due(ctx, now.Add(29*time.Second)); len(due) != 0 {
		t.Fatalf("inside the interval: %+v", due)
	}
	if due, _ = svc.Due(ctx, now.Add(30*time.Second)); len(due) != 1 {
		t.Fatalf("at the interval: %+v", due)
	}
}

func TestObserveRecordsStateTransitionsAndSends(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})

	observe(svc, "a", poller.OK, *now)
	tg, _ := st.Targets().GetTarget(ctx, "a")
	if tg.State != "ok" || tg.LastPolledAt == nil || tg.LastLatencyMS != 12 {
		t.Fatalf("after first ok: %+v", tg)
	}
	if poster.calls != 0 {
		t.Fatal("a fresh target coming up must not notify")
	}
	for i := 1; i <= 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	tg, _ = st.Targets().GetTarget(ctx, "a")
	if tg.State != "down" || tg.Cause != "refused" {
		t.Fatalf("after three downs: %+v", tg)
	}
	events, total, _ := st.Targets().ListEvents(ctx, "a", 0, 10)
	if total != 2 || events[0].ToState != "down" || !events[0].Notified || events[0].NotifyError != "" {
		t.Fatalf("events: %d %+v", total, events)
	}
	if poster.calls != 1 || poster.sent[0].App != "KyVault" || poster.sent[0].State != "down" || poster.sent[0].Previous != "ok" || poster.sent[0].Reason != "refused" || poster.sent[0].URL != "https://pulse.lan/#/apps/a" {
		t.Fatalf("sent: %d %+v", poster.calls, poster.sent)
	}
	st1, ok, _ := svc.Webhooks.Status(ctx)
	if !ok || !st1.OK {
		t.Fatalf("delivery status: %+v", st1)
	}

	// An hour later, still down: one reminder, recorded as an event.
	observe(svc, "a", poller.Down, now.Add(2*time.Hour))
	events, total, _ = st.Targets().ListEvents(ctx, "a", 0, 10)
	if total != 3 || !events[0].Reminder || poster.calls != 2 || !poster.sent[1].Reminder {
		t.Fatalf("reminder: %d %+v calls=%d", total, events[0], poster.calls)
	}
}

func TestObserveRecordsAFailedSend(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	poster.codes = []int{503, 503}
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})
	for i := 0; i < 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	events, _, _ := st.Targets().ListEvents(ctx, "a", 0, 1)
	if events[0].Notified || events[0].NotifyError == "" {
		t.Fatalf("failed send not recorded: %+v", events[0])
	}
	s, ok, _ := svc.Webhooks.Status(ctx)
	if !ok || s.OK || s.Error == "" {
		t.Fatalf("delivery status: %+v", s)
	}
	tg, _ := st.Targets().GetTarget(ctx, "a")
	if tg.State != "down" {
		t.Fatal("a failed send must not stop the state from being recorded")
	}
}

func TestObserveRecordsAnUnreadableWebhook(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})
	other, err := recoveryclient.NewAESGCMSealer([]byte(strings.Repeat("k", 32)), "kypulse:setting:alert_webhook")
	if err != nil {
		t.Fatal(err)
	}
	svc.Webhooks.Sealer = other
	for i := 0; i < 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	events, _, _ := st.Targets().ListEvents(ctx, "a", 0, 1)
	if events[0].Notified || events[0].NotifyError != "unreadable" {
		t.Fatalf("unreadable webhook not recorded: %+v", events[0])
	}
	s, ok, _ := svc.Webhooks.Status(ctx)
	if !ok || s.OK || s.Error != "unreadable" {
		t.Fatalf("delivery status: %+v", s)
	}
	rows, _, _ := st.Audit().ListAuditRecords(ctx, 0, 1)
	if len(rows) != 1 || rows[0].Action != "alert.send_failed" || rows[0].UserID != "system" || !strings.Contains(rows[0].Details, "reason=unreadable") {
		t.Fatalf("audit: %+v", rows)
	}
	if poster.calls != 0 {
		t.Fatal("must not attempt delivery when the webhook cannot be read")
	}
}

func TestSendErrorsNeverCarryTheURL(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	poster.err = &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/1/SECRETTOKEN", Err: errors.New("dial: connection refused")}
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})
	for i := 0; i < 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	events, _, _ := st.Targets().ListEvents(ctx, "a", 0, 1)
	if events[0].NotifyError != "refused" {
		t.Fatalf("notify_error = %q, want refused", events[0].NotifyError)
	}
	status, _, _ := svc.Webhooks.Status(ctx)
	rows, _, _ := st.Audit().ListAuditRecords(ctx, 0, 1)
	if len(rows) != 1 {
		t.Fatalf("audit rows: %+v", rows)
	}
	for what, v := range map[string]string{"event": events[0].NotifyError, "status": status.Error, "audit": rows[0].Resource + " " + rows[0].Details} {
		if strings.Contains(v, "SECRETTOKEN") {
			t.Errorf("%s carries the webhook URL: %q", what, v)
		}
	}
}

func TestSilenceStopsSendsAndUntilFixedClears(t *testing.T) {
	svc, st, poster, now := newService(t)
	ctx := context.Background()
	_ = st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "KyVault", URL: "https://a/", IntervalSec: 30, Enabled: true})
	observe(svc, "a", poller.OK, *now)
	if err := svc.Silence(ctx, "a", 0, true); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		observe(svc, "a", poller.Down, now.Add(time.Duration(i)*30*time.Second))
	}
	tg, _ := st.Targets().GetTarget(ctx, "a")
	if tg.State != "down" || poster.calls != 0 || !tg.UntilFixed || !monitor.Track(tg).UntilFixed {
		t.Fatalf("silenced down: state=%s calls=%d untilFixed=%v track=%+v", tg.State, poster.calls, tg.UntilFixed, monitor.Track(tg))
	}
	observe(svc, "a", poller.OK, now.Add(4*30*time.Second))
	observe(svc, "a", poller.OK, now.Add(5*30*time.Second))
	tg, _ = st.Targets().GetTarget(ctx, "a")
	if tg.State != "ok" || poster.calls != 1 || poster.sent[0].State != "ok" || tg.UntilFixed || monitor.Track(tg).UntilFixed {
		t.Fatalf("recovery: state=%s calls=%d untilFixed=%v track=%+v", tg.State, poster.calls, tg.UntilFixed, monitor.Track(tg))
	}
	if err := svc.Silence(ctx, "a", time.Hour, false); err != nil {
		t.Fatal(err)
	}
	tg, _ = st.Targets().GetTarget(ctx, "a")
	if tg.SilencedUntil == nil || !tg.SilencedUntil.Equal(now.Add(time.Hour)) || !monitor.Track(tg).SilencedUntil.Equal(now.Add(time.Hour)) {
		t.Fatalf("timed silence: %+v", tg)
	}
	if err := svc.Silence(ctx, "missing", time.Hour, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing target: %v", err)
	}
}

func TestSendTestRecordsStatus(t *testing.T) {
	svc, _, poster, _ := newService(t)
	if err := svc.SendTest(context.Background()); err != nil || poster.calls != 1 || !poster.sent[0].Test {
		t.Fatalf("test send: %v calls=%d %+v", err, poster.calls, poster.sent)
	}
	s, ok, _ := svc.Webhooks.Status(context.Background())
	if !ok || !s.OK {
		t.Fatalf("status: %+v", s)
	}
}

func TestObserveIgnoresADeletedTarget(t *testing.T) {
	svc, _, poster, now := newService(t)
	observe(svc, "gone", poller.Down, *now) // must not panic or send
	if poster.calls != 0 {
		t.Fatal("sent for a target that does not exist")
	}
}
