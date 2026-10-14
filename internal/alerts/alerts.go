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

// Flapping is not a target state: it names the condition in notices about it. FlapAfter
// transitions within FlapWindow (three outages in an hour) make a target flap; it settles
// after StableAfter without a transition.
const (
	Flapping    State = "flapping"
	FlapAfter         = 6
	FlapWindow        = time.Hour
	StableAfter       = 15 * time.Minute
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
	// Announced is true once a webhook for the current problem was handed to the sender; only
	// then is the recovery sent too. Delivery is asynchronous, so it does not mean the receiver
	// got it: a recovery after a failed problem webhook is still sent, which costs the operator
	// one message and never leaves them believing an app is still down.
	Announced bool `json:"announced,omitempty"`
	// Changes are the recent transition times, at most FlapAfter within FlapWindow; Flapping
	// is set while they say the target flaps.
	Changes  []time.Time `json:"changes,omitempty"`
	Flapping bool        `json:"flapping,omitempty"`
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
	// Notice, when set, is a change of the flapping condition (From or To is Flapping). It is
	// recorded even when not sent, and it is what a send reports instead of the transition.
	Notice *Transition
}

// Decide says whether a webhook goes out now. A transition into a problem is sent once;
// while the problem persists, a reminder goes out every ReminderEvery. The recovery is sent
// only if the problem was announced. Silence stops the webhook only; the transition is still
// recorded and shown on screen, and it still stamps LastNotified, so once the silence lapses
// the hourly reminder resumes counting from the transition rather than never firing.
// FlapAfter transitions within FlapWindow switch to flapping (see flap).
func Decide(t Track, tr *Transition, now time.Time) (Decision, Track) {
	silenced := t.UntilFixed || now.Before(t.SilencedUntil)
	if tr != nil && tr.From != Pending {
		t.Changes = append(t.Changes, now)
		for len(t.Changes) > FlapAfter || now.Sub(t.Changes[0]) > FlapWindow {
			t.Changes = t.Changes[1:]
		}
	}
	if t.Flapping || (tr != nil && len(t.Changes) >= FlapAfter) {
		return flap(t, tr, now, silenced)
	}
	if tr != nil {
		if tr.To == OK {
			announced := t.Announced
			t.Announced, t.LastNotified = false, time.Time{}
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

// flap replaces per-transition webhooks while a target flaps: one notice on entry, a
// reminder every ReminderEvery, and one notice for the settled state once no transition has
// happened for StableAfter. Transitions are still recorded, just not sent.
func flap(t Track, tr *Transition, now time.Time, silenced bool) (Decision, Track) {
	switch {
	case !t.Flapping:
		t.Flapping, t.LastNotified = true, now
		return announce(t, &Transition{From: tr.From, To: Flapping, At: now, Cause: tr.Cause}, silenced)
	case tr != nil:
		return Decision{}, t
	case len(t.Changes) == 0 || now.Sub(t.Changes[len(t.Changes)-1]) >= StableAfter:
		t.Flapping, t.Changes = false, nil
		settled := &Transition{From: Flapping, To: t.State, At: now, Cause: t.Cause}
		if t.State == OK {
			announced := t.Announced
			t.Announced, t.LastNotified = false, time.Time{}
			return Decision{Send: announced, Notice: settled}, t
		}
		t.LastNotified = now
		return announce(t, settled, silenced)
	case !silenced && now.Sub(t.LastNotified) >= ReminderEvery:
		t.LastNotified, t.Announced = now, true
		return Decision{Send: true, Reminder: true, Notice: &Transition{From: Flapping, To: Flapping, At: now, Cause: t.Cause}}, t
	}
	return Decision{}, t
}

// announce sends notice unless silenced; the notice is recorded either way.
func announce(t Track, notice *Transition, silenced bool) (Decision, Track) {
	if silenced {
		return Decision{Notice: notice}, t
	}
	t.Announced = true
	return Decision{Send: true, Notice: notice}, t
}
