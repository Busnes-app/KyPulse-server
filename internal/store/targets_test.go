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
	if err := st.Targets().SetTrack(ctx, "tgt_1", `{"state":"down","until_fixed":true}`); err != nil {
		t.Fatal(err)
	}
	got, _ = st.Targets().GetTarget(ctx, "tgt_1")
	if got.TrackJSON != `{"state":"down","until_fixed":true}` {
		t.Fatalf("set track: %q", got.TrackJSON)
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
	all, total, err := st.Targets().ListEvents(ctx, "", 0, 10)
	if err != nil || total != 3 || len(all) != 3 || !all[0].At.Equal(base.Add(2*time.Minute)) {
		t.Fatalf("all: %d %v %+v", total, err, all)
	}
	onlyA, total, _ := st.Targets().ListEvents(ctx, "a", 0, 1)
	if total != 2 || len(onlyA) != 1 || onlyA[0].TargetID != "a" {
		t.Fatalf("a: %d %+v", total, onlyA)
	}
	if err := st.Targets().SetEventNotified(ctx, all[0].ID, false, "receiver answered 503"); err != nil {
		t.Fatal(err)
	}
	after, _, _ := st.Targets().ListEvents(ctx, "", 0, 1)
	if after[0].Notified || after[0].NotifyError != "receiver answered 503" {
		t.Fatalf("notified: %+v", after[0])
	}
	if err := st.Targets().DeleteTarget(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, total, _ = st.Targets().ListEvents(ctx, "", 0, 10); total != 1 {
		t.Fatalf("events must cascade on delete: %d left", total)
	}
}
