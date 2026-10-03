package kyyard_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
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
	if err := svc.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	return svc, &now
}

// yard: a live Docker endpoint (ep_1), an offline one (ep_2), an approved one with no inventory
// yet (ep_3, 404), a pending and a revoked one (404, never read), and a Kubernetes one.
func yard() *fakeHTTP {
	return answers(map[string]struct {
		code int
		body any
	}{
		"/api/organizations/org_a/endpoints?limit=200": {200, []map[string]any{
			{"id": "ep_1", "name": "host-1", "runtime": "docker", "state": "active"},
			{"id": "ep_2", "name": "host-2", "runtime": "docker", "state": "offline"},
			{"id": "ep_3", "name": "host-3", "runtime": "docker", "state": "approved"},
			{"id": "ep_p", "name": "host-p", "runtime": "docker", "state": "pending"},
			{"id": "ep_r", "name": "host-r", "runtime": "docker", "state": "revoked"},
			{"id": "ep_k", "name": "k8s", "runtime": "kubernetes", "state": "active"},
		}},
		"/api/organizations/org_a/endpoints/ep_1/inventory": {200, map[string]any{"endpoint_id": "ep_1", "state": "active", "observed_at": "2026-09-27T09:59:50Z", "received_at": "2026-09-27T09:59:51Z",
			"snapshot": map[string]any{"containers": []map[string]any{{"id": "c1", "name": "kyvault", "image": "ghcr.io/busnes-app/kyvault:1.2", "state": "running", "status": "Up 3 hours (healthy)"}}}}},
		"/api/organizations/org_a/endpoints/ep_1/samples": {200, []map[string]any{{"container_id": "c1", "observed_at": "2026-09-27T09:59:50Z", "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 2}}},
		"/api/organizations/org_a/endpoints/ep_2/inventory": {200, map[string]any{"endpoint_id": "ep_2", "state": "offline", "observed_at": "2026-09-27T09:59:50Z", "received_at": "2026-09-27T09:59:51Z",
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
		for _, ep := range []string{"ep_k/", "ep_p/", "ep_r/"} {
			if containsStr(g.url, ep) {
				t.Fatalf("kubernetes, pending and revoked endpoints must not be pulled: %s", g.url)
			}
		}
	}
	f, ok := svc.Facts("ep_1/kyvault", *now)
	if !ok || f.Health != "healthy" || !f.HasSample || f.SampleAt == nil || !f.SampleAt.Equal(time.Date(2026, 9, 27, 9, 59, 50, 0, time.UTC)) ||
		f.MemoryBytes != 1000 || f.MemoryLimit != 4000 || f.RestartCount != 2 || f.EndpointName != "host-1" || f.EndpointOffline || f.Stale {
		t.Fatalf("facts: %+v %v", f, ok)
	}
	g, ok := svc.Facts("ep_2/kyvault", *now)
	if !ok || g.ExitCode == nil || *g.ExitCode != 137 || g.State != "exited" || g.HasSample || g.SampleAt != nil || !g.EndpointOffline || !g.Stale {
		t.Fatalf("offline endpoint's facts: %+v %v", g, ok)
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

func TestAnEndpointFailingWithAnythingButA404FailsThePull(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	h.answers["/api/organizations/org_a/endpoints/ep_3/inventory"] = struct {
		code int
		body any
	}{500, nil}
	if err := svc.PullNow(context.Background()); err == nil {
		t.Fatal("a 500 on one endpoint must fail the pull")
	}
	if st := svc.Status(*now); st.Error != "status_500" || st.FetchedAt != nil {
		t.Fatalf("status: %+v", st)
	}
}

func TestOpenAdoptsTheStoredPairingBeforeAnyPull(t *testing.T) {
	h := yard()
	h.err = context.DeadlineExceeded
	svc, now := pairedService(t, h) // Open ran; no pull yet
	want := func(when string) {
		t.Helper()
		st := svc.Status(*now)
		if !st.Paired || st.URL != "https://yard.lan" || st.Organization != "A" || st.FetchedAt != nil || !st.Stale {
			t.Fatalf("%s: %+v", when, st)
		}
	}
	want("before any pull")
	if err := svc.PullNow(context.Background()); err == nil {
		t.Fatal("pull must fail")
	}
	want("after a failing first pull")
	if st := svc.Status(*now); st.Error != "timeout" {
		t.Fatalf("reason: %+v", st)
	}
}

func TestPullRecoversPairingAfterStartupReadFailure(t *testing.T) {
	h := yard()
	h.err = context.DeadlineExceeded
	svc, now := pairedService(t, h)
	svc.Clear()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.Open(ctx); err == nil {
		t.Fatal("startup read must fail")
	}
	if err := svc.PullNow(context.Background()); err == nil {
		t.Fatal("upstream pull must still fail")
	}
	if st := svc.Status(*now); !st.Paired || st.URL != "https://yard.lan" || st.Organization != "A" || st.Error != "timeout" || st.FetchedAt != nil || !st.Stale {
		t.Fatalf("readable pairing must be visible even while upstream is down: %+v", st)
	}
}

func TestAllMissingEndpointDataFailsPullWithoutReplacingSnapshot(t *testing.T) {
	for _, route := range []string{"inventory", "samples"} {
		t.Run(route, func(t *testing.T) {
			h := yard()
			svc, now := pairedService(t, h)
			if err := svc.PullNow(context.Background()); err != nil {
				t.Fatal(err)
			}
			last := svc.Status(*now).FetchedAt
			for _, ep := range []string{"ep_1", "ep_2"} {
				delete(h.answers, "/api/organizations/org_a/endpoints/"+ep+"/"+route)
			}
			*now = now.Add(time.Minute)
			if err := svc.PullNow(context.Background()); kyyard.Reason(err) != "status_404" {
				t.Fatalf("missing endpoint data: %v", err)
			}
			if st := svc.Status(*now); st.Error != "status_404" || st.FetchedAt == nil || !st.FetchedAt.Equal(*last) {
				t.Fatalf("all-404 pull must not advance last success: %+v", st)
			}
			if _, ok := svc.Facts("ep_1/kyvault", *now); !ok {
				t.Fatal("last successful snapshot must survive")
			}
		})
	}
}

func TestNoEligibleEndpointsIsSuccessfulEmptyInventory(t *testing.T) {
	h := answers(map[string]struct {
		code int
		body any
	}{"/api/organizations/org_a/endpoints?limit=200": {200, []map[string]any{
		{"id": "ep_p", "runtime": "docker", "state": "pending"},
		{"id": "ep_k", "runtime": "kubernetes", "state": "active"},
	}}})
	svc, now := pairedService(t, h)
	if err := svc.PullNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := svc.Status(*now); st.FetchedAt == nil || st.Error != "" || st.Stale {
		t.Fatalf("no eligible endpoints is not an upstream failure: %+v", st)
	}
}

func TestUnreadablePairingIsLogged(t *testing.T) {
	svc, _ := pairedService(t, yard())
	if err := svc.Pairing.Settings.SetSetting(context.Background(), "kyyard_enc", "not-sealed"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	svc.Logger, _ = logging.New(logging.Config{App: "kypulse", Out: &buf})
	if err := svc.PullNow(context.Background()); !errors.Is(err, kyyard.ErrUnreadable) {
		t.Fatalf("err: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "kyyard_pull_failed") || !strings.Contains(out, "unreadable") {
		t.Fatalf("log: %s", out)
	}
}

func TestRestartsInTheLastHourAndStaleness(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	_ = svc.PullNow(context.Background())
	if f, _ := svc.Facts("ep_1/kyvault", *now); f.HistoryMinutes != 0 {
		t.Fatalf("first pull covers no span: %+v", f)
	}
	*now = now.Add(30 * time.Minute)
	h.answers["/api/organizations/org_a/endpoints/ep_1/samples"] = struct {
		code int
		body any
	}{200, []map[string]any{{"container_id": "c1", "observed_at": now.Format(time.RFC3339), "memory_bytes": 1000, "memory_limit": 4000, "restart_count": 5}}}
	_ = svc.PullNow(context.Background())
	f, _ := svc.Facts("ep_1/kyvault", *now)
	if f.RestartsLastHour != 3 || f.HistoryMinutes != 30 {
		t.Fatalf("restarts in the last hour: %+v", f)
	}
	*now = now.Add(45 * time.Minute) // the first sample (restart_count 2) is older than an hour now
	_ = svc.PullNow(context.Background())
	f, _ = svc.Facts("ep_1/kyvault", *now)
	if f.RestartsLastHour != 0 || f.HistoryMinutes != 45 {
		t.Fatalf("history not trimmed: %+v", f)
	}
	if f, _ := svc.Facts("ep_1/kyvault", now.Add(2*time.Hour)); f.HistoryMinutes != 60 {
		t.Fatalf("history span caps at 60: %+v", f)
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

func TestRevokedTokenReportsUnauthorizedAndStale(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	if err := svc.PullNow(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.answers["/api/organizations/org_a/endpoints?limit=200"] = struct {
		code int
		body any
	}{401, map[string]string{"error": "Authentication required"}}
	_ = svc.PullNow(context.Background())
	if st := svc.Status(*now); st.Error != "unauthorized" || !st.Stale {
		t.Fatalf("a refused token is stale at once: %+v", st)
	}
	if f, ok := svc.Facts("ep_1/kyvault", *now); !ok || !f.Stale {
		t.Fatalf("facts under a refused token are stale: %+v %v", f, ok)
	}
}

func TestUnpairClearsAndDisappearedContainerIsGone(t *testing.T) {
	h := yard()
	svc, now := pairedService(t, h)
	_ = svc.PullNow(context.Background())
	h.answers["/api/organizations/org_a/endpoints/ep_1/inventory"] = struct {
		code int
		body any
	}{200, map[string]any{"endpoint_id": "ep_1", "state": "active", "observed_at": now.Format(time.RFC3339), "received_at": now.Format(time.RFC3339), "snapshot": map[string]any{"containers": []map[string]any{}}}}
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
	for h.getCount() < 12 { // two full pulls
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
	for h.getCount() < 6 { // Run's own immediate first pull: endpoints, 2x ep_1, 2x ep_2, ep_3
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
