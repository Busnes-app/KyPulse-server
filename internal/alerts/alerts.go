// Package alerts is the per-target state machine: a single bad poll changes nothing, a run of
// them does, and every change is one notification. It holds no I/O; the caller persists the
// Track it returns and sends what Decide tells it to.
package alerts

import (
	"time"

	"github.com/Busnes-app/kypulse-server/internal/poller"
)

// State is a target's settled state. Pending is a target that has not yet been classified.
type State = poller.State

const (
	Pending  State = "pending"
	OK             = poller.OK
	Degraded       = poller.Degraded
	Down           = poller.Down
)

// Thresholds, in consecutive polls. At the default 30 s interval, down takes 90 s to declare.
const (
	DownAfter     = 3
	DegradedAfter = 2
	RecoverAfter  = 2
	ReminderEvery = time.Hour
)

// Track is everything the machine needs between polls. It is stored on the target row.
type Track struct {
	State          State     `json:"state"`
	Since          time.Time `json:"since"`
	Cause          string    `json:"cause,omitempty"`
	OKStreak       int       `json:"ok_streak"`
	DegradedStreak int       `json:"degraded_streak"`
	DownStreak     int       `json:"down_streak"`
	// LastNotified is when the last webhook for the current problem went out; zero when the
	// target is fine. Reminders are counted from it.
	LastNotified time.Time `json:"last_notified,omitempty"`
	// Announced is true once a webhook for the current problem was sent; only then is the
	// recovery sent too.
	Announced bool `json:"announced,omitempty"`
	// SilencedUntil suppresses webhooks until then; UntilFixed silences until the next OK.
	SilencedUntil time.Time `json:"silenced_until,omitempty"`
	UntilFixed    bool      `json:"until_fixed,omitempty"`
}

// Transition is one state change, the unit the Alerts tab lists.
type Transition struct {
	From  State     `json:"from"`
	To    State     `json:"to"`
	At    time.Time `json:"at"`
	Cause string    `json:"cause,omitempty"`
}

// Next feeds one observation in. It returns the updated track and the transition, if any.
func Next(t Track, obs poller.Result, now time.Time) (Track, *Transition) {
	if t.State == "" {
		t.State, t.Since = Pending, now
	}
	switch obs.State {
	case OK:
		t.OKStreak, t.DegradedStreak, t.DownStreak = t.OKStreak+1, 0, 0
	case Degraded:
		t.OKStreak, t.DegradedStreak, t.DownStreak = 0, t.DegradedStreak+1, 0
	case Down:
		t.OKStreak, t.DegradedStreak, t.DownStreak = 0, 0, t.DownStreak+1
	}
	next := t.State
	switch {
	case t.State == Pending && t.OKStreak >= 1:
		next = OK
	case t.State != OK && t.OKStreak >= RecoverAfter:
		next = OK
	case t.State != Down && t.DownStreak >= DownAfter:
		next = Down
	case t.State != Degraded && t.State != Down && t.DegradedStreak >= DegradedAfter:
		next = Degraded
	case t.State == Down && t.DegradedStreak >= DegradedAfter:
		next = Degraded
	}
	if next == t.State {
		return t, nil
	}
	tr := &Transition{From: t.State, To: next, At: now, Cause: obs.Cause}
	t.State, t.Since, t.Cause = next, now, obs.Cause
	if next == OK {
		t.LastNotified = time.Time{}
		// "Until fixed" is over once the app recovers from a problem. A fresh target's first
		// OK is not a recovery, so a silence set before it was ever classified holds.
		if t.UntilFixed && tr.From != Pending {
			t.UntilFixed, t.SilencedUntil = false, time.Time{}
		}
	}
	return t, tr
}

// Decision is what the caller should send, if anything.
type Decision struct {
	Send     bool
	Reminder bool // a repeat for a problem already announced
}

// Decide says whether a webhook goes out now. A transition into a problem is sent once;
// while the problem persists, a reminder goes out every ReminderEvery. The recovery is sent
// only if the problem was announced. Silence stops the webhook only; the transition is still
// recorded and shown on screen, and it still stamps LastNotified, so once the silence lapses
// the hourly reminder resumes counting from the transition rather than never firing.
func Decide(t Track, tr *Transition, now time.Time) (Decision, Track) {
	silenced := t.UntilFixed || now.Before(t.SilencedUntil)
	if tr != nil {
		if tr.To == OK {
			announced := t.Announced
			t.Announced = false
			return Decision{Send: announced}, t
		}
		t.LastNotified = now // reminders count from here, silenced or not
		if silenced {
			return Decision{}, t
		}
		t.Announced = true
		return Decision{Send: true}, t
	}
	if t.State == OK || t.State == Pending || silenced {
		return Decision{}, t
	}
	if !t.LastNotified.IsZero() && now.Sub(t.LastNotified) >= ReminderEvery {
		t.LastNotified, t.Announced = now, true
		return Decision{Send: true, Reminder: true}, t
	}
	return Decision{}, t
}
