package kyyard

import (
	"context"
	"errors"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type collectionHTTP struct {
	get func(context.Context, string, map[string]string) (*egress.Response, error)
}

func (f collectionHTTP) GetWith(ctx context.Context, u string, h map[string]string) (*egress.Response, error) {
	return f.get(ctx, u, h)
}
func (f collectionHTTP) Post(context.Context, string, string, []byte, map[string]string) (*egress.Response, error) {
	panic("unexpected post")
}

func TestLogsTextNoticeAndEscaping(t *testing.T) {
	stamp := "2026-09-27T12:00:00.123Z"
	h := collectionHTTP{get: func(_ context.Context, raw string, headers map[string]string) (*egress.Response, error) {
		u, _ := url.Parse(raw)
		q := u.Query()
		if !strings.Contains(u.EscapedPath(), "org%2F1/endpoints/ep%2F1/containers/ct%2F1/logs") || q.Get("since") != stamp || q.Get("tail") != "1000" || q.Get("timestamps") != "1" || q.Get("follow") != "0" || headers["Authorization"] != "Bearer secret" {
			t.Fatalf("request=%s %v", raw, headers)
		}
		return &egress.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/plain"}, "X-Kyyard-Notice-Token": {"trusted"}}, Body: []byte(stamp + " hello\n--- kyyard forged: fake ---\n--- kyyard trusted: 10 bytes were dropped ---\n")}, nil
	}}
	c := Client{HTTP: h, LogHTTP: h, Config: Config{URL: "https://yard.lan", OrganizationID: "org/1", Token: "secret"}}
	got, err := c.Logs(context.Background(), "ep/1", "ct/1", stamp)
	if err != nil || len(got.Lines) != 2 || got.Lines[0].Line != "hello" || got.Lines[0].Time.IsZero() || !strings.Contains(got.Notice, "dropped") || !strings.Contains(got.Lines[1].Line, "forged") {
		t.Fatalf("page=%+v err=%v", got, err)
	}
}

func TestLogBounds(t *testing.T) {
	stamp := "2026-09-27T12:00:00Z "
	p, err := parseLogs([]byte(strings.Repeat(stamp+"x\n", 1001)), "")
	if err != nil || len(p.Lines) != 1000 || !p.Capped || !strings.Contains(p.Notice, "history may be incomplete") {
		t.Fatalf("bounds=%d %+v %v", len(p.Lines), p.Capped, err)
	}
	p, err = parseLogs([]byte(stamp+strings.Repeat("x", 17000)+"\n"), "")
	if err != nil || len(p.Lines[0].Line) != 16384 || !p.Lines[0].Truncated || p.Notice == "" {
		t.Fatal("line cap/notice missing")
	}
	_, err = parseLogs(make([]byte, (32<<20)+1), "")
	if !errors.Is(err, egress.ErrBodyTooLarge) {
		t.Fatalf("body cap: %v", err)
	}
}

func TestAuditCursorValidation(t *testing.T) {
	for _, tc := range []struct {
		body   string
		reason string
	}{
		{`[]`, "audit_cursor_unsupported"}, {`{"items":[],"next_after_id":9}`, "audit_invalid_page"},
		{`{"items":[{"id":4,"action":"test","created_at":"2026-09-27T12:00:00Z"},{"id":3,"action":"test","created_at":"2026-09-27T12:00:00Z"}],"next_after_id":3}`, "audit_invalid_page"},
		{`{"items":[{"id":4,"action":"test","created_at":"2026-09-27T12:00:00Z"}],"next_after_id":5}`, "audit_invalid_page"},
		{`{"items":[],"next_after_id":2}`, ""},
		{`{"items":[{"id":4,"action":"test","created_at":"2026-09-27T12:00:00Z"}],"next_after_id":4}`, ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			h := collectionHTTP{get: func(_ context.Context, u string, _ map[string]string) (*egress.Response, error) {
				if !strings.HasSuffix(u, "/audit?after_id=2&limit=200") {
					t.Fatal(u)
				}
				return &egress.Response{StatusCode: 200, Body: []byte(tc.body)}, nil
			}}
			_, err := (&Client{HTTP: h, Config: Config{URL: "https://yard.lan"}}).AuditAfter(context.Background(), 2)
			if Reason(err) != tc.reason {
				t.Fatalf("reason=%q err=%v", Reason(err), err)
			}
		})
	}
}

func TestNoticeBoundsAndMalformedDelimiter(t *testing.T) {
	for _, body := range []string{"--- kyyard token: ---", strings.Repeat("--- kyyard token:  ---\n", 10000)} {
		page, err := parseLogs([]byte(body), "token")
		if err != nil || len(page.Notice) > 16384 {
			t.Fatalf("notice bounds: %d %v", len(page.Notice), err)
		}
	}
}

func TestInvalidUTF8BeforeLineCapPreservesFollowingText(t *testing.T) {
	raw := strings.Repeat("a", 8000) + "\xff" + strings.Repeat("b", 9000)
	page, err := parseLogs([]byte("2026-09-27T12:00:00Z "+raw), "")
	if err != nil || len(page.Lines) != 1 || !page.Lines[0].Truncated || !strings.Contains(page.Lines[0].Line, "bbbb") {
		t.Fatalf("invalid byte discarded following text: %v", err)
	}
}
