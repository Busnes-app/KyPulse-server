package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Busnes-app/ky-primitives/applysetup"
	"github.com/Busnes-app/kypulse-server/internal/config"
	"github.com/Busnes-app/kypulse-server/internal/store"
)

func monitoringBundle(t *testing.T) (config.InstallerBundle, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "source-token")
	token := strings.Repeat("ab", 32)
	if err := os.WriteFile(file, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	b := config.InstallerBundle{Common: applysetup.Common{Schema: applysetup.Schema, Service: "kypulse", SetupID: "271333e2-c221-4601-b249-563b3d828530"}, Monitoring: &config.InstallerMonitoring{Targets: []config.InstallerTarget{{Key: "identity", Name: "KyIdentity", URL: "https://identity.example/healthz"}}, Sources: []config.InstallerSource{{Key: "offboard", Name: "offboard", Target: "identity", TokenFile: file, OffboardingAlerts: true}}}}
	if err := b.PrepareMonitoring(&config.Config{}); err != nil {
		t.Fatal(err)
	}
	return b, token
}

func TestInstallerMonitoringOwnedCheckResumeAndDrift(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b, token := monitoringBundle(t)
	if _, err := st.ApplyInstaller(ctx, b, true); err == nil {
		t.Fatal("missing inspection accepted")
	}
	r, err := st.ApplyInstaller(ctx, b, false)
	if err != nil || len(r.Created) != 2 {
		t.Fatalf("create: %+v %v", r, err)
	}
	hash := sha256.Sum256([]byte(token))
	source, err := st.Sources().Authenticate(ctx, hex.EncodeToString(hash[:]))
	if err != nil || !source.OffboardingAlerts || source.TargetID == "" {
		t.Fatalf("native source: %+v %v", source, err)
	}
	before, err := st.Audit().VerifyChain(ctx)
	if err != nil || before.Count != 1 {
		t.Fatalf("atomic audit: %+v %v", before, err)
	}
	until := time.Now().Add(time.Hour)
	if err := st.Targets().SetSilence(ctx, source.TargetID, &until, true); err != nil {
		t.Fatal(err)
	}
	// Health and silence are not installer-owned fields.
	if err := st.Targets().RecordPoll(ctx, source.TargetID, store.PollUpdate{PolledAt: time.Now(), State: "down", StateSince: time.Now(), Cause: "fixture", TrackJSON: `{"state":"down"}`}); err != nil {
		t.Fatal(err)
	}
	for _, check := range []bool{true, false} {
		r, err = st.ApplyInstaller(ctx, b, check)
		if err != nil || len(r.Created) != 0 || len(r.Present) != 2 {
			t.Fatalf("resume/check: %+v %v", r, err)
		}
	}
	if after, err := st.Audit().VerifyChain(ctx); err != nil || after != before {
		t.Fatal("resume/check changed keyed audit", err)
	}
	got, err := st.Targets().GetTarget(ctx, source.TargetID)
	if err != nil || got.State != "down" || got.SilencedUntil == nil || !got.UntilFixed {
		t.Fatal("resume changed monitoring state", err)
	}
	serialized, _ := json.Marshal(b)
	report, _ := json.Marshal(r)
	settings, err := st.Settings().GetAllSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range append([]string{string(serialized), string(report)}, mapValues(settings)...) {
		if strings.Contains(value, token) {
			t.Fatal("source credential persisted or reported")
		}
	}
	// Revocation must remain permanent even when the token file is retained.
	if err := st.Sources().Revoke(ctx, source.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplyInstaller(ctx, b, false); err == nil {
		t.Fatal("revoked source was reactivated")
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func TestInstallerMonitoringForeignCollisionAtomicRollback(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	b, token := monitoringBundle(t)
	// A source collision occurs after the candidate target and identity would be created.
	b.Admins = []applysetup.Identity{{Issuer: "https://identity.example", Subject: "owner", Username: "owner-admin"}}
	hash := sha256.Sum256([]byte(token))
	if err := st.Sources().CreateCode(ctx, "foreign", "", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	foreign, err := st.Sources().Claim(ctx, "foreign", hex.EncodeToString(hash[:]), "retained-source")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplyInstaller(ctx, b, false); err == nil {
		t.Fatal("foreign source adopted")
	}
	if targets, err := st.Targets().ListTargets(ctx); err != nil || len(targets) != 0 {
		t.Fatal("partial target persisted", err)
	}
	if count, err := st.Users().CountUsers(ctx); err != nil || count != 0 {
		t.Fatal("partial administrator persisted", err)
	}
	if got, err := st.Sources().Authenticate(ctx, hex.EncodeToString(hash[:])); err != nil || got.ID != foreign.ID || got.OffboardingAlerts {
		t.Fatal("foreign source changed", err)
	}
	if chain, err := st.Audit().VerifyChain(ctx); err != nil || chain.Count != 0 {
		t.Fatal("partial setup audit persisted", err)
	}
	all, err := st.Settings().GetAllSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for key := range all {
		if strings.HasPrefix(key, "installer.") {
			t.Fatal("partial ownership persisted")
		}
	}
}

func TestInstallerMonitoringMissingOwnedObjectsAndChangedCredentialRefused(t *testing.T) {
	for _, mode := range []string{"target-name", "target-delete", "credential", "setup-id", "source-scope"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			st := newTestStore(t)
			b, token := monitoringBundle(t)
			if _, err := st.ApplyInstaller(ctx, b, false); err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256([]byte(token))
			source, err := st.Sources().Authenticate(ctx, hex.EncodeToString(hash[:]))
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "target-name":
				target, err := st.Targets().GetTarget(ctx, source.TargetID)
				if err != nil {
					t.Fatal(err)
				}
				target.Name = "Retained operator change"
				if err := st.Targets().UpdateTarget(ctx, target); err != nil {
					t.Fatal(err)
				}
			case "target-delete":
				if err := st.Targets().DeleteTarget(ctx, source.TargetID); err != nil {
					t.Fatal(err)
				}
			case "credential":
				if err := os.WriteFile(b.Monitoring.Sources[0].TokenFile, []byte(strings.Repeat("cd", 32)), 0600); err != nil {
					t.Fatal(err)
				}
				if err := b.PrepareMonitoring(&config.Config{}); err != nil {
					t.Fatal(err)
				}
			case "setup-id":
				b.SetupID = "371333e2-c221-4601-b249-563b3d828530"
			case "source-scope":
				b.Monitoring.Sources[0].OffboardingAlerts = false
			}
			if _, err := st.ApplyInstaller(ctx, b, false); err == nil {
				t.Fatal("owned drift was silently repaired")
			}
			if chain, err := st.Audit().VerifyChain(ctx); err != nil || chain.Count != 1 {
				t.Fatal("refusal changed audit", err)
			}
		})
	}
}
