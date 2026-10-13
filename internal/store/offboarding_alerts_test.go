package store

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOffboardingAlertAtomicReceiptAndSourceScope(t *testing.T) {
	s := newLogTestStore(t)
	ctx := context.Background()
	target := &Target{ID: "bridge", Name: "Offboarding bridge", URL: "https://bridge.example/healthz", Enabled: true}
	if err := s.Targets().CreateTarget(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := s.Sources().CreateOffboardingCode(ctx, "missing", "", time.Now().Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unbound policy: %v", err)
	}
	if err := s.Sources().CreateOffboardingCode(ctx, "code", target.ID, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	source, err := s.Sources().Claim(ctx, "code", "token", "bridge")
	if err != nil || !source.OffboardingAlerts {
		t.Fatalf("claim: %+v %v", source, err)
	}
	if got, err := s.Sources().Authenticate(ctx, "token"); err != nil || !got.OffboardingAlerts {
		t.Fatalf("persisted policy: %+v %v", got, err)
	}
	now := time.Now().UTC()
	batch := func(outcome string) LogBatch {
		line := testLine(now, "attempt")
		line.App, line.TargetID = "kyoffboard", "forged-target"
		return LogBatch{Logs: []LogLine{line}, Activity: []Activity{{Time: now, ReceivedAt: now, App: "kyoffboard", Actor: "kyoffboard", TargetID: "forged-target", Target: "bookstack:" + strings.Repeat("ab", 32), Action: "offboarding." + outcome, Outcome: outcome}}}
	}
	r := LogReceipt{1, strings.Repeat("ab", 32)}
	if err := s.Logs().Append(ctx, source.ID, batch("failed"), 1<<20); !errors.Is(err, ErrLogSequence) {
		t.Fatalf("unsequenced alert: %v", err)
	}
	bad := batch("failed")
	bad.Activity[0].Actor = "spoofed"
	if err := s.Logs().AppendSequenced(ctx, source.ID, bad, r, 1<<20); !errors.Is(err, ErrOffboardingAudit) {
		t.Fatalf("invalid projection: %v", err)
	}
	bad = batch("failed")
	bad.Logs = append(bad.Logs, bad.Logs[0])
	if err := s.Logs().AppendSequenced(ctx, source.ID, bad, r, 1<<20); !errors.Is(err, ErrOffboardingAudit) {
		t.Fatalf("multi-row batch: %v", err)
	}
	// Force an insert failure after the receipt and log inserts, and prove rollback.
	trigger := `CREATE TRIGGER reject_offboarding_alert BEFORE INSERT ON target_events WHEN NEW.cause = 'offboarding_failed' BEGIN SELECT RAISE(ABORT, 'fixture rejected alert'); END;`
	drop := "DROP TRIGGER reject_offboarding_alert"
	if s.driver == "postgres" {
		trigger = `CREATE FUNCTION reject_offboarding_alert() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'fixture rejected alert'; END$$;
CREATE TRIGGER reject_offboarding_alert BEFORE INSERT ON target_events FOR EACH ROW EXECUTE FUNCTION reject_offboarding_alert();`
		drop += " ON target_events; DROP FUNCTION reject_offboarding_alert()"
	}
	if _, err := s.db.ExecContext(ctx, trigger); err != nil {
		t.Fatal(err)
	}
	before := usage(t, s)
	if err := s.Logs().AppendSequenced(ctx, source.ID, batch("failed"), r, 1<<20); err == nil {
		t.Fatal("failed alert insert accepted")
	}
	if usage(t, s) != before {
		t.Fatal("failed alert changed usage")
	}
	if _, err := s.db.ExecContext(ctx, drop); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.Logs().AppendSequenced(ctx, source.ID, batch("failed"), r, 1<<20) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	events, total, err := s.Targets().ListEvents(ctx, "", 0, 10)
	if err != nil || total != 1 || events[0].TargetID != target.ID || events[0].Cause != "offboarding_failed" || events[0].Notified || events[0].NotifyError != "pending" {
		t.Fatalf("alerts: %+v total=%d %v", events, total, err)
	}
	if got, err := s.Targets().GetTarget(ctx, target.ID); err != nil || got.State != "pending" || got.LastPolledAt != nil {
		t.Fatalf("alert changed health: %+v %v", got, err)
	}
	// Pruning collected payloads does not lose or duplicate the alert receipt.
	if err := s.Logs().Prune(ctx, now, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Logs().AppendSequenced(ctx, source.ID, batch("failed"), r, 1<<20); err != nil {
		t.Fatal(err)
	}
	for i, outcome := range []string{"started", "success"} {
		if err := s.Logs().AppendSequenced(ctx, source.ID, batch(outcome), LogReceipt{int64(i + 2), r.Digest}, 1<<20); err != nil {
			t.Fatal(err)
		}
	}
	if _, total, err := s.Targets().ListEvents(ctx, "", 0, 10); err != nil || total != 1 {
		t.Fatalf("duplicate/nonfailure alert: %d %v", total, err)
	}
	// Ordinary paired senders cannot opt themselves into privileged alert production.
	if err := s.Sources().CreateCode(ctx, "ordinary", target.ID, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ordinary, err := s.Sources().Claim(ctx, "ordinary", "ordinary-token", "ordinary")
	if err != nil || ordinary.OffboardingAlerts {
		t.Fatal("ordinary source gained alert policy", err)
	}
	if err := s.Logs().AppendSequenced(ctx, ordinary.ID, batch("failed"), r, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, total, err := s.Targets().ListEvents(ctx, "", 0, 10); err != nil || total != 1 {
		t.Fatal("ordinary source created alert", err)
	}
	if err := s.Targets().DeleteTarget(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Logs().AppendSequenced(ctx, source.ID, batch("failed"), LogReceipt{4, r.Digest}, 1<<20); !errors.Is(err, ErrLogAlertScope) {
		t.Fatalf("deleted target silently accepted: %v", err)
	}
}
