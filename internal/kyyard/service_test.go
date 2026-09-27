package kyyard_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/logging"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/kyyard"
	"github.com/Busnes-app/kypulse-server/internal/store"
	"github.com/Busnes-app/kypulse-server/internal/testdb"
)

func pairedService(t *testing.T, h *fakeHTTP) (*kyyard.Service, *time.Time) {
	t.Helper()
	cfg := &config.Config{Database: testdb.Config(t)}
	cfg.Security.EncryptionKey = make([]byte, 32)
	st, err := store.Open(context.Background(), cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	p, _ := kyyard.NewPairing(cfg, st.Settings())
	if err := p.Save(context.Background(), kyyard.Config{URL: "https://yard.lan", Token: "tok", OrganizationID: "org_a", OrganizationName: "A"}); err != nil {
		t.Fatal(err)
	}
	lg, _ := logging.New(logging.Config{App: "kypulse", Out: io.Discard})
	now := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	svc := &kyyard.Service{Pairing: p, HTTP: h, Logger: lg, Now: func() time.Time { return now }}
	return svc, &now
}

func yard() *fakeHTTP {
	return answers(map[string]struct {
		code int
		body any
	}{
		"/api/organizations/org_a/endpoints?limit=200": {200, []map[string]any{{"id": "ep_1", "name": "host-1", "runtime": "docker", "state": "active"}, {"id": "ep_2", "name": "host-2", "runtime": "docker", "state": "active"}, {"id": "ep_k", "name": "k8s", "runtime": "kubernetes", "state": "active"}}},
		"/api/organizations/org_a/endpoints/ep_1/inventory": {200, map[string]any{"endpoint_id": "ep_1", "state": "complete", "observed_at": "2026-09-27T09:59:50Z", "received_at": "2026-09-27T09:59:51Z",
			"snapshot": map[string]any{"containers": []map[string]any{{"id": "c1", "name": "kyvault", "image": "ghcr.io/busnes-app/kyvault:1.2", "state": "running", "status": "Up 3 hours (healthy)"}}}}},
		"/api/organizations/org_a/endpoints/ep_1/samples": {200, []map[string]any{{"container_id": "c1", "observed_at": "2026-09-27T09:59:50Z", "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 2}}},
		"/api/organizations/org_a/endpoints/ep_2/inventory": {200, map[string]any{"endpoint_id": "ep_2", "state": "complete", "observed_at": "2026-09-27T09:59:50Z", "received_at": "2026-09-27T09:59:51Z",
			"snapshot": map[string]any{"containers": []map[string]any{{"id": "c9", "name": "kyvault", "image": "ghcr.io/busnes-app/kyvault:1.1", "state": "exited", "status": "Exited (137) 2 hours ago"}}}}},
		"/api/organizations/org_a/endpoints/ep_2/samples": {200, []map[string]any{}},
	})
}

func TestPullBuildsFactsPerEndpointAndSkipsKubernetes(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	if err := svc.PullNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, g := range h.gets {
		if len(g.url) >= 4 && g.url[len(g.url)-4:] == "ep_k" || containsStr(g.url, "ep_k/") {
			t.Fatalf("kubernetes endpoint must not be pulled: %s", g.url)
		}
	}
	f, ok := svc.Facts("ep_1/kyvault", *now)
	if !ok || f.Health != "healthy" || f.MemoryBytes != 1000 || f.MemoryLimit != 4000 || f.RestartCount != 2 || f.EndpointName != "host-1" || f.Stale {
		t.Fatalf("facts: %+v %v", f, ok)
	}
	g, ok := svc.Facts("ep_2/kyvault", *now)
	if !ok || g.ExitCode == nil || *g.ExitCode != 137 || g.State != "exited" {
		t.Fatalf("second endpoint's facts: %+v %v", g, ok)
	}
	if _, ok := svc.Facts("ep_1/nothere", *now); ok {
		t.Fatal("unknown link must not resolve")
	}
	sug := svc.Suggestions()
	if len(sug) != 2 || sug[0].Link != "ep_1/kyvault" || sug[0].EndpointName != "host-1" || sug[1].Link != "ep_2/kyvault" {
		t.Fatalf("suggestions: %+v", sug)
	}
	st := svc.Status(*now)
	if !st.Paired || st.Stale || st.Organization != "A" || st.URL != "https://yard.lan" || st.Error != "" || st.FetchedAt == nil {
		t.Fatalf("status: %+v", st)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRestartsInTheLastHourAndStaleness(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	_ = svc.PullNow(context.Background())
	*now = now.Add(30 * time.Minute)
	h.answers["/api/organizations/org_a/endpoints/ep_1/samples"] = struct {
		code int
		body any
	}{200, []map[string]any{{"container_id": "c1", "observed_at": now.Format(time.RFC3339), "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 5}}}
	_ = svc.PullNow(context.Background())
	f, _ := svc.Facts("ep_1/kyvault", *now)
	if f.RestartsLastHour != 3 {
		t.Fatalf("restarts in the last hour: %+v", f)
	}
	*now = now.Add(45 * time.Minute) // the first sample (restart_count 2) is older than an hour now
	_ = svc.PullNow(context.Background())
	f, _ = svc.Facts("ep_1/kyvault", *now)
	if f.RestartsLastHour != 0 {
		t.Fatalf("history not trimmed: %+v", f)
	}
	// A failing pull keeps the last snapshot, records the reason, and goes stale after 3 min.
	h.err = context.DeadlineExceeded
	*now = now.Add(time.Minute)
	if err := svc.PullNow(context.Background()); err == nil {
		t.Fatal("failed pull must report")
	}
	st := svc.Status(*now)
	if st.Error != "timeout" || st.Stale {
		t.Fatalf("one failure is not yet stale: %+v", st)
	}
	f, ok := svc.Facts("ep_1/kyvault", *now)
	if !ok || f.Stale {
		t.Fatalf("last snapshot must survive one failure: %+v %v", f, ok)
	}
	*now = now.Add(3 * time.Minute)
	if st := svc.Status(*now); !st.Stale {
		t.Fatalf("must be stale after StaleAfter: %+v", st)
	}
	if f, _ := svc.Facts("ep_1/kyvault", *now); !f.Stale {
		t.Fatal("facts must carry stale")
	}
}

func TestRevokedTokenReportsUnauthorized(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	h.answers["/api/organizations/org_a/endpoints?limit=200"] = struct {
		code int
		body any
	}{401, map[string]string{"error": "Authentication required"}}
	_ = svc.PullNow(context.Background())
	if st := svc.Status(*now); st.Error != "unauthorized" {
		t.Fatalf("status: %+v", st)
	}
}

func TestUnpairClearsAndDisappearedContainerIsGone(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	_ = svc.PullNow(context.Background())
	h.answers["/api/organizations/org_a/endpoints/ep_1/inventory"] = struct {
		code int
		body any
	}{200, map[string]any{"endpoint_id": "ep_1", "state": "complete", "observed_at": now.Format(time.RFC3339), "received_at": now.Format(time.RFC3339), "snapshot": map[string]any{"containers": []map[string]any{}}}}
	_ = svc.PullNow(context.Background())
	if _, ok := svc.Facts("ep_1/kyvault", *now); ok {
		t.Fatal("a container missing from the latest inventory must not resolve")
	}
	if err := svc.Pairing.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc.Clear()
	if st := svc.Status(*now); st.Paired || st.FetchedAt != nil {
		t.Fatalf("after unpair: %+v", st)
	}
	if err := svc.PullNow(context.Background()); err != nil {
		t.Fatalf("an unpaired pull is a no-op: %v", err)
	}
	if len(svc.Suggestions()) != 0 {
		t.Fatal("suggestions must be empty when unpaired")
	}
}

func TestClearDuringPullDoesNotResurrect(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	h.beforeAnswer = func() {
		if err := svc.Pairing.Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		svc.Clear()
	}
	if err := svc.PullNow(context.Background()); err != nil {
		t.Fatalf("a pull racing an unpair is not itself an error: %v", err)
	}
	if st := svc.Status(*now); st.Paired {
		t.Fatalf("unpair mid-pull must not be resurrected: %+v", st)
	}
	if _, ok := svc.Facts("ep_1/kyvault", *now); ok {
		t.Fatal("facts must stay empty after an unpair mid-pull")
	}
	if len(svc.Suggestions()) != 0 {
		t.Fatal("suggestions must stay empty after an unpair mid-pull")
	}

	// The failure path: the same race, but every request after the hook also fails.
	h2 := yard()
	svc2, now2 := pairedService(t, h2)
	h2.beforeAnswer = func() {
		if err := svc2.Pairing.Delete(context.Background()); err != nil {
			t.Fatal(err)
		}
		svc2.Clear()
		h2.err = context.DeadlineExceeded
	}
	_ = svc2.PullNow(context.Background())
	if st := svc2.Status(*now2); st.Paired || st.Error != "" {
		t.Fatalf("unpair mid-pull-that-then-fails must stay unpaired with no error: %+v", st)
	}
}

func TestRunPullsOnScheduleAndStops(t *testing.T) {
	h := yard()
	svc, _ := pairedService(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go svc.Run(ctx, 10*time.Millisecond, done)
	deadline := time.After(2 * time.Second)
	for h.getCount() < 6 {
		select {
		case <-deadline:
			t.Fatalf("only %d requests", h.getCount())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}

// TestKickWakesTheLoop proves Kick nudges Run to pull immediately instead of waiting for its
// ticker: the ticker here is an hour, so any pull after the first must have come from Kick.
func TestKickWakesTheLoop(t *testing.T) {
	h := yard()
	svc, _ := pairedService(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go svc.Run(ctx, time.Hour, done)

	deadline := time.After(2 * time.Second)
	for h.getCount() < 5 { // Run's own immediate first pull
		select {
		case <-deadline:
			t.Fatalf("first pull did not happen: %d requests", h.getCount())
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	before := h.getCount()

	svc.Kick()
	deadline = time.After(time.Second)
	for h.getCount() <= before {
		select {
		case <-deadline:
			t.Fatalf("Kick did not wake a pull within a second: got %d requests, want more than %d", h.getCount(), before)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}
