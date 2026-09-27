package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

var msg = Message{App: "KyVault", State: "down", Previous: "ok", Reason: "refused",
	Time: time.Date(2026, 9, 26, 10, 41, 0, 0, time.UTC), URL: "https://pulse.lan/apps/kyvault"}

func TestValidate(t *testing.T) {
	if err := Validate(Config{Preset: "slack", URL: "https://x.lan/"}, false); !errors.Is(err, ErrBadPreset) {
		t.Errorf("bad preset: %v", err)
	}
	if err := Validate(Config{Preset: Ntfy, URL: "http://ntfy.lan/t"}, false); !errors.Is(err, egress.ErrScheme) {
		t.Errorf("http without opt-in: %v", err)
	}
	if err := Validate(Config{Preset: Ntfy, URL: "http://ntfy.lan/t"}, true); err != nil {
		t.Errorf("http with opt-in: %v", err)
	}
	if err := Validate(Config{Preset: Discord, URL: "https://127.0.0.1/hook"}, false); !errors.Is(err, egress.ErrRefusedAddress) {
		t.Errorf("loopback: %v", err)
	}
}

func TestBuildPresets(t *testing.T) {
	ntfy, _ := Build(Config{Preset: Ntfy, URL: "https://ntfy.lan/pulse", Token: "tk"}, msg)
	if ntfy.ContentType != "text/plain" || ntfy.Headers["Title"] != "kyPulse: KyVault is down" || ntfy.Headers["Priority"] != "5" || ntfy.Headers["Authorization"] != "Bearer tk" {
		t.Errorf("ntfy = %+v", ntfy)
	}
	if !strings.Contains(string(ntfy.Body), "(refused) at 2026-09-26T10:41:00Z\nhttps://pulse.lan/apps/kyvault") {
		t.Errorf("ntfy body = %q", ntfy.Body)
	}

	gotify, _ := Build(Config{Preset: Gotify, URL: "https://gotify.lan/", Token: "gk"}, msg)
	var g map[string]any
	_ = json.Unmarshal(gotify.Body, &g)
	if gotify.URL != "https://gotify.lan/message" || gotify.Headers["X-Gotify-Key"] != "gk" || g["priority"] != float64(8) || g["title"] != "kyPulse: KyVault is down" {
		t.Errorf("gotify = %+v body=%v", gotify, g)
	}
	if strings.Contains(gotify.URL, "gk") {
		t.Error("gotify token must not be in the URL")
	}

	discord, _ := Build(Config{Preset: Discord, URL: "https://discord.com/api/webhooks/1/x"}, msg)
	var d map[string]string
	_ = json.Unmarshal(discord.Body, &d)
	if !strings.HasPrefix(d["content"], "kyPulse: KyVault is down") {
		t.Errorf("discord = %v", d)
	}

	generic, _ := Build(Config{Preset: Generic, URL: "https://hooks.lan/x", Token: "gt"}, msg)
	var m Message
	if err := json.Unmarshal(generic.Body, &m); err != nil || m.App != "KyVault" || m.Previous != "ok" || generic.Headers["Authorization"] != "Bearer gt" {
		t.Errorf("generic = %+v err=%v", m, err)
	}
	if strings.Contains(string(generic.Body), "gt") {
		t.Error("generic body must not carry the token")
	}
}

func TestTitles(t *testing.T) {
	cases := map[string]Message{
		"kyPulse: KyVault is down":     msg,
		"kyPulse: KyVault still down":  {App: "KyVault", State: "down", Reminder: true},
		"kyPulse: KyVault recovered":   {App: "KyVault", State: "ok"},
		"kyPulse: KyVault is degraded": {App: "KyVault", State: "degraded"},
		"kyPulse: test alert":          {Test: true},
	}
	for want, m := range cases {
		if got := m.Title(); got != want {
			t.Errorf("Title() = %q, want %q", got, want)
		}
	}
}

type fakePoster struct {
	codes []int // one per call; a negative code means a transport error
	calls int
}

func (f *fakePoster) Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error) {
	f.calls++
	code := f.codes[f.calls-1]
	if code < 0 {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &egress.Response{StatusCode: code}, nil
}

func TestSendRetriesTransportAnd5xxButNot4xx(t *testing.T) {
	cfg := Config{Preset: Generic, URL: "https://hooks.lan/x"}
	fast := []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

	p := &fakePoster{codes: []int{-1, 503, 429, 200}}
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); err != nil || p.calls != 4 {
		t.Fatalf("recovering send: err=%v calls=%d", err, p.calls)
	}

	p = &fakePoster{codes: []int{500, 500, 500, 500}}
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); Reason(err) != "receiver_500" || p.calls != 4 {
		t.Fatalf("exhausted: err=%v calls=%d", err, p.calls)
	}

	p = &fakePoster{codes: []int{403}}
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); !errors.Is(err, ErrRejected) || Reason(err) != "rejected_403" || p.calls != 1 {
		t.Fatalf("rejected: err=%v calls=%d", err, p.calls)
	}

	p = &fakePoster{codes: []int{500, 200}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&Notifier{Post: p, Backoff: []time.Duration{time.Hour}}).Send(ctx, cfg, msg); !errors.Is(err, context.Canceled) || p.calls != 1 {
		t.Fatalf("cancelled: err=%v calls=%d", err, p.calls)
	}
}

func TestReasonIsAFixedVocabulary(t *testing.T) {
	leak := &url.Error{Op: "Post", URL: "https://discord.com/api/webhooks/1/SECRET", Err: errors.New("dial tcp: connection refused")}
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{leak, "refused"},
		{&url.Error{Op: "Post", URL: "https://x.lan/SECRET", Err: errors.New("EOF")}, "network"},
		{&url.Error{Op: "Post", URL: "https://x.lan/SECRET", Err: context.Canceled}, "cancelled"},
		{context.DeadlineExceeded, "timeout"},
		{RejectedError{Code: 404}, "rejected_404"},
		{fmt.Errorf("wrapped: %w", ReceiverError{Code: 429}), "receiver_429"},
		{fmt.Errorf("%w: %w", ErrUnreadable, errors.New("cipher: message authentication failed")), "unreadable"},
	}
	for _, tc := range cases {
		if got := Reason(tc.err); got != tc.want {
			t.Errorf("Reason(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
