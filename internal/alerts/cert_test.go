package alerts

import (
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/poller"
)

func TestDaysLeft(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		in   time.Duration
		want int
	}{{21*24*time.Hour + time.Minute, 21}, {21*24*time.Hour - time.Minute, 20}, {time.Minute, 0}, {-time.Minute, -1}, {-24 * time.Hour, -1}} {
		if got := DaysLeft(now.Add(tc.in), now); got != tc.want {
			t.Errorf("%v: got %d want %d", tc.in, got, tc.want)
		}
	}
}

func TestCertLevel(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for _, tc := range []struct {
		name  string
		cert  poller.Cert
		level State
		cause string
	}{
		{"fine", poller.Cert{NotAfter: now.Add(60 * day), NameMatch: true}, OK, ""},
		{"warning at 21", poller.Cert{NotAfter: now.Add(21*day + time.Hour), NameMatch: true}, CertWarning, "cert_expiring"},
		{"not yet warning at 22", poller.Cert{NotAfter: now.Add(22*day + time.Hour), NameMatch: true}, OK, ""},
		{"critical at 7", poller.Cert{NotAfter: now.Add(7*day + time.Hour), NameMatch: true}, CertCritical, "cert_expiring"},
		{"expired", poller.Cert{NotAfter: now.Add(-time.Second), NameMatch: true}, CertFailed, "cert_expired"},
		{"expired outranks untrusted", poller.Cert{NotAfter: now, Problem: "cert_untrusted"}, CertFailed, "cert_expired"},
		{"untrusted", poller.Cert{NotAfter: now.Add(60 * day), NameMatch: true, Problem: "cert_untrusted"}, CertFailed, "cert_untrusted"},
		{"name mismatch", poller.Cert{NotAfter: now.Add(60 * day)}, CertFailed, "cert_name_mismatch"},
	} {
		level, cause := CertLevel(&tc.cert, now, DefaultCertWarnDays, DefaultCertCriticalDays)
		if level != tc.level || cause != tc.cause {
			t.Errorf("%s: got %s %s", tc.name, level, cause)
		}
	}
	// The administrator's thresholds apply.
	if level, _ := CertLevel(&poller.Cert{NotAfter: now.Add(25 * day), NameMatch: true}, now, 30, 10); level != CertWarning {
		t.Errorf("custom warn: got %s", level)
	}
}

func TestNextCertNeedsTwoPollsToChange(t *testing.T) {
	now := time.Now()
	tr, change := NextCert(Track{}, OK, "", now)
	if change == nil || tr.State != OK || change.From != Pending {
		t.Fatalf("fresh track settles on its first poll: %+v %+v", tr, change)
	}
	tr, change = NextCert(tr, CertWarning, "cert_expiring", now)
	if change != nil || tr.State != OK {
		t.Fatalf("one warning poll changed the level: %+v", tr)
	}
	tr, change = NextCert(tr, OK, "", now)
	if change != nil || tr.Candidate != "" {
		t.Fatalf("an ok poll did not reset the candidate: %+v", tr)
	}
	tr, _ = NextCert(tr, CertWarning, "cert_expiring", now)
	tr, change = NextCert(tr, CertWarning, "cert_expiring", now)
	if change == nil || change.To != CertWarning || tr.State != CertWarning || tr.Cause != "cert_expiring" {
		t.Fatalf("two warning polls: %+v %+v", tr, change)
	}
}

func TestCertTrackRemindsHourlyAndHonoursSilence(t *testing.T) {
	now := time.Now()
	tr, change := NextCert(Track{}, CertCritical, "cert_expiring", now)
	d, tr := Decide(tr, change, now)
	if !d.Send {
		t.Fatal("a fresh critical certificate was not announced")
	}
	if d, _ := Decide(tr, nil, now.Add(30*time.Minute)); d.Send {
		t.Fatal("reminder before the hour")
	}
	d, tr = Decide(tr, nil, now.Add(ReminderEvery))
	if !d.Send || !d.Reminder {
		t.Fatal("no hourly reminder")
	}
	tr.SilencedUntil = now.Add(3 * time.Hour)
	if d, _ := Decide(tr, nil, now.Add(2*ReminderEvery)); d.Send {
		t.Fatal("reminder while silenced")
	}
}

func TestCertRecoveryEndsUntilFixed(t *testing.T) {
	now := time.Now()
	tr := Track{State: CertWarning, Since: now, UntilFixed: true}
	tr, _ = NextCert(tr, OK, "", now)
	tr, change := NextCert(tr, OK, "", now)
	if change == nil || tr.UntilFixed {
		t.Fatalf("recovery kept until-fixed: %+v", tr)
	}
}
