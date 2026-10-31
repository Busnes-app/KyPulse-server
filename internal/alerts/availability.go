package alerts

import (
	"slices"
	"time"
)

// AvailabilityWindow is how far back Measure reads a target's recorded transitions.
const AvailabilityWindow = 24 * time.Hour

// Change is one recorded transition between settled states.
type Change struct {
	At       time.Time
	From, To State
}

// Availability is a target's last AvailabilityWindow, read from its transitions.
type Availability struct {
	// Uptime is the percentage of classified time the target was ok; degraded and down both
	// count against it. Nil when nothing in the window was classified.
	Uptime *float64
	// Incidents counts problems that began in the window: a move from ok or pending into
	// degraded or down. Degraded worsening to down is the same incident.
	Incidents int
}

// Measure reads the window ending at now, starting no earlier than the target's creation.
// The state before the first change is that change's From; with no change it is current.
// Reminders, flapping notices and offboarding alerts are not changes and are ignored.
func Measure(current State, created time.Time, changes []Change, now time.Time) Availability {
	start := now.Add(-AvailabilityWindow)
	if created.After(start) {
		start = created
	}
	var in []Change
	for _, c := range changes {
		if c.From != c.To && settled(c.From) && settled(c.To) && !c.At.Before(start) && !c.At.After(now) {
			in = append(in, c)
		}
	}
	slices.SortStableFunc(in, func(a, b Change) int { return a.At.Compare(b.At) })

	state := current
	if len(in) > 0 {
		state = in[0].From
	}
	var up, classified time.Duration
	at := start
	span := func(until time.Time) {
		if d := until.Sub(at); d > 0 && state != Pending {
			classified += d
			if state == OK {
				up += d
			}
		}
	}
	var out Availability
	for _, c := range in {
		span(c.At)
		if (c.From == OK || c.From == Pending) && (c.To == Degraded || c.To == Down) {
			out.Incidents++
		}
		at, state = c.At, c.To
	}
	span(now)
	if classified > 0 {
		u := 100 * float64(up) / float64(classified)
		out.Uptime = &u
	}
	return out
}

func settled(s State) bool { return s == Pending || s == OK || s == Degraded || s == Down }
