package alerts

import (
	"time"

	"github.com/Busnes-app/kypulse-server/internal/poller"
)

// Certificate levels. A certificate track uses OK and Pending like a health track, so Decide
// treats it the same way: one message per change, hourly reminders, silences.
const (
	CertWarning  State = "warning"
	CertCritical State = "critical"
	CertFailed   State = "failed"
)

// Default thresholds in days left, and how many consecutive polls a level change needs.
const (
	DefaultCertWarnDays     = 21
	DefaultCertCriticalDays = 7
	CertAfter               = 2
)

// DaysLeft is whole days until notAfter, rounded down; negative once expired.
func DaysLeft(notAfter, now time.Time) int {
	d := notAfter.Sub(now)
	days := int(d / (24 * time.Hour))
	if d < 0 && d%(24*time.Hour) != 0 {
		days--
	}
	return days
}

// CertLevel classifies one certificate, worst first: expired, a refused handshake (name or
// chain), then the critical and warning thresholds.
func CertLevel(c *poller.Cert, now time.Time, warnDays, criticalDays int) (State, string) {
	switch {
	case !now.Before(c.NotAfter):
		return CertFailed, "cert_expired"
	case c.Problem != "":
		return CertFailed, c.Problem
	case !c.NameMatch:
		return CertFailed, "cert_name_mismatch"
	}
	switch days := DaysLeft(c.NotAfter, now); {
	case days <= criticalDays:
		return CertCritical, "cert_expiring"
	case days <= warnDays:
		return CertWarning, "cert_expiring"
	}
	return OK, ""
}

// NextCert feeds one certificate level in. A change needs CertAfter consecutive polls at
// the new level (one from a fresh track), so one odd answer during a proxy reload does not
// alert. A recovery to OK ends an "until fixed" silence, as a health recovery does.
func NextCert(t Track, level State, cause string, now time.Time) (Track, *Transition) {
	if t.State == "" {
		t.State, t.Since = Pending, now
	}
	if level == t.State {
		t.Candidate, t.CandidateStreak = "", 0
		return t, nil
	}
	if level == t.Candidate {
		t.CandidateStreak++
	} else {
		t.Candidate, t.CandidateStreak = level, 1
	}
	if t.State != Pending && t.CandidateStreak < CertAfter {
		return t, nil
	}
	tr := &Transition{From: t.State, To: level, At: now, Cause: cause}
	t.State, t.Since, t.Cause, t.Candidate, t.CandidateStreak = level, now, cause, "", 0
	if level == OK && t.UntilFixed && tr.From != Pending {
		t.UntilFixed, t.SilencedUntil = false, time.Time{}
	}
	return t, tr
}
