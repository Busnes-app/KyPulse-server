package kyyard

import "testing"

// TestClearIfCurrentSkipsAStaleGen is a white-box test of the guard PullNow's unpaired branch
// relies on: a Load that found no pairing must not un-adopt a pairing that landed (via a
// concurrent Clear+Adopt, which bumps gen) after this call's generation was captured. It is
// in package kyyard, not kyyard_test, because clearIfCurrent and the fields it touches are
// unexported and there is no other way to drive a stale-gen call without one.
func TestClearIfCurrentSkipsAStaleGen(t *testing.T) {
	s := &Service{}
	stale := s.currentGen() // captured before the race, the way PullNow captures gen

	// A concurrent re-pair: Clear bumps the generation, then Adopt (stood in for directly,
	// to keep this test to the one thing it is proving) lands a pairing under the new one.
	s.Clear()
	s.mu.Lock()
	s.paired, s.cfg = true, Config{OrganizationName: "A"}
	s.mu.Unlock()

	s.clearIfCurrent(stale)
	if !s.paired || s.cfg.OrganizationName != "A" {
		t.Fatalf("clearIfCurrent must not un-adopt a pairing landed under a newer generation: paired=%v cfg=%+v", s.paired, s.cfg)
	}

	// The current generation still clears -- the guard only refuses a stale one.
	s.clearIfCurrent(s.currentGen())
	if s.paired {
		t.Fatal("clearIfCurrent must clear when gen is still current")
	}
}
