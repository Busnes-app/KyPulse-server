package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func newLogTestStore(t *testing.T) *SQLStore {
	t.Helper()
	v, err := Open(context.Background(), testdb.Config(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { v.Close() })
	return v.(*SQLStore)
}
func testLine(now time.Time, msg string) LogLine {
	return LogLine{Time: now, ReceivedAt: now, Source: "host", App: "app", Message: msg, Raw: msg}
}
func usage(t *testing.T, s *SQLStore) int64 {
	t.Helper()
	var n int64
	if err := s.db.QueryRow("SELECT bytes FROM log_usage WHERE id=1").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestLogAtomicityAndAccounting(t *testing.T) {
	ctx := context.Background()
	s := newLogTestStore(t)
	now := time.Now().UTC()
	if err := s.Logs().Append(ctx, "unknown", LogBatch{Logs: []LogLine{testLine(now, "rejected")}}, 100000); err == nil {
		t.Fatal("unknown source accepted")
	}
	if usage(t, s) != 0 {
		t.Fatal("source rejection changed usage")
	}
	batch := LogBatch{Logs: []LogLine{testLine(now, "good"), testLine(now, "bad")}, Activity: []Activity{{Time: now, ReceivedAt: now, Action: "event", ExternalKey: "same"}, {Time: now, ReceivedAt: now, Action: "event", ExternalKey: "same"}}}
	if err := s.Logs().Append(ctx, "", batch, 100000); err != nil {
		t.Fatal(err)
	}
	logs, err := s.Logs().List(ctx, LogFilter{})
	if err != nil || len(logs) != 2 {
		t.Fatalf("logs=%d err=%v", len(logs), err)
	}
	activity, err := s.Logs().ListActivity(ctx, ActivityFilter{})
	if err != nil || len(activity) != 1 {
		t.Fatalf("activity=%d err=%v", len(activity), err)
	}
	expected := logs[0].Bytes + logs[1].Bytes + activity[0].Bytes
	if got := usage(t, s); got != expected {
		t.Fatalf("usage=%d want %d", got, expected)
	}
	if err := s.Logs().Append(ctx, "", LogBatch{Activity: []Activity{{Time: now, ReceivedAt: now, Action: "event", ExternalKey: "same"}}}, 100000); err != nil {
		t.Fatal(err)
	}
	if got := usage(t, s); got != expected {
		t.Fatalf("duplicate changed usage: %d", got)
	}
	if err := s.Logs().Append(ctx, "", LogBatch{Activity: []Activity{{Time: now, ReceivedAt: now, Action: "changed", ExternalKey: "same"}}}, 100000); err != ErrAlreadyExists {
		t.Fatalf("conflicting replay: %v", err)
	}
	if got := usage(t, s); got != expected {
		t.Fatalf("collision changed usage: %d", got)
	}
	// A failed activity insert must roll back earlier log rows and usage together.
	if s.driver == "sqlite" {
		_, err = s.db.Exec(`CREATE TRIGGER fail_activity BEFORE INSERT ON activity BEGIN SELECT RAISE(ABORT, 'activity failed'); END`)
	}
	if s.driver == "postgres" {
		_, err = s.db.Exec(`CREATE FUNCTION fail_activity() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'activity failed'; END $$`)
		if err == nil {
			_, err = s.db.Exec(`CREATE TRIGGER fail_activity BEFORE INSERT ON activity FOR EACH ROW EXECUTE FUNCTION fail_activity()`)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	err = s.Logs().Append(ctx, "", LogBatch{Logs: []LogLine{testLine(now, "rollback")}, Activity: []Activity{{Time: now, ReceivedAt: now, Action: "fails"}}}, 100000)
	if err == nil {
		t.Fatal("trigger failure accepted")
	}
	logs, err = s.Logs().List(ctx, LogFilter{Text: "rollback"})
	if err != nil || len(logs) != 0 || usage(t, s) != expected {
		t.Fatalf("rollback leaked: logs=%v err=%v", logs, err)
	}
}
func TestLogPagesFiltersAndRetention(t *testing.T) {
	ctx := context.Background()
	s := newLogTestStore(t)
	now := time.Now().UTC()
	batch := LogBatch{Logs: []LogLine{testLine(now, "literal %_"), testLine(now, "wild xyz"), testLine(now, "newest")}, Activity: []Activity{{Time: now, ReceivedAt: now, Action: "a", Actor: "alice"}, {Time: now, ReceivedAt: now, Action: "b", Actor: "bob"}}}
	if err := s.Logs().Append(ctx, "", batch, 100000); err != nil {
		t.Fatal(err)
	}
	found, err := s.Logs().List(ctx, LogFilter{Text: "%_"})
	if err != nil || len(found) != 1 || found[0].Message != "literal %_" {
		t.Fatalf("literal filter: %+v %v", found, err)
	}
	page, err := s.Logs().List(ctx, LogFilter{Limit: 2})
	if err != nil || len(page) != 2 || page[0].ID <= page[1].ID {
		t.Fatalf("page: %+v %v", page, err)
	}
	next, err := s.Logs().List(ctx, LogFilter{BeforeID: page[1].ID})
	if err != nil || len(next) != 1 || next[0].ID >= page[1].ID {
		t.Fatalf("next page: %+v %v", next, err)
	}
	activities, err := s.Logs().ListActivity(ctx, ActivityFilter{Actor: "alice"})
	if err != nil || len(activities) != 1 {
		t.Fatalf("activity filter: %+v %v", activities, err)
	}
	encoded, err := json.Marshal(activities[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"external_key", "bytes"} {
		if strings.Contains(string(encoded), hidden) {
			t.Fatalf("exposed %s: %s", hidden, encoded)
		}
	}
	// The cap uses received order rather than application time and includes activity.
	if err := s.Logs().Prune(ctx, now, 0); err != nil {
		t.Fatal(err)
	}
	if usage(t, s) != 0 {
		t.Fatalf("usage after prune: %d", usage(t, s))
	}
	all, _ := s.Logs().List(ctx, LogFilter{})
	act, _ := s.Logs().ListActivity(ctx, ActivityFilter{})
	if len(all) != 0 || len(act) != 0 {
		t.Fatalf("prune left rows: %d %d", len(all), len(act))
	}
}
func TestLogsSurviveTargetDeletionAndAgePrune(t *testing.T) {
	ctx := context.Background()
	s := newLogTestStore(t)
	now := time.Now().UTC()
	target := &Target{ID: strings.Repeat("x", 64), Name: "target-log-test", URL: "https://example.com"}
	if err := s.Targets().CreateTarget(ctx, target); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-8 * 24 * time.Hour)
	batch := LogBatch{Logs: []LogLine{{Time: now.Add(100 * 24 * time.Hour), ReceivedAt: old, Source: "host", TargetID: target.ID, Message: "old", Raw: "old"}, testLine(now, "recent")}, Activity: []Activity{{Time: now, ReceivedAt: now, TargetID: target.ID, Action: "update"}}}
	if err := s.Logs().Append(ctx, "", batch, 100000); err != nil {
		t.Fatal(err)
	}
	beforeDelete := usage(t, s)
	if err := s.Targets().DeleteTarget(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := usage(t, s), beforeDelete-2*int64(len(target.ID)); got != want {
		t.Fatalf("target deletion usage=%d, want %d", got, want)
	}
	activities, err := s.Logs().ListActivity(ctx, ActivityFilter{})
	if err != nil || len(activities) != 1 || activities[0].TargetID != "" {
		t.Fatalf("deleted target activity: %+v %v", activities, err)
	}
	if activities[0].Bytes != activityBytes(activities[0]) {
		t.Fatalf("activity size changed after target deletion: %+v", activities[0])
	}
	logs, err := s.Logs().List(ctx, LogFilter{})
	if err != nil || len(logs) != 2 || logs[1].TargetID != "" || logs[1].Bytes != logBytes(logs[1]) {
		t.Fatalf("deleted target log: %+v %v", logs, err)
	}
	newLine := testLine(now.Add(time.Minute), "after delete")
	if err := s.Logs().Append(ctx, "", LogBatch{Logs: []LogLine{newLine}}, 100000); err != nil {
		t.Fatal(err)
	}
	if got, want := usage(t, s), beforeDelete-2*int64(len(target.ID))+logBytes(newLine); got != want {
		t.Fatalf("usage after append=%d, want %d", got, want)
	}
	if err := s.Logs().Prune(ctx, now, 100000); err != nil {
		t.Fatal(err)
	}
	logs, err = s.Logs().List(ctx, LogFilter{})
	if err != nil || len(logs) != 2 || logs[0].Message != "after delete" || logs[1].Message != "recent" {
		t.Fatalf("age prune: %+v %v", logs, err)
	}
	if got, want := usage(t, s), logs[0].Bytes+logs[1].Bytes+activities[0].Bytes; got != want {
		t.Fatalf("usage after prune=%d, want %d", got, want)
	}
}

func TestLogUsageAcrossAgeAndSizePruning(t *testing.T) {
	ctx := context.Background()
	s := newLogTestStore(t)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	old := now.Add(-8 * 24 * time.Hour)
	batch := LogBatch{
		Logs: []LogLine{testLine(old, "expired log"), testLine(now, "recent log")},
		Activity: []Activity{
			{Time: old, ReceivedAt: old, Action: "expired activity"},
			{Time: now, ReceivedAt: now, Action: "recent activity"},
		},
	}
	if err := s.Logs().Append(ctx, "", batch, 100000); err != nil {
		t.Fatal(err)
	}
	if got, want := usage(t, s), logBytes(batch.Logs[0])+logBytes(batch.Logs[1])+activityBytes(batch.Activity[0])+activityBytes(batch.Activity[1]); got != want {
		t.Fatalf("initial usage=%d want %d", got, want)
	}
	if err := s.Logs().Prune(ctx, now, 100000); err != nil {
		t.Fatal(err)
	}
	if got, want := usage(t, s), logBytes(batch.Logs[1])+activityBytes(batch.Activity[1]); got != want {
		t.Fatalf("usage after age pruning=%d want %d", got, want)
	}
	latest := testLine(now.Add(time.Minute), "latest")
	if err := s.Logs().Append(ctx, "", LogBatch{Logs: []LogLine{latest}}, logBytes(latest)); err != nil {
		t.Fatal(err)
	}
	logs, err := s.Logs().List(ctx, LogFilter{})
	if err != nil || len(logs) != 1 || logs[0].Message != "latest" {
		t.Fatalf("size pruning left logs=%+v err=%v", logs, err)
	}
	activity, err := s.Logs().ListActivity(ctx, ActivityFilter{})
	if err != nil || len(activity) != 0 || usage(t, s) != logs[0].Bytes {
		t.Fatalf("size pruning left activity=%+v usage=%d err=%v", activity, usage(t, s), err)
	}
}
