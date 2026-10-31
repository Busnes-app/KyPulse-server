package alerts

import (
	"testing"
	"time"
)

func TestMeasure(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	ago := func(h float64) time.Time { return now.Add(-time.Duration(h * float64(time.Hour))) }
	old := ago(48)

	cases := []struct {
		name      string
		current   State
		created   time.Time
		changes   []Change
		uptime    float64 // -1: nil
		incidents int
	}{
		{"no changes, ok all day", OK, old, nil, 100, 0},
		{"no changes, never classified", Pending, ago(1), nil, -1, 0},
		{"down for the last 6 of 24 hours", Down, old, []Change{{ago(6), OK, Down}}, 75, 1},
		{"degraded counts against uptime; worsening is one incident", Down, old,
			[]Change{{ago(12), OK, Degraded}, {ago(6), Degraded, Down}}, 50, 1},
		{"pending time is not counted", OK, ago(2),
			[]Change{{ago(1.5), Pending, Down}, {ago(1), Down, OK}}, 66.66666666666667, 1},
		{"changes before the window set nothing; reminders and notices are ignored", OK, old,
			[]Change{{ago(30), OK, Down}, {ago(3), Down, Down}, {ago(2), OK, Flapping}}, 100, 0},
		{"two outages", OK, old,
			[]Change{{ago(10), OK, Down}, {ago(8), Down, OK}, {ago(4), OK, Down}, {ago(2), Down, OK}}, 100 * 20.0 / 24, 2},
	}
	for _, c := range cases {
		got := Measure(c.current, c.created, c.changes, now)
		if c.uptime < 0 {
			if got.Uptime != nil {
				t.Errorf("%s: uptime %v, want nil", c.name, *got.Uptime)
			}
		} else if got.Uptime == nil || *got.Uptime < c.uptime-1e-9 || *got.Uptime > c.uptime+1e-9 {
			t.Errorf("%s: uptime %v, want %v", c.name, got.Uptime, c.uptime)
		}
		if got.Incidents != c.incidents {
			t.Errorf("%s: incidents %d, want %d", c.name, got.Incidents, c.incidents)
		}
	}
}
