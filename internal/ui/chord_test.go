package ui

import "testing"

// The gg chord was delivered to the hosts table by name rather than to whatever
// was holding the keyboard. G was never routed that way, so the pair the help
// popup advertises together as "first / last" agreed with each other on exactly
// one panel and silently disagreed everywhere else.
//
// Found by a demo driver, not by reading: the manage tour pressed gg to walk
// back to the top of the nav, the nav did not move, and the three keys meant
// for Hosts landed on Changes instead — where they read as menu, Clear changes,
// confirm. The recording emptied its own fixture and filmed the empty panel.
func TestGGGoesWhereTheKeyboardIs(t *testing.T) {
	m := appWith(sample(), nil)

	m = pressA(m, "1", "G")
	if m.pref.item != prefChanges {
		t.Fatalf("G on the nav should land on the last section, got %v", m.pref.item)
	}

	m = pressA(m, "g", "g")
	if m.pref.item != prefHosts {
		t.Errorf("gg on the nav should return to the first section, got %v", m.pref.item)
	}
}
