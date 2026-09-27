package logstore

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/auditchain"
	"github.com/Busnes-app/ky-primitives/logging"
)

func TestPlainAndControlText(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	line, activity := Parse("\x1b[31m<img onerror=x>\x1b[0m\x00", "s", "host", "t", now, now, false)
	if activity != nil || line.Source != "host" || line.TargetID != "t" {
		t.Fatalf("wrong attribution: %+v %+v", line, activity)
	}
	if got := Display(line.Message); got != "<img onerror=x>" {
		t.Fatalf("display = %q", got)
	}
	for _, v := range []string{"a\x1b]0;title\aZ", "a\x1b]0;title\x1b\\Z", "a\x1b[31", "a\u009b31mZ", "a\x1b]unfinished"} {
		if strings.Contains(Display(v), "title") || strings.Contains(Display(v), "31") || strings.Contains(Display(v), "unfinished") {
			t.Fatalf("control leaked: %q", Display(v))
		}
	}
}
func TestBoundsAndFallback(t *testing.T) {
	now := time.Now().UTC()
	raw := strings.Repeat("界", 5462) + "x"
	line, _ := Parse(raw, "s", "h", "", time.Time{}, now, false)
	if len(line.Raw) > MaxLineBytes || !line.Truncated || !line.Time.Equal(now) {
		t.Fatalf("boundary: %+v", line)
	}
	for _, raw := range []string{`[]`, `"x"`, `{"timestamp":"bad","app":4,"message":[]}`, `{"other":"value"}`} {
		v, a := Parse(raw, "", "h", "", now, now, false)
		if a != nil || v.Message != raw || !v.Time.Equal(now) {
			t.Fatalf("fallback %q: %+v %+v", raw, v, a)
		}
	}
	v, a := Parse(`{"seq":1,"hash":"h","fields":[],"action":""}`, "", "h", "", now, now, false)
	if a != nil || v.Raw == "" {
		t.Fatal("missing action should not be activity")
	}
}
func TestRealLoggerLines(t *testing.T) {
	var buf bytes.Buffer
	lg, err := logging.New(logging.Config{App: "kytest", Out: &buf})
	if err != nil {
		t.Fatal(err)
	}
	ev := logging.DeclareEvent("login", "signed in", slog.LevelInfo)
	lg.Log(context.Background(), ev, logging.UserID("alice"))
	line, a := Parse(strings.TrimSpace(buf.String()), "src", "host", "", time.Time{}, time.Now(), false)
	if a != nil || line.App != "kytest" || line.Event != "login" || line.Message != "signed in" || line.Level != "INFO" {
		t.Fatalf("ordinary: %+v %+v", line, a)
	}
	buf.Reset()
	chain, err := auditchain.New(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	fields := logging.AuditFields(logging.UserID("alice"), logging.Action("login"))
	rec, err := chain.Append(context.Background(), func(auditchain.Record, auditchain.Anchor) error { return nil }, fields...)
	if err != nil {
		t.Fatal(err)
	}
	if err = lg.Audit(context.Background(), ev, rec, logging.UserID("alice"), logging.Action("login")); err != nil {
		t.Fatal(err)
	}
	_, a = Parse(strings.TrimSpace(buf.String()), "src", "host", "", time.Time{}, time.Now(), false)
	if a == nil || a.Actor != "alice" || a.Action != "login" || a.App != "kytest" {
		t.Fatalf("audit: %+v", a)
	}
}
