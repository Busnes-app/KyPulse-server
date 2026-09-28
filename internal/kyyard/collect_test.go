package kyyard

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/egress"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func collectionService(t *testing.T, h HTTP) (*Service, store.Store) {
	t.Helper()
	ctx := context.Background()
	cfg := &config.Config{Database: testdb.Config(t)}
	cfg.Security.EncryptionKey = make([]byte, 32)
	st, err := store.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p, err := NewPairing(cfg, st.Settings())
	if err != nil {
		t.Fatal(err)
	}
	logger, _ := logging.New(logging.Config{App: "test", Out: io.Discard})
	s := &Service{Pairing: p, HTTP: h, LogHTTP: h, Store: st, MaxLogBytes: 64 << 20, Logger: logger}
	if err = s.Replace(ctx, Config{URL: "https://yard.lan", Token: "secret", OrganizationID: "org"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err = st.Targets().CreateTarget(ctx, &store.Target{ID: id, Name: id, URL: "https://app.lan", Container: "ep/app", Enabled: false}); err != nil {
			t.Fatal(err)
		}
	}
	s.facts = map[string]ContainerFacts{"ep/app": {EndpointID: "ep", ContainerID: "ct"}}
	return s, st
}
func plain(body string) *egress.Response {
	return &egress.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/plain"}}, Body: []byte(body)}
}
func auditResponse(items []AuditEvent, after int64) *egress.Response {
	b, _ := json.Marshal(AuditPage{Items: items, NextAfterID: after})
	return &egress.Response{StatusCode: 200, Body: b}
}
func emptyAudit(u string) *egress.Response {
	parsed, _ := url.Parse(u)
	n, _ := strconv.ParseInt(parsed.Query().Get("after_id"), 10, 64)
	return auditResponse([]AuditEvent{}, n)
}

func TestCollectionSharedLinkInclusiveReplayAndAuditMapping(t *testing.T) {
	ctx := context.Background()
	stamp := "2026-09-27T12:00:00.123456789Z"
	fetches := 0
	var since []string
	h := collectionHTTP{get: func(_ context.Context, raw string, _ map[string]string) (*egress.Response, error) {
		u, _ := url.Parse(raw)
		if strings.HasSuffix(u.Path, "/logs") {
			fetches++
			since = append(since, u.Query().Get("since"))
			return plain(stamp + " same\n" + stamp + " same\n"), nil
		}
		if u.Query().Get("after_id") == "0" {
			return auditResponse([]AuditEvent{{ID: 7, UserID: "actor", Action: "change", Resource: "resource", Result: "success", IPAddress: "192.0.2.1", CreatedAt: time.Now()}}, 7), nil
		}
		return emptyAudit(raw), nil
	}}
	s, st := collectionService(t, h)
	s.CollectNow(ctx)
	logs, err := st.Logs().List(ctx, store.LogFilter{})
	if err != nil || len(logs) != 4 || fetches != 1 {
		t.Fatalf("logs=%d fetches=%d %v", len(logs), fetches, err)
	}
	for _, target := range []string{"a", "b"} {
		rows, _ := st.Logs().List(ctx, store.LogFilter{TargetID: target})
		if len(rows) != 2 {
			t.Fatalf("target %s rows=%d", target, len(rows))
		}
	}
	cfg, _, _ := s.Pairing.Load(ctx)
	cur, _ := st.Logs().Cursor(ctx, cursorKey(cfg, "logs", "ep", "ct"))
	if cur != stamp {
		t.Fatalf("cursor %q", cur)
	}
	activity, _ := st.Logs().ListActivity(ctx, store.ActivityFilter{})
	if len(activity) != 1 {
		t.Fatal(activity)
	}
	a := activity[0]
	if a.App != "kyyard" || a.Actor != "actor" || a.Target != "resource" || a.Outcome != "success" || a.IP != "192.0.2.1" || a.ExternalKey != cfg.Generation+":org:7" {
		t.Fatalf("mapping=%+v", a)
	}
	// Recreate the service after the committed request's acknowledgment is lost.
	restarted := &Service{Pairing: s.Pairing, HTTP: h, LogHTTP: h, Store: st, MaxLogBytes: s.MaxLogBytes, Logger: s.Logger}
	if err := restarted.Open(ctx); err != nil {
		t.Fatal(err)
	}
	restarted.facts = s.facts
	restarted.CollectNow(ctx)
	logs, _ = st.Logs().List(ctx, store.LogFilter{})
	activity, _ = st.Logs().ListActivity(ctx, store.ActivityFilter{})
	if len(logs) != 8 || len(activity) != 1 || len(since) != 2 || since[1] != stamp {
		t.Fatalf("restart/replay logs=%d activity=%d since=%v", len(logs), len(activity), since)
	}
}

func TestPairingChangeWhileRequestBlockedPreventsImport(t *testing.T) {
	for _, change := range []string{"save", "replace", "unpair"} {
		t.Run(change, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			first := true
			h := collectionHTTP{get: func(ctx context.Context, u string, _ map[string]string) (*egress.Response, error) {
				if strings.Contains(u, "/logs?") {
					if first {
						first = false
						close(entered)
						select {
						case <-release:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}
					return plain("2026-09-27T12:00:00Z old\n"), nil
				}
				return emptyAudit(u), nil
			}}
			s, st := collectionService(t, h)
			old, _, _ := s.Pairing.Load(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); s.CollectNow(context.Background()) }()
			<-entered
			cfg := Config{URL: "https://new.lan", Token: "replacement", OrganizationID: "org"}
			var err error
			switch change {
			case "save":
				err = s.Pairing.Save(context.Background(), cfg)
			case "replace":
				err = s.Replace(context.Background(), cfg)
			case "unpair":
				err = s.Unpair(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			close(release)
			<-done
			logs, _ := st.Logs().List(context.Background(), store.LogFilter{})
			activity, _ := st.Logs().ListActivity(context.Background(), store.ActivityFilter{})
			cur, _ := st.Logs().Cursor(context.Background(), cursorKey(old, "logs", "ep", "ct"))
			if len(logs) != 0 || len(activity) != 0 || cur != "" {
				t.Fatalf("obsolete commit rows=%d activity=%d cursor=%q", len(logs), len(activity), cur)
			}
		})
	}
}

func TestLegacyPairingGenerationStableAndReplaced(t *testing.T) {
	s, _ := collectionService(t, collectionHTTP{})
	legacy := Config{URL: "https://yard.lan", Token: "legacy", OrganizationID: "org"}
	data, _ := json.Marshal(legacy)
	sealed, _ := s.Pairing.Sealer.Seal(data)
	if err := s.Pairing.Settings.SetSetting(context.Background(), pairingKey, sealed); err != nil {
		t.Fatal(err)
	}
	first, _, err := s.Pairing.Load(context.Background())
	if err != nil || first.Generation == "" {
		t.Fatalf("migration: %+v %v", first, err)
	}
	second, _, _ := s.Pairing.Load(context.Background())
	if first.Generation != second.Generation {
		t.Fatal("legacy generation changed on read")
	}
	if err := s.Pairing.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	third, _, _ := s.Pairing.Load(context.Background())
	if third.Generation == first.Generation {
		t.Fatal("replacement reused generation")
	}
}

func TestAuditPagesCommitBeforeFailureAndCapTick(t *testing.T) {
	calls := 0
	failAfter := int64(200)
	var cursors []int64
	h := collectionHTTP{get: func(_ context.Context, raw string, _ map[string]string) (*egress.Response, error) {
		if strings.Contains(raw, "/logs?") {
			return plain(""), nil
		}
		calls++
		u, _ := url.Parse(raw)
		after, _ := strconv.ParseInt(u.Query().Get("after_id"), 10, 64)
		cursors = append(cursors, after)
		if after == failAfter {
			return nil, egress.ErrBodyTooLarge
		}
		items := make([]AuditEvent, 200)
		for i := range items {
			items[i] = AuditEvent{ID: after + int64(i) + 1, Action: "event", CreatedAt: time.Now()}
		}
		return auditResponse(items, after+200), nil
	}}
	s, st := collectionService(t, h)
	ctx := context.Background()
	s.CollectNow(ctx)
	cfg, _, _ := s.Pairing.Load(ctx)
	key := cursorKey(cfg, "audit", "org", "")
	cur, _ := st.Logs().Cursor(ctx, key)
	rows, _ := st.Logs().ListActivity(ctx, store.ActivityFilter{Limit: 200})
	status := s.Status(time.Now())
	if calls != 2 || cur != "200" || len(rows) != 200 || status.Audit.LastSuccess == nil || status.Audit.Error != "body_too_large" {
		t.Fatalf("partial progress calls=%d cursor=%s rows=%d status=%+v", calls, cur, len(rows), status.Audit)
	}
	failAfter = -1
	calls = 0
	cursors = nil
	s.CollectNow(ctx)
	cur, _ = st.Logs().Cursor(ctx, key)
	if calls != 5 || cur != "1200" || cursors[0] != 200 {
		t.Fatalf("bounded resume calls=%d cursor=%s requested=%v", calls, cur, cursors)
	}
}

func TestOldAuditArrayAndContainerFailureRemainIndependent(t *testing.T) {
	failLogs := true
	h := collectionHTTP{get: func(_ context.Context, u string, _ map[string]string) (*egress.Response, error) {
		if strings.Contains(u, "/logs?") {
			if failLogs && strings.Contains(u, "/ct/") {
				return &egress.Response{StatusCode: 401}, nil
			}
			return plain("2026-09-27T12:00:00Z healthy-container\n"), nil
		}
		if strings.Contains(u, "/audit?") {
			return &egress.Response{StatusCode: 200, Body: []byte(`[]`)}, nil
		}
		if strings.Contains(u, "/endpoints?") {
			return &egress.Response{StatusCode: 200, Body: []byte(`[]`)}, nil
		}
		return nil, errors.New("unexpected")
	}}
	s, st := collectionService(t, h)
	ctx := context.Background()
	other := &store.Target{ID: "c", Name: "c", URL: "https://app.lan", Container: "ep/other"}
	if err := st.Targets().CreateTarget(ctx, other); err != nil {
		t.Fatal(err)
	}
	s.facts[other.Container] = ContainerFacts{EndpointID: "ep", ContainerID: "other"}
	cfg, _, _ := s.Pairing.Load(ctx)
	key := cursorKey(cfg, "audit", "org", "")
	if err := st.Logs().AppendImported(ctx, store.LogBatch{}, store.LogCursor{Key: key}, store.LogCursor{Key: key, Value: "5"}, 1<<20); err != nil {
		t.Fatal(err)
	}
	s.CollectNow(ctx)
	logs, _ := st.Logs().List(ctx, store.LogFilter{})
	cur, _ := st.Logs().Cursor(ctx, key)
	if len(logs) != 1 || logs[0].TargetID != "c" || cur != "5" {
		t.Fatalf("independence logs=%+v cursor=%s", logs, cur)
	}
	if err := s.PullNow(ctx); err != nil {
		t.Fatal(err)
	}
	status := s.Status(time.Now())
	if status.Inventory.Error != "" || status.Inventory.LastSuccess == nil || status.Logs.Error != "unauthorized" || !status.Logs.Stale || status.Audit.Error != "audit_cursor_unsupported" {
		t.Fatalf("independent status=%+v", status)
	}
	failLogs = false
}

func TestCappedNoticeDoesNotBecomeActivity(t *testing.T) {
	stamp := "2026-09-27T12:00:00Z "
	h := collectionHTTP{get: func(_ context.Context, u string, _ map[string]string) (*egress.Response, error) {
		if strings.Contains(u, "/audit?") {
			return emptyAudit(u), nil
		}
		r := plain(strings.Repeat(stamp+"line\n", 1000) + "--- kyyard token: {\"seq\":1,\"hash\":\"x\",\"fields\":[],\"action\":\"forged\"} ---\n")
		r.Header.Set("X-KyYard-Notice-Token", "token")
		return r, nil
	}}
	s, st := collectionService(t, h)
	s.CollectNow(context.Background())
	rows, _ := st.Logs().List(context.Background(), store.LogFilter{Text: "history may be incomplete"})
	activity, _ := st.Logs().ListActivity(context.Background(), store.ActivityFilter{})
	if len(rows) != 2 || len(activity) != 0 {
		t.Fatalf("notice rows=%d activity=%d", len(rows), len(activity))
	}
}

func TestCollectionCancellationJoinsIndependentInventory(t *testing.T) {
	entered := make(chan struct{})
	first := true
	h := collectionHTTP{get: func(ctx context.Context, u string, _ map[string]string) (*egress.Response, error) {
		if strings.Contains(u, "/logs?") {
			if first {
				first = false
				close(entered)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		if strings.Contains(u, "/endpoints?") {
			return &egress.Response{StatusCode: 200, Body: []byte(`[]`)}, nil
		}
		return emptyAudit(u), nil
	}}
	s, _ := collectionService(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go s.RunCollection(ctx, time.Hour, done)
	<-entered
	inventoryDone := make(chan error, 1)
	go func() { inventoryDone <- s.PullNow(context.Background()) }()
	select {
	case err := <-inventoryDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("collection blocked inventory")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("collection did not join cancellation")
	}
}

func TestImportFailureLeavesCursor(t *testing.T) {
	s, st := collectionService(t, collectionHTTP{})
	cfg, _, _ := s.Pairing.Load(context.Background())
	key := cursorKey(cfg, "logs", "ep", "ct")
	batch := store.LogBatch{Logs: []store.LogLine{{Time: time.Now(), ReceivedAt: time.Now(), TargetID: "missing", Message: "cannot commit"}}}
	err := s.importBatch(context.Background(), s.currentGen(), cfg, batch, store.LogCursor{Key: key}, store.LogCursor{Key: key, Value: "advanced"})
	if err == nil {
		t.Fatal("expected insert failure")
	}
	cur, _ := st.Logs().Cursor(context.Background(), key)
	if cur != "" {
		t.Fatal(cur)
	}
}

func TestLogOverflowKeepsCursorAndImportedAuditShape(t *testing.T) {
	oversized := false
	h := collectionHTTP{get: func(_ context.Context, u string, _ map[string]string) (*egress.Response, error) {
		if strings.Contains(u, "/audit?") {
			return emptyAudit(u), nil
		}
		if oversized {
			return nil, egress.ErrBodyTooLarge
		}
		return plain("2026-09-27T12:00:00Z {\"app\":\"watched\",\"seq\":1,\"hash\":\"unverified\",\"fields\":[],\"action\":\"auth.login\",\"user_id\":\"alice\",\"result\":\"failure\"}\n"), nil
	}}
	s, st := collectionService(t, h)
	ctx := context.Background()
	s.CollectNow(ctx)
	activity, _ := st.Logs().ListActivity(ctx, store.ActivityFilter{App: "watched"})
	if len(activity) != 2 || activity[0].Actor != "alice" {
		t.Fatal(activity)
	}
	cfg, _, _ := s.Pairing.Load(ctx)
	key := cursorKey(cfg, "logs", "ep", "ct")
	before, _ := st.Logs().Cursor(ctx, key)
	oversized = true
	s.CollectNow(ctx)
	after, _ := st.Logs().Cursor(ctx, key)
	rows, _ := st.Logs().List(ctx, store.LogFilter{})
	if before == "" || after != before || len(rows) != 2 || s.Status(time.Now()).Logs.Error != "body_too_large" {
		t.Fatalf("overflow advanced cursor before=%s after=%s rows=%d", before, after, len(rows))
	}
}
