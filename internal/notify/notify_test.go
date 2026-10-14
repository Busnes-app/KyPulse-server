package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/egress"
)

var msg = Message{App: "KyVault", State: "down", Previous: "ok", Reason: "refused",
	Time: time.Date(2026, 9, 26, 10, 41, 0, 0, time.UTC), URL: "https://pulse.lan/apps/kyvault"}

func TestValidate(t *testing.T) {
	for _, token := range []string{"", " \t"} {
		if err := Validate(Config{Preset: Gotify, URL: "https://gotify.lan", Token: token}, false); err == nil {
			t.Error("Gotify must require an app token")
		}
	}
	if err := Validate(Config{Preset: Gotify, URL: "https://gotify.lan", Token: "app-token"}, false); err != nil {
		t.Errorf("Gotify with token: %v", err)
	}
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
	for in, want := range map[string]string{
		"https://gotify.lan":             "https://gotify.lan/message",
		"https://x.lan/gotify/?tenant=a": "https://x.lan/gotify/message?tenant=a",
	} {
		if r, err := Build(Config{Preset: Gotify, URL: in, Token: "gk"}, msg); err != nil || r.URL != want {
			t.Errorf("gotify %s = %q %v, want %q", in, r.URL, err, want)
		}
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
	codes      []int // one per call; a negative code means a transport error
	calls      int
	retryAfter string
}

func (f *fakePoster) Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error) {
	f.calls++
	code := f.codes[f.calls-1]
	if code < 0 {
		return nil, errors.New("dial tcp: connection refused")
	}
	return &egress.Response{StatusCode: code, Header: http.Header{"Retry-After": {f.retryAfter}}}, nil
}

func TestJitterStaysWithinHalfToFull(t *testing.T) {
	d := 6 * time.Second
	seen := map[time.Duration]bool{}
	for range 200 {
		w := jitter(d)
		if w < d/2 || w > d {
			t.Fatalf("jitter(%s) = %s", d, w)
		}
		seen[w] = true
	}
	if len(seen) < 2 {
		t.Fatal("jitter must vary")
	}
	if jitter(0) != 0 {
		t.Fatal("no backoff stays no backoff")
	}
}

func TestSendHonoursRetryAfter(t *testing.T) {
	cfg := Config{Preset: Generic, URL: "https://hooks.lan/x"}
	fast := []time.Duration{time.Millisecond}
	p := &fakePoster{codes: []int{429, 200}, retryAfter: "1"}
	start := time.Now()
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); err != nil || p.calls != 2 || time.Since(start) < time.Second {
		t.Fatalf("Retry-After 1: err=%v calls=%d after %s", err, p.calls, time.Since(start))
	}
	p = &fakePoster{codes: []int{429, 200}, retryAfter: "3600"}
	if err := (&Notifier{Post: p, Backoff: fast}).Send(context.Background(), cfg, msg); Reason(err) != "receiver_429" || p.calls != 1 {
		t.Fatalf("Retry-After beyond the cap must end the retries: err=%v calls=%d", err, p.calls)
	}
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	for v, want := range map[string]time.Duration{"7": 7 * time.Second, "-3": 0, now.Add(5 * time.Second).Format(http.TimeFormat): 5 * time.Second, "99999999999999999": 24 * time.Hour} {
		if got, ok := retryAfter(v, now); !ok || got != want {
			t.Errorf("retryAfter(%q) = %s %v, want %s", v, got, ok, want)
		}
	}
	if _, ok := retryAfter("soon", now); ok {
		t.Error("an unparseable Retry-After must be ignored")
	}
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
