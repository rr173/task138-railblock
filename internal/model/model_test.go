package model

import "testing"

// TestSwitchPositionOpposite checks the two-state toggle.
func TestSwitchPositionOpposite(t *testing.T) {
	if PositionNormal.Opposite() != PositionReverse {
		t.Fatal("normal opposite should be reverse")
	}
	if PositionReverse.Opposite() != PositionNormal {
		t.Fatal("reverse opposite should be normal")
	}
}

// TestSwitchPositionString checks the display string.
func TestSwitchPositionString(t *testing.T) {
	if PositionNormal.String() != "normal" {
		t.Fatalf("normal string = %s", PositionNormal.String())
	}
	if PositionReverse.String() != "reverse" {
		t.Fatalf("reverse string = %s", PositionReverse.String())
	}
}

// TestRouteStateIsLocked covers the lock-holding states.
func TestRouteStateIsLocked(t *testing.T) {
	locked := []RouteState{RouteEstablished, RouteApproachLocked, RouteCancelling}
	for _, s := range locked {
		if !s.IsLocked() {
			t.Fatalf("%s should be locked", s)
		}
	}
	notLocked := []RouteState{RoutePending, RouteRequested, RouteUnlocked, RouteCancelled, RouteFailed}
	for _, s := range notLocked {
		if s.IsLocked() {
			t.Fatalf("%s should not be locked", s)
		}
	}
}

// TestRouteStateIsTerminal covers the terminal states.
func TestRouteStateIsTerminal(t *testing.T) {
	terminal := []RouteState{RouteUnlocked, RouteCancelled, RouteFailed}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Fatalf("%s should be terminal", s)
		}
	}
	if RouteEstablished.IsTerminal() {
		t.Fatal("established is not terminal")
	}
}

// TestRouteKindApproachLockDelay checks the per-kind timed-cancel delay.
func TestRouteKindApproachLockDelay(t *testing.T) {
	if got := RouteKindTrain.ApproachLockDelay(); got.Seconds() != 180 {
		t.Fatalf("train delay = %v, want 180s", got)
	}
	if got := RouteKindShunt.ApproachLockDelay(); got.Seconds() != 30 {
		t.Fatalf("shunt delay = %v, want 30s", got)
	}
}

// TestRouteSectionIDsAndHasSection covers the route section accessors.
func TestRouteSectionIDsAndHasSection(t *testing.T) {
	r := &Route{Sections: []RouteSection{{SectionID: "S1", Seq: 0}, {SectionID: "S2", Seq: 1}}}
	ids := r.SectionIDs()
	if len(ids) != 2 || ids[0] != "S1" || ids[1] != "S2" {
		t.Fatalf("SectionIDs = %v", ids)
	}
	if !r.HasSection("S1") || !r.HasSection("S2") || r.HasSection("S9") {
		t.Fatal("HasSection mismatch")
	}
}

// TestRouteRequiredPositionOf covers switch-position lookup.
func TestRouteRequiredPositionOf(t *testing.T) {
	r := &Route{SwitchPositions: []RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: PositionReverse}}}
	pos, ok := r.RequiredPositionOf("W1")
	if !ok || pos != PositionReverse {
		t.Fatalf("RequiredPositionOf W1 = %v,%v", pos, ok)
	}
	if _, ok := r.RequiredPositionOf("W9"); ok {
		t.Fatal("unknown switch should report not-ok")
	}
}
