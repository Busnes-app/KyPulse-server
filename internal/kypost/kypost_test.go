package kypost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/suitedir"
	"github.com/Busnes-app/ky-primitives/syncauth"
	"github.com/Busnes-app/kypulse-server/internal/egress"
)

var secret = []byte(strings.Repeat("s", 32))

type fakePost struct {
	url     string
	headers map[string]string
	body    []byte
	code    int
	reply   string
	err     error
}

func (f *fakePost) Post(_ context.Context, url, _ string, body []byte, h map[string]string) (*egress.Response, error) {
	f.url, f.headers, f.body = url, h, body
	if f.err != nil {
		return nil, f.err
	}
	return &egress.Response{StatusCode: f.code, Body: []byte(f.reply)}, nil
}

func paired(f *fakePost) *Link {
	l := &Link{Post: f, Lookup: func(context.Context) (suitedir.Peer, error) { return suitedir.Peer{}, errors.New("unused") }}
	l.Set(&Peer{URL: "https://mail.example/", Secret: secret})
	return l
}

func TestSendSignsTheExactBody(t *testing.T) {
	f := &fakePost{code: 202, reply: `{"outcome":"sent"}`}
	req := SendRequest{Sub: "s1", Issuer: "https://id.example", Recipients: []string{"ops@example.com"}, Subject: "KyPost is down", Text: "x"}
	if err := paired(f).Send(context.Background(), "kypulse-alert-7", req); err != nil {
		t.Fatal(err)
	}
	if f.url != "https://mail.example/api/suite/kypulse/alert" {
		t.Fatalf("url %s", f.url)
	}
	r, _ := http.NewRequest("POST", f.url, nil)
	for k, v := range f.headers {
		r.Header.Set(k, v)
	}
	h := syncauth.FromRequest(r)
	ev, err := syncauth.Verify(secret, h, f.body, syncauth.Options{})
	if err != nil || ev.Type != EventSend || ev.ID != "kypulse-alert-7" {
		t.Fatalf("verify: %v %+v", err, ev)
	}
	var fields map[string]any
	_ = json.Unmarshal(f.body, &fields)
	if len(fields) != 5 {
		t.Fatalf("body has %d fields: %s", len(fields), f.body)
	}
}

func TestSendOutcomes(t *testing.T) {
	for _, tc := range []struct {
		f      fakePost
		retry  bool
		reason string
	}{
		{fakePost{code: 409, reply: `{"error":"x","code":"recipient_unconfirmed"}`}, false, "recipient_unconfirmed"},
		{fakePost{code: 409, reply: `{"code":"Not A Code <script>"}`}, false, "status_409"},
		{fakePost{code: 404, reply: `{"code":"kypulse_off"}`}, false, "kypulse_off"},
		{fakePost{code: 429}, true, "unavailable"},
		{fakePost{code: 503, reply: `{"code":"smtp_failed"}`}, true, "unavailable"},
		{fakePost{err: errors.New("dial tcp: connection refused")}, true, "unavailable"},
		{fakePost{code: 401, reply: `{"code":"bad_signature"}`}, true, "bad_signature"},
	} {
		err := paired(&tc.f).Send(context.Background(), "id", SendRequest{})
		if errors.Is(err, ErrRetry) != tc.retry || Reason(err) != tc.reason {
			t.Errorf("%+v: err %v retry %v reason %s", tc.f, err, errors.Is(err, ErrRetry), Reason(err))
		}
	}
}

func TestUnpairedSendAndKick(t *testing.T) {
	l := &Link{Post: &fakePost{}}
	if err := l.Send(context.Background(), "id", SendRequest{}); !errors.Is(err, ErrNotPaired) || Reason(err) != "not_paired" {
		t.Fatalf("got %v", err)
	}
	if l.Available() {
		t.Fatal("available without KyIdentity")
	}
	if _, reason := l.Status(); reason != ReasonNoKyIdentity {
		t.Fatalf("reason %s", reason)
	}
	f := &fakePost{code: 401, reply: `{"code":"bad_signature"}`}
	p := paired(f)
	_ = p.Send(context.Background(), "id", SendRequest{})
	select {
	case <-p.kick:
	default:
		t.Fatal("a refused signature did not ask for a fresh pairing")
	}
}

func TestResolve(t *testing.T) {
	var answer suitedir.Peer
	var fail error
	l := &Link{Post: &fakePost{}, Lookup: func(context.Context) (suitedir.Peer, error) { return answer, fail }}
	l.init()
	answer = suitedir.Peer{Client: "kypost", URL: "https://mail.example", Secret: secret}
	if !l.resolve(context.Background()) {
		t.Fatal("first lookup")
	}
	// KyIdentity down: the working pairing stays.
	fail = errors.New("connection refused")
	l.resolve(context.Background())
	if ok, _ := l.Status(); !ok {
		t.Fatal("an outage unpaired")
	}
	for _, tc := range []struct {
		err    error
		url    string
		reason string
	}{
		{suitedir.ErrNoPeer, "", ReasonNoPeer},
		{suitedir.ErrUnauthorized, "", ReasonUnauthorized},
		{nil, "https://mail.example/path", ReasonInvalidPeer},
		{nil, "https://user@mail.example", ReasonInvalidPeer},
	} {
		l.Set(&Peer{URL: "https://mail.example", Secret: secret})
		fail, answer = tc.err, suitedir.Peer{URL: tc.url, Secret: secret}
		l.resolve(context.Background())
		if ok, reason := l.Status(); ok || reason != tc.reason {
			t.Errorf("%v %s: paired=%v reason=%s", tc.err, tc.url, ok, reason)
		}
	}
}

func TestRunStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := &Link{Post: &fakePost{}, Lookup: func(context.Context) (suitedir.Peer, error) { return suitedir.Peer{}, suitedir.ErrNoPeer }}
	done := make(chan struct{})
	go func() { l.Run(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}
