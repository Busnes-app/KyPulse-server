package api

import (
	"testing"
	"time"
)

func TestIngestSourceWindowUsesFixedClock(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	s := &Server{ingestWindows: make(map[string]attemptWindow), ingestNow: func() time.Time { return now }}
	for i := 0; i < 60; i++ {
		if !s.allowIngest("a") {
			t.Fatalf("request %d rejected", i+1)
		}
	}
	if s.allowIngest("a") || !s.allowIngest("b") {
		t.Fatal("source quota not scoped")
	}
	now = now.Add(time.Minute)
	if !s.allowIngest("a") {
		t.Fatal("quota did not reset")
	}
}
