package ingest

import (
	"bytes"
	"strings"
	"testing"
)

func TestBadLastRecordRejectsBatch(t *testing.T) {
	rows, err := Decode([]byte("{\"line\":\"first\"}\n{\"line\":7}\n"))
	if err == nil || rows != nil {
		t.Fatalf("accepted malformed batch: %#v %v", rows, err)
	}
}

func TestDecodeFramingAndBounds(t *testing.T) {
	for _, body := range [][]byte{nil, []byte("\n"), []byte("{\"line\":\"ok\"}\n\n"), []byte("{\"x\":1}"), []byte("{\"line\":null}"), []byte("{\"line\":\"a\"}\n\xff"), bytes.Repeat([]byte("x"), MaxRequestBytes+1)} {
		if rows, err := Decode(body); err == nil || rows != nil {
			t.Errorf("accepted %q: %#v %v", body, rows, err)
		}
	}
	for _, body := range []string{"{\"line\":\"\"}", "{\"line\":\"a\"}\n", "{\"line\":\"a\"}\r\n"} {
		if rows, err := Decode([]byte(body)); err != nil || len(rows) != 1 {
			t.Errorf("rejected %q: %#v %v", body, rows, err)
		}
	}
	line := strings.Repeat("a", MaxLineBytes-1) + "😀"
	rows, err := Decode([]byte("{\"line\":\"" + line + "\"}"))
	if err != nil || len(rows) != 1 || len(rows[0].Line) != MaxLineBytes-1 || !rows[0].Truncated {
		t.Fatalf("bad multibyte truncation: %#v %v", rows, err)
	}
}
