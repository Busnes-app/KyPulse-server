package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestActivityBursts(t *testing.T) {
	s := newLogTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	var events []Activity
	add := func(app, actor, ip, action, outcome string, n int, start time.Time, step time.Duration) {
		for i := 0; i < n; i++ {
			events = append(events, Activity{Time: start.Add(time.Duration(i) * step), ReceivedAt: now, App: app, Actor: actor, IP: ip, Action: action, Outcome: outcome})
		}
	}
	add("four", "alice", "", "auth.login", "failure", 4, now, time.Minute)
	add("boundary", "alice", "", "auth.login", "failure", 5, now, 75*time.Second)
	add("outside", "alice", "", "auth.login", "failure", 5, now, 75*time.Second+time.Microsecond)
	add("cross1", "shared", "", "auth.login", "failure", 3, now, 0)
	add("cross2", "shared", "", "auth.login", "failure", 2, now, 0)
	add("blank", "", "", "auth.login", "failure", 5, now, 0)
	add("ip", "", "192.0.2.1", "sign-in", "failure", 5, now, 0)
	add("wrong", "alice", "", "contains.login", "failure", 5, now, 0)
	add("success", "alice", "", "auth.login", "success", 5, now, 0)
	if err := s.Logs().Append(ctx, "", LogBatch{Activity: events}, 1<<24); err != nil {
		t.Fatal(err)
	}
	rule := ActivityBurstRule{Actions: []string{"auth.login", "sign-in"}, Outcome: "failure", Minimum: 5, Window: 5 * time.Minute}
	got, err := s.Logs().ActivityBursts(ctx, ActivityFilter{BeforeID: 1, Limit: 1}, rule)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].App != "boundary" || got[0].Count != 5 || got[1].App != "ip" {
		t.Fatalf("bursts: %+v", got)
	}
	got, err = s.Logs().ActivityBursts(ctx, ActivityFilter{App: "boundary", From: now.Add(time.Second)}, rule)
	if err != nil || len(got) != 0 {
		t.Fatalf("filters: %+v %v", got, err)
	}
	events = nil
	for i := 0; i < 105; i++ {
		add(fmt.Sprintf("app%03d", i), "alice", "", "auth.login", "failure", 5, now.Add(time.Duration(i)*time.Minute), 0)
	}
	if err := s.Logs().Append(ctx, "", LogBatch{Activity: events}, 1<<24); err != nil {
		t.Fatal(err)
	}
	got, err = s.Logs().ActivityBursts(ctx, ActivityFilter{}, rule)
	if err != nil || len(got) != 100 || got[0].App != "app104" {
		t.Fatalf("bound: %d %+v %v", len(got), got, err)
	}
}
