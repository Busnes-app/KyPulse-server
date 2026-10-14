package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/store"
)

func TestTargetLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	tg := &store.Target{ID: "tgt_1", Name: "KyVault", URL: "https://vault.lan/healthz", IntervalSec: 30, Enabled: true}
	if err := st.Targets().CreateTarget(ctx, tg); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "tgt_2", Name: "KyVault", URL: "https://x/", IntervalSec: 30}); !errors.Is(err, store.ErrAlreadyExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	got, err := st.Targets().GetTarget(ctx, "tgt_1")
	if err != nil || got.Name != "KyVault" || got.State != "pending" || got.LastPolledAt != nil || !got.Enabled {
		t.Fatalf("get: %+v %v", got, err)
	}
	if _, err := st.Targets().GetTarget(ctx, "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}

	got.Name, got.IntervalSec, got.Enabled = "KyVault (prod)", 60, false
	if err := st.Targets().UpdateTarget(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	list, _ := st.Targets().ListTargets(ctx)
	if len(list) != 1 || list[0].Name != "KyVault (prod)" || list[0].IntervalSec != 60 || list[0].Enabled {
		t.Fatalf("list after update: %+v", list[0])
	}

	now := time.Date(2026, 9, 26, 10, 41, 0, 0, time.UTC)
	err = st.Targets().RecordPoll(ctx, "tgt_1", store.PollUpdate{PolledAt: now, LatencyMS: 12, LastResult: `{"state":"down","cause":"refused"}`, TrackJSON: `{"state":"down"}`, State: "down", StateSince: now, Cause: "refused"})
	if err != nil {
		t.Fatalf("record poll: %v", err)
	}
	got, _ = st.Targets().GetTarget(ctx, "tgt_1")
	if got.State != "down" || got.Cause != "refused" || got.LastPolledAt == nil || !got.LastPolledAt.Equal(now) || got.LastLatencyMS != 12 || got.TrackJSON != `{"state":"down"}` {
		t.Fatalf("after poll: %+v", got)
	}
	until := now.Add(time.Hour)
	if err := st.Targets().SetSilence(ctx, "tgt_1", &until, false); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Targets().GetTarget(ctx, "tgt_1")
	if got.SilencedUntil == nil || !got.SilencedUntil.Equal(until) || got.UntilFixed {
		t.Fatalf("set silence: %+v", got)
	}
	if err := st.Targets().SetSilence(ctx, "tgt_1", nil, true); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Targets().GetTarget(ctx, "tgt_1")
	if got.SilencedUntil != nil || !got.UntilFixed {
		t.Fatalf("until fixed: %+v", got)
	}
	if err := st.Targets().SetSilence(ctx, "tgt_1", nil, false); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Targets().GetTarget(ctx, "tgt_1")
	if got.SilencedUntil != nil || got.UntilFixed {
		t.Fatalf("clear silence: %+v", got)
	}

	if err := st.Targets().DeleteTarget(ctx, "tgt_1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Targets().GetTarget(ctx, "tgt_1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestTargetEvents(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	for _, id := range []string{"a", "b"} {
		if err := st.Targets().CreateTarget(ctx, &store.Target{ID: id, Name: id, URL: "https://" + id + "/", IntervalSec: 30, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	for i, tid := range []string{"a", "b", "a"} {
		e := &store.TargetEvent{TargetID: tid, At: base.Add(time.Duration(i) * time.Minute), FromState: "ok", ToState: "down", Cause: "refused"}
		if err := st.Targets().RecordEvent(ctx, e); err != nil || e.ID == 0 {
			t.Fatalf("record %d: id=%d %v", i, e.ID, err)
		}
	}
	all, err := st.Targets().ListEvents(ctx, "", 0, 10)
	total := len(all)
	if err != nil || total != 3 || len(all) != 3 || !all[0].At.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("all: %d %v %+v", total, err, all)
	}
	onlyA, _ := st.Targets().ListEvents(ctx, "a", 0, 1)
	olderA, _ := st.Targets().ListEvents(ctx, "a", onlyA[0].ID, 10)
	if len(onlyA) != 1 || onlyA[0].TargetID != "a" || len(olderA) != 1 || olderA[0].ID >= onlyA[0].ID || olderA[0].TargetID != "a" {
		t.Fatalf("a: %+v %+v", onlyA, olderA)
	}
	if err := st.Targets().SetEventNotified(ctx, all[0].ID, false, "receiver answered 503"); err != nil {
		t.Fatal(err)
	}
	after, _ := st.Targets().ListEvents(ctx, "", 0, 1)
	if after[0].Notified || after[0].NotifyError != "receiver answered 503" {
		t.Fatalf("notified: %+v", after[0])
	}
	if err := st.Targets().DeleteTarget(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.Targets().ListEvents(ctx, "", 0, 10); len(left) != 1 {
		t.Fatalf("events must cascade on delete: %d left", len(left))
	}
}

func TestPruneEventsKeepsRecentAndPending(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "a", URL: "https://a/", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.Add(-100 * 24 * time.Hour)
	for _, e := range []*store.TargetEvent{
		{TargetID: "a", At: old, FromState: "ok", ToState: "down"},
		{TargetID: "a", At: old, FromState: "ok", ToState: "down", Cause: "offboarding_failed", NotifyError: "pending"},
		{TargetID: "a", At: now, FromState: "down", ToState: "ok"},
	} {
		if err := st.Targets().RecordEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Targets().PruneEvents(ctx, now.Add(-90*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	left, err := st.Targets().ListEvents(ctx, "", 0, 10)
	if err != nil || len(left) != 2 || left[0].ToState != "ok" || left[1].Cause != "offboarding_failed" {
		t.Fatalf("after prune: %+v %v", left, err)
	}
}

// A recovery clears "until fixed" only: a timed silence set meanwhile must survive it.
func TestClearUntilFixedKeepsATimedSilence(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	if err := st.Targets().CreateTarget(ctx, &store.Target{ID: "a", Name: "a", URL: "https://a/", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	until := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := st.Targets().SetSilence(ctx, "a", &until, false); err != nil {
		t.Fatal(err)
	}
	if err := st.Targets().ClearUntilFixed(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if tg, _ := st.Targets().GetTarget(ctx, "a"); tg.SilencedUntil == nil || !tg.SilencedUntil.Equal(until) {
		t.Fatalf("timed silence lost: %+v", tg)
	}
	if err := st.Targets().SetSilence(ctx, "a", nil, true); err != nil {
		t.Fatal(err)
	}
	if err := st.Targets().ClearUntilFixed(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if tg, _ := st.Targets().GetTarget(ctx, "a"); tg.UntilFixed || tg.SilencedUntil != nil {
		t.Fatalf("until fixed not cleared: %+v", tg)
	}
	if err := st.Targets().ClearUntilFixed(ctx, "missing"); err != nil {
		t.Fatalf("nothing to clear is not an error: %v", err)
	}
}

func TestUpdateTargetResetsHealthOnNewURL(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	tg := &store.Target{ID: "a", Name: "a", URL: "https://a/", IntervalSec: 30, Enabled: true}
	if err := st.Targets().CreateTarget(ctx, tg); err != nil {
		t.Fatal(err)
	}
	polled := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if err := st.Targets().RecordPoll(ctx, "a", store.PollUpdate{PolledAt: polled, LatencyMS: 5, LastResult: `{"state":"down"}`,
		TrackJSON: `{"state":"down","down_streak":5}`, State: "down", StateSince: polled, Cause: "refused"}); err != nil {
		t.Fatal(err)
	}
	tg.Name = "renamed"
	if err := st.Targets().UpdateTarget(ctx, tg); err != nil {
		t.Fatal(err)
	}
	same, _ := st.Targets().GetTarget(ctx, "a")
	if same.State != "down" || same.TrackJSON == "" || same.Cause != "refused" || same.LastPolledAt == nil || same.Name != "renamed" {
		t.Fatalf("same URL must keep health: %+v", same)
	}
	tg.URL = "https://b/"
	if err := st.Targets().UpdateTarget(ctx, tg); err != nil {
		t.Fatal(err)
	}
	moved, _ := st.Targets().GetTarget(ctx, "a")
	if moved.URL != "https://b/" || moved.State != "pending" || moved.TrackJSON != "" || moved.Cause != "" || moved.LastResult != "" ||
		moved.LastPolledAt != nil || moved.LastLatencyMS != 0 || !moved.StateSince.After(polled) {
		t.Fatalf("new URL must reset health: %+v", moved)
	}
}
