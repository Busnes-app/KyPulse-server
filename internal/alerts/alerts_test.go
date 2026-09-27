package alerts

import (
	"testing"
	"time"

	"github.com/Busnes-app/kypulse-server/internal/poller"
)

var t0 = time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)

// run feeds a sequence of states 30 s apart and returns the transitions and final track.
func run(t Track, seq ...poller.State) (Track, []Transition) {
	var out []Transition
	now := t0
	for _, s := range seq {
		var tr *Transition
		t, tr = Next(t, poller.Result{State: s, Cause: "refused"}, now)
		if tr != nil {
			out = append(out, *tr)
		}
		now = now.Add(30 * time.Second)
	}
	return t, out
}

func states(trs []Transition) []State {
	var out []State
	for _, tr := range trs {
		out = append(out, tr.To)
	}
	return out
}

func equal(a, b []State) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTransitions(t *testing.T) {
	cases := []struct {
		name string
		seq  []poller.State
		want []State
	}{
		{"fresh target comes up on the first ok", []poller.State{OK}, []State{OK}},
		{"one failure changes nothing", []poller.State{OK, Down, OK}, []State{OK}},
		{"two failures change nothing", []poller.State{OK, Down, Down, OK}, []State{OK}},
		{"three failures are down", []poller.State{OK, Down, Down, Down}, []State{OK, Down}},
		{"fresh target that never answers is down after three", []poller.State{Down, Down, Down}, []State{Down}},
		{"one good poll does not recover", []poller.State{OK, Down, Down, Down, OK, Down}, []State{OK, Down}},
		{"two good polls recover", []poller.State{OK, Down, Down, Down, OK, OK}, []State{OK, Down, OK}},
		{"one degraded changes nothing", []poller.State{OK, Degraded, OK}, []State{OK}},
		{"two degraded are degraded", []poller.State{OK, Degraded, Degraded}, []State{OK, Degraded}},
		{"degraded then down", []poller.State{OK, Degraded, Degraded, Down, Down, Down}, []State{OK, Degraded, Down}},
		{"down then degraded", []poller.State{Down, Down, Down, Degraded, Degraded}, []State{Down, Degraded}},
		{"mixed bad polls never reach a threshold", []poller.State{OK, Down, Degraded, Down, Degraded, Down}, []State{OK}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, trs := run(Track{}, tc.seq...)
			if got := states(trs); !equal(got, tc.want) {
				t.Fatalf("transitions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTransitionCarriesCauseAndTime(t *testing.T) {
	tr, trs := run(Track{}, OK, Down, Down, Down)
	if len(trs) != 2 || trs[1].Cause != "refused" || trs[1].From != OK || !trs[1].At.Equal(t0.Add(90*time.Second)) {
		t.Fatalf("transitions = %+v", trs)
	}
	if tr.Cause != "refused" || !tr.Since.Equal(t0.Add(90*time.Second)) {
		t.Fatalf("track = %+v", tr)
	}
}

func TestDecideSendsOnChangeAndRemindsHourly(t *testing.T) {
	tr := Track{}
	now := t0
	feed := func(s poller.State) Decision {
		var trans *Transition
		tr, trans = Next(tr, poller.Result{State: s}, now)
		var d Decision
		d, tr = Decide(tr, trans, now)
		now = now.Add(30 * time.Second)
		return d
	}
	if d := feed(OK); d.Send {
		t.Fatal("a fresh target coming up must not notify")
	}
	feed(Down)
	feed(Down)
	if d := feed(Down); !d.Send || d.Reminder {
		t.Fatalf("third failure: %+v", d)
	}
	if d := feed(Down); d.Send {
		t.Fatalf("still down a moment later must not resend: %+v", d)
	}
	now = now.Add(ReminderEvery)
	if d := feed(Down); !d.Send || !d.Reminder {
		t.Fatalf("an hour later: %+v", d)
	}
	if d := feed(Down); d.Send {
		t.Fatal("reminder must not repeat every poll")
	}
	feed(OK)
	if d := feed(OK); !d.Send || d.Reminder {
		t.Fatalf("recovery: %+v", d)
	}
	if !tr.LastNotified.IsZero() || tr.Announced {
		t.Fatal("recovery must clear LastNotified and Announced")
	}
}

func TestSilenceStopsWebhooksNotTransitions(t *testing.T) {
	tr := Track{SilencedUntil: t0.Add(8 * time.Hour)}
	now := t0
	var last *Transition
	for _, s := range []poller.State{OK, Down, Down, Down} {
		tr, last = Next(tr, poller.Result{State: s}, now)
		now = now.Add(30 * time.Second)
	}
	if last == nil || last.To != Down {
		t.Fatalf("transition must still be recorded under silence: %+v", last)
	}
	d, tr := Decide(tr, last, now)
	if d.Send {
		t.Fatal("silenced target must not send")
	}
	// Silence expires: the hourly reminder resumes from the transition.
	now = t0.Add(9 * time.Hour)
	d, _ = Decide(tr, nil, now)
	if !d.Send || !d.Reminder {
		t.Fatal("silence lapsed with the app still down: the reminder resumes")
	}
}

func TestReminderResumesAfterASilenceLapses(t *testing.T) {
	tr := Track{SilencedUntil: t0.Add(30 * time.Minute)}
	now := t0
	var trans *Transition
	for _, s := range []poller.State{OK, Down, Down} {
		tr, _ = Next(tr, poller.Result{State: s}, now)
		now = now.Add(30 * time.Second)
	}
	// The third Down is the transition; Decide is called with the same 'now', so
	// LastNotified and the transition timestamp agree exactly.
	tr, trans = Next(tr, poller.Result{State: Down}, now)
	d, tr := Decide(tr, trans, now)
	if d.Send {
		t.Fatal("silenced at the transition: must not send")
	}
	transitionAt := now
	at31 := transitionAt.Add(31 * time.Minute)
	if d, _ := Decide(tr, nil, at31); d.Send {
		t.Fatalf("31 min: silence is over but the hourly reminder is not due yet: %+v", d)
	}
	at60 := transitionAt.Add(60 * time.Minute)
	if d, _ := Decide(tr, nil, at60); !d.Send || !d.Reminder {
		t.Fatalf("60 min: the reminder should fire: %+v", d)
	}
}

func TestSilenceUntilFixedEndsOnRecovery(t *testing.T) {
	tr := Track{UntilFixed: true}
	tr, _ = run(tr, OK, Down, Down, Down)
	if !tr.UntilFixed {
		t.Fatal("still down: silence must hold")
	}
	tr, trs := run(tr, OK, OK)
	if len(trs) != 1 || trs[0].To != OK {
		t.Fatalf("expected recovery, got %+v", trs)
	}
	if tr.UntilFixed || !tr.SilencedUntil.IsZero() {
		t.Fatalf("recovery must clear the silence: %+v", tr)
	}
	d, _ := Decide(tr, &trs[0], t0)
	if d.Send {
		t.Fatal("the operator never heard about the problem, so there is nothing to close")
	}
}

func TestRecoveryIsNotSentForAnUnannouncedProblem(t *testing.T) {
	tr := Track{SilencedUntil: t0.Add(8 * time.Hour)}
	now := t0
	for _, s := range []poller.State{OK, Down, Down, Down, OK, OK} {
		var trans *Transition
		var d Decision
		tr, trans = Next(tr, poller.Result{State: s}, now)
		d, tr = Decide(tr, trans, now)
		if d.Send {
			t.Fatalf("%s at %s sent under a silence covering the whole incident", s, now)
		}
		now = now.Add(30 * time.Second)
	}
	if tr.State != OK || tr.Announced {
		t.Fatalf("track: %+v", tr)
	}
}
