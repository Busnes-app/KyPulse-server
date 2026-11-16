package monitor_test

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/ky-primitives/suitedir"
	"github.com/Busnes-app/ky-primitives/syncauth"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/kypost"
	"github.com/Busnes-app/kypulse-server/internal/monitor"
	"github.com/Busnes-app/kypulse-server/internal/notify"
	"github.com/Busnes-app/kypulse-server/internal/poller"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

// kypostFake answers every alert with codes in turn and keeps the bodies.
type kypostFake struct {
	codes  []int
	reply  string
	bodies []kypost.SendRequest
	ids    []string
}

func (f *kypostFake) Post(_ context.Context, _, _ string, body []byte, h map[string]string) (*egress.Response, error) {
	var r kypost.SendRequest
	_ = json.Unmarshal(body, &r)
	f.bodies, f.ids = append(f.bodies, r), append(f.ids, h[syncauth.HeaderEventID])
	code := 202
	if n := len(f.bodies) - 1; n < len(f.codes) {
		code = f.codes[n]
	}
	return &egress.Response{StatusCode: code, Body: []byte(f.reply)}, nil
}

func mailService(t *testing.T, withWebhook bool, fake *kypostFake) (*monitor.Service, store.Store, *fakePoster) {
	t.Helper()
	cfg, st := testStore(t)
	cfg.SSO.KySignOnIssuer = "https://id.example"
	w, _ := monitor.NewWebhooks(cfg, st.Settings())
	if withWebhook {
		if err := w.Save(context.Background(), notify.Config{Preset: notify.Generic, URL: "https://hooks.lan/x"}); err != nil {
			t.Fatal(err)
		}
	}
	link := &kypost.Link{Post: fake, Lookup: func(context.Context) (suitedir.Peer, error) { return suitedir.Peer{}, suitedir.ErrNoPeer }}
	link.Set(&kypost.Peer{URL: "https://mail.example", Secret: []byte(strings.Repeat("s", 32))})
	mail, err := monitor.NewMail(cfg, st, link)
	if err != nil {
		t.Fatal(err)
	}
	mail.Backoff = []time.Duration{time.Millisecond}
	lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	poster := &fakePoster{}
	svc := &monitor.Service{Store: st, Webhooks: w, Mail: mail, Logger: lg, AppURL: "https://pulse.lan",
		Notifier: &notify.Notifier{Post: poster, Backoff: []time.Duration{time.Millisecond}}}
	svc.Start(context.Background())
	t.Cleanup(svc.Drain)
	ctx := context.Background()
	for _, u := range []*store.User{
		{ID: "u_sso", Username: "ops", Role: "admin", Status: "active", SSOProvider: "kysignon", SSOSubject: "sub-ops"},
		{ID: "u_local", Username: "local", Role: "admin", Status: "active", SSOProvider: "local"},
	} {
		if err := st.Users().CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	return svc, st, poster
}

func downOnce(t *testing.T, svc *monitor.Service, st store.Store) {
	t.Helper()
	at := time.Date(2026, 11, 17, 9, 0, 0, 0, time.UTC)
	oneDownFromDown(t, st, "app", at)
	tg, _ := st.Targets().GetTarget(context.Background(), "app")
	svc.Observe(context.Background(), poller.Observation{Target: poller.Target{ID: "app", URL: tg.URL}, Result: poller.Result{State: poller.Down, Cause: "refused"}, At: at})
	monitor.Flush(svc)
}

func TestAlertGoesToWebhookAndKyPost(t *testing.T) {
	fake := &kypostFake{}
	svc, st, poster := mailService(t, true, fake)
	if err := svc.Mail.Save(context.Background(), monitor.MailConfig{OwnerID: "u_sso", Recipients: []string{"ops@example.com"}}); err != nil {
		t.Fatal(err)
	}
	downOnce(t, svc, st)
	if len(poster.sent) != 1 || len(fake.bodies) != 1 {
		t.Fatalf("webhook %d, kypost %d", len(poster.sent), len(fake.bodies))
	}
	b := fake.bodies[0]
	if b.Sub != "sub-ops" || b.Issuer != "https://id.example" || b.Subject != "app is down" || !strings.Contains(b.Text, "refused") ||
		!strings.HasPrefix(fake.ids[0], "kypulse-alert-") {
		t.Fatalf("body %+v id %v", b, fake.ids)
	}
	if s, ok := svc.Mail.Status(context.Background()); !ok || !s.OK {
		t.Fatalf("status %+v", s)
	}
}

func TestKyPostRefusalIsRecordedWithoutAddresses(t *testing.T) {
	fake := &kypostFake{codes: []int{409}, reply: `{"error":"x","code":"recipient_unconfirmed"}`}
	svc, st, _ := mailService(t, false, fake)
	if err := svc.Mail.Save(context.Background(), monitor.MailConfig{OwnerID: "u_sso", Recipients: []string{"ops@example.com"}}); err != nil {
		t.Fatal(err)
	}
	downOnce(t, svc, st)
	if len(fake.bodies) != 1 {
		t.Fatalf("a refusal was retried: %d", len(fake.bodies))
	}
	s, _ := svc.Mail.Status(context.Background())
	if s.OK || s.Error != "recipient_unconfirmed" {
		t.Fatalf("status %+v", s)
	}
	// No webhook: the event carries the mail outcome.
	evs, _ := st.Targets().ListEvents(context.Background(), "app", 0, 5)
	if evs[0].Notified || evs[0].NotifyError != "recipient_unconfirmed" {
		t.Fatalf("event %+v", evs[0])
	}
	recs, _, _ := st.Audit().ListAuditRecords(context.Background(), 0, 20)
	for _, r := range recs {
		if strings.Contains(r.Details, "@") {
			t.Fatalf("an address reached the audit: %+v", r)
		}
	}
}

func TestKyPostRetriesAnOutageThenSends(t *testing.T) {
	fake := &kypostFake{codes: []int{503, 202}}
	svc, st, _ := mailService(t, false, fake)
	_ = svc.Mail.Save(context.Background(), monitor.MailConfig{OwnerID: "u_sso", Recipients: []string{"ops@example.com"}})
	downOnce(t, svc, st)
	if len(fake.bodies) != 2 || fake.ids[0] != fake.ids[1] {
		t.Fatalf("retry: %d %v", len(fake.bodies), fake.ids)
	}
}

func TestKyPostOwnerMustSignInThroughKyIdentity(t *testing.T) {
	fake := &kypostFake{}
	svc, st, _ := mailService(t, false, fake)
	_ = svc.Mail.Save(context.Background(), monitor.MailConfig{OwnerID: "u_local", Recipients: []string{"ops@example.com"}})
	downOnce(t, svc, st)
	if len(fake.bodies) != 0 {
		t.Fatal("sent for a local account")
	}
	if s, _ := svc.Mail.Status(context.Background()); s.Error != "owner_unavailable" {
		t.Fatalf("status %+v", s)
	}
}

func TestMailConfigValidate(t *testing.T) {
	ok := monitor.MailConfig{OwnerID: "u", Recipients: []string{" Ops@Example.com "}}
	if err := ok.Validate(); err != nil || ok.Recipients[0] != "ops@example.com" {
		t.Fatalf("%v %v", err, ok.Recipients)
	}
	many := make([]string, 12)
	for i := range many {
		many[i] = string(rune('a'+i)) + "@example.com"
	}
	for _, bad := range []monitor.MailConfig{
		{OwnerID: "", Recipients: []string{"a@example.com"}},
		{OwnerID: "u"},
		{OwnerID: "u", Recipients: many},
		{OwnerID: "u", Recipients: []string{"a@example.com", "A@example.com"}},
		{OwnerID: "u", Recipients: []string{"Ops <ops@example.com>"}},
		{OwnerID: "u", Recipients: []string{"ops@example.com\r\nBcc: x@evil.example"}},
		{OwnerID: "u", Recipients: []string{"not-an-address"}},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}
