package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// FuzzDecode feeds sender request bodies to Decode. A rejected body yields no records; an
// accepted one satisfies the wire bounds and re-encodes to a body that decodes to the same rows.
func FuzzDecode(f *testing.F) {
	for _, s := range []string{
		"{\"line\":\"first\"}\n{\"line\":7}\n", "\n", "{\"line\":\"ok\"}\n\n", "{\"x\":1}", "{\"line\":null}",
		"{\"line\":\"a\"}\n\xff", "{\"line\":\"\"}", "{\"line\":\"a\"}\n", "{\"line\":\"a\"}\r\n",
		"{\"line\":\"" + strings.Repeat("a", MaxLineBytes-1) + "😀\"}",
		`{"line":"x","time":"2026-09-27T12:00:00.123+02:00","truncated":true}`,
		`{"line":"x","time":"0000-01-01T00:00:00+01:00"}`, `{"line":"x","time":"9999-12-31T23:59:59-01:00"}`,
		`{"line":"x","time":""}`, `{"line":"x","truncated":null}`, `{"line":"\u0000\ud800"}`, `{"line":"a","line":"b"}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		rows, err := Decode(body)
		if err != nil {
			if rows != nil || !errors.Is(err, ErrInvalidBatch) {
				t.Fatalf("rejection returned %#v, %v", rows, err)
			}
			return
		}
		if len(rows) == 0 || len(rows) > MaxRecords {
			t.Fatalf("accepted %d records", len(rows))
		}
		var again bytes.Buffer
		enc := json.NewEncoder(&again)
		enc.SetEscapeHTML(false)
		for _, r := range rows {
			if len(r.Line) > MaxLineBytes || !utf8.ValidString(r.Line) {
				t.Fatalf("line out of bounds: %d bytes, valid=%v", len(r.Line), utf8.ValidString(r.Line))
			}
			if r.Time.Location() != time.UTC || r.Time.Year() < 0 || r.Time.Year() > 9999 {
				t.Fatalf("unstorable time %v", r.Time)
			}
			if err := enc.Encode(r); err != nil {
				t.Fatal(err)
			}
		}
		if again.Len() > MaxRequestBytes {
			return // escaping can grow a body that sat near the cap
		}
		back, err := Decode(again.Bytes())
		if err != nil || len(back) != len(rows) {
			t.Fatalf("re-encoded batch: %d rows, %v", len(back), err)
		}
		for i := range rows {
			if back[i].Line != rows[i].Line || back[i].Truncated != rows[i].Truncated || !back[i].Time.Equal(rows[i].Time) {
				t.Fatalf("row %d: %#v became %#v", i, rows[i], back[i])
			}
		}
	})
}
