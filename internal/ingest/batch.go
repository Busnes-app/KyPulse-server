// Package ingest validates the bounded NDJSON wire format for source logs.
package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"
)

const MaxRequestBytes = 1 << 20
const MaxLineBytes = 16 << 10
const MaxRecords = 1000

var ErrInvalidBatch = errors.New("invalid log batch")

type Record struct {
	Line      string    `json:"line"`
	Time      time.Time `json:"time,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}

// Decode either validates the entire body or returns no records.
func Decode(body []byte) ([]Record, error) {
	if len(body) == 0 || len(body) > MaxRequestBytes || !utf8.Valid(body) {
		return nil, ErrInvalidBatch
	}
	parts := bytes.Split(body, []byte{'\n'})
	if len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 || len(parts) > MaxRecords {
		return nil, ErrInvalidBatch
	}
	rows := make([]Record, 0, len(parts))
	for _, part := range parts {
		part = bytes.TrimSuffix(part, []byte{'\r'})
		if len(part) == 0 {
			return nil, ErrInvalidBatch
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(part, &fields); err != nil || fields == nil {
			return nil, ErrInvalidBatch
		}
		line, ok := fields["line"]
		var row Record
		if !ok || string(line) == "null" || json.Unmarshal(line, &row.Line) != nil {
			return nil, ErrInvalidBatch
		}
		if raw, ok := fields["time"]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || value == "" {
				return nil, ErrInvalidBatch
			}
			var err error
			row.Time, err = time.Parse(time.RFC3339Nano, value)
			if err != nil {
				return nil, ErrInvalidBatch
			}
		}
		if raw, ok := fields["truncated"]; ok {
			if json.Unmarshal(raw, &row.Truncated) != nil || string(raw) == "null" {
				return nil, ErrInvalidBatch
			}
		}
		if len(row.Line) > MaxLineBytes {
			row.Line = row.Line[:MaxLineBytes]
			for !utf8.ValidString(row.Line) {
				row.Line = row.Line[:len(row.Line)-1]
			}
			row.Truncated = true
		}
		rows = append(rows, row)
	}
	return rows, nil
}
