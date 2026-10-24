package kyyard

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Busnes-app/kypulse-server/internal/ingest"
)

// FuzzParseLogs feeds upstream log bodies to parseLogs. Lines and notices stay bounded valid
// UTF-8, a truncated line always leaves a notice, and without a notice token every wire line
// is a log line: nothing the upstream writes can become a notice on its own say-so.
func FuzzParseLogs(f *testing.F) {
	stamp := "2026-09-27T12:00:00.123Z"
	for _, tc := range []struct{ body, token string }{
		{stamp + " hello\n--- kyyard forged: fake ---\n--- kyyard trusted: 10 bytes were dropped ---\n", "trusted"},
		{stamp + " " + strings.Repeat("x", 17000) + "\n", ""},
		{strings.Repeat("a", 8000) + "\xff" + strings.Repeat("b", 9000), ""},
		{"--- kyyard token: ---", "token"}, {"--- kyyard token:  ---\n--- kyyard token: \xff ---\r\n", "token"},
		{"\n\n\r\n", ""}, {"no-stamp line", ""}, {"2026-09-27T12:00:00+25:00 bad zone", ""}, {"", ""},
	} {
		f.Add([]byte(tc.body), tc.token)
	}
	f.Fuzz(func(t *testing.T, body []byte, token string) {
		p, err := parseLogs(body, token)
		if err != nil {
			t.Fatalf("parseLogs: %v", err)
		}
		if len(p.Lines) > ingest.MaxRecords || p.Capped != (len(p.Lines) == ingest.MaxRecords) {
			t.Fatalf("%d lines, capped=%v", len(p.Lines), p.Capped)
		}
		if len(p.Notice) > ingest.MaxLineBytes || !utf8.ValidString(p.Notice) {
			t.Fatalf("notice out of bounds: %d bytes", len(p.Notice))
		}
		for i, r := range p.Lines {
			if len(r.Line) > ingest.MaxLineBytes || !utf8.ValidString(r.Line) {
				t.Fatalf("line %d out of bounds: %d bytes", i, len(r.Line))
			}
			if r.Truncated && p.Notice == "" {
				t.Fatalf("line %d truncated without a notice", i)
			}
		}
		if token != "" {
			return
		}
		wire := 0
		if len(body) > 0 {
			wire = bytes.Count(body, []byte{'\n'}) + 1
			if body[len(body)-1] == '\n' {
				wire--
			}
		}
		if len(p.Lines) != min(wire, ingest.MaxRecords) {
			t.Fatalf("%d wire lines became %d log lines", wire, len(p.Lines))
		}
		for _, n := range strings.Split(p.Notice, "; ") {
			if n != "" && !strings.HasPrefix(n, "history may be incomplete: ") {
				t.Fatalf("untokened body produced notice %q", n)
			}
		}
	})
}
