package logstore

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// FuzzDisplay feeds untrusted log text to Display. The output is valid UTF-8 with no controls
// or format characters, Display is idempotent, and text with nothing to strip passes unchanged.
func FuzzDisplay(f *testing.F) {
	for _, s := range []string{
		"\x1b[31m<img onerror=x>\x1b[0m\ufffd", "a\x1b]0;title\aZ", "a\x1b]0;title\x1b\\Z", "a\x1b[31", "a\u009b31mZ",
		"a\x1b]unfinished", "a\u202eb\u202ac\u2066d\u2069e\u200bf\u200d\ufeffg\u00adh", "\x1bP\x1b\\", "\u009d\x07",
		"\xff\xfe", "\x1b", "\u0090x", strings.Repeat("界", 10) + "x", "plain <b>text</b>",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := Display(s)
		if !utf8.ValidString(got) || len(got) > len(s) {
			t.Fatalf("Display(%q) = %q", s, got)
		}
		clean := func(r rune) bool {
			return r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f) && !unicode.Is(unicode.Cf, r)
		}
		for _, r := range got {
			if !clean(r) {
				t.Fatalf("Display(%q) kept %U", s, r)
			}
		}
		if again := Display(got); again != got {
			t.Fatalf("not idempotent: %q then %q", got, again)
		}
		if utf8.ValidString(s) && strings.IndexFunc(s, func(r rune) bool { return !clean(r) }) < 0 && got != s {
			t.Fatalf("plain text %q changed to %q", s, got)
		}
	})
}
