// Package logstore parses untrusted application lines for bounded storage and safe display.
package logstore

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

const MaxLineBytes = 16 << 10

func bounded(s string) (string, bool) {
	if len(s) <= MaxLineBytes {
		return s, false
	}
	s = s[:MaxLineBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s, true
}

// Parse keeps original bounded text while extracting the suite logging shape when present.
func Parse(raw, sourceID, source, targetID string, transportTime, receivedAt time.Time, truncated bool) (store.LogLine, *store.Activity) {
	raw, cut := bounded(raw)
	truncated = truncated || cut
	sourceID, _ = bounded(sourceID)
	source, _ = bounded(source)
	targetID, _ = bounded(targetID)
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	if transportTime.IsZero() {
		transportTime = receivedAt
	}
	line := store.LogLine{Time: transportTime, ReceivedAt: receivedAt, SourceID: sourceID, Source: source, TargetID: targetID, Message: raw, Raw: raw, Truncated: truncated}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil || obj == nil {
		return line, nil
	}
	get := func(k string) string { var s string; _ = json.Unmarshal(obj[k], &s); s, _ = bounded(s); return s }
	if ts := get("timestamp"); ts != "" {
		if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			line.Time = t
		}
	}
	line.App = get("app")
	line.Level = get("level")
	line.Event = get("event")
	if m := get("message"); m != "" {
		line.Message = m
	}
	var seq int64
	_ = json.Unmarshal(obj["seq"], &seq)
	var fields []string
	if seq <= 0 || get("hash") == "" || json.Unmarshal(obj["fields"], &fields) != nil || get("action") == "" {
		return line, nil
	}
	outcome := get("outcome")
	if outcome == "" {
		outcome = get("result")
	}
	a := &store.Activity{Time: line.Time, ReceivedAt: receivedAt, SourceID: sourceID, TargetID: targetID, App: line.App, Actor: get("user_id"), Action: get("action"), Target: get("resource"), Outcome: outcome, IP: get("ip_address")}
	return line, a
}

// Display removes terminal controls while preserving ordinary text for HTML escaping by React.
func Display(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			i++
			continue
		}
		switch r {
		case '\x1b':
			i += n
			if i >= len(s) {
				break
			}
			x := s[i]
			i++
			if x == '[' {
				for i < len(s) {
					c := s[i]
					i++
					if c >= 0x40 && c <= 0x7e {
						break
					}
				}
				continue
			}
			if x == ']' || x == 'P' || x == '^' || x == '_' {
				for i < len(s) {
					if s[i] == '\a' {
						i++
						break
					}
					if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
				continue
			}
			continue
		case '\u009b':
			i += n
			for i < len(s) {
				c := s[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					break
				}
			}
			continue
		case '\u009d', '\u0090', '\u009e', '\u009f':
			i += n
			for i < len(s) {
				if s[i] == '\a' {
					i++
					break
				}
				if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			i += n
			continue
		}
		b.WriteRune(r)
		i += n
	}
	return b.String()
}
