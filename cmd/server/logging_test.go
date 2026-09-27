package main

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// Every line the process writes must be one JSON object: the stdlib log package and raw slog
// calls both route through the ky-primitives handler, and a newline inside a value cannot
// split a record, so a caller-controlled string cannot forge a log line.
func TestLogBridgeEmitsOneJSONLinePerCall(t *testing.T) {
	prev := slog.Default()
	var buf bytes.Buffer
	if _, err := newLogger(&buf); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
		slog.SetDefault(prev)
	})

	log.Printf("[KYPULSE] listening on %s", "127.0.0.1:8080\nlevel=FATAL forged=1")
	slog.Warn("raw slog", "user_id", "u1", "password", "hunter2")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d:\n%s", len(lines), buf.String())
	}
	for i, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", i, err, l)
		}
		if m["app"] != "kypulse" {
			t.Errorf("line %d app = %v", i, m["app"])
		}
	}
	if strings.Contains(lines[0], "\n") || !strings.Contains(lines[0], "listening on 127.0.0.1:8080") {
		t.Errorf("stdlib line was not sanitised into one record: %s", lines[0])
	}
	if strings.Contains(lines[1], "hunter2") {
		t.Errorf("undeclared slog attribute reached the line: %s", lines[1])
	}
}

// With KY_LOG_LEVEL=warn, slog.NewLogLogger(h, slog.LevelInfo).Writer() drops every ordinary
// log.Printf/log.Fatalf line before it reaches the handler -- silently, since the write still
// reports success. fatal must bypass that gate so a fatal exit is never a silent one.
func TestFatalLinesEscapeTheLevelGate(t *testing.T) {
	t.Setenv("KY_LOG_LEVEL", "warn")
	prev := slog.Default()
	var buf bytes.Buffer
	if _, err := newLogger(&buf); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
		slog.SetDefault(prev)
	})

	log.Printf("info line")
	if buf.Len() != 0 {
		t.Fatalf("info line was not gated by KY_LOG_LEVEL=warn: %s", buf.String())
	}

	fatalf("bootstrap failed: %s", "disk full")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("want 1 line, got %d:\n%s", len(lines), buf.String())
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("fatal line is not JSON: %v\n%s", err, lines[0])
	}
	if m["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", m["level"])
	}
	if m["event"] != "fatal" {
		t.Errorf("event = %v, want fatal", m["event"])
	}
	if m["detail"] != "bootstrap failed: disk full" {
		t.Errorf("detail = %v", m["detail"])
	}
}
