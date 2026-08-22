package interlocking

import (
	"errors"
	"testing"
	"time"

	"task138-railblock/internal/model"
	"task138-railblock/internal/routeplan"
)

func mkYard() *Yard {
	return &Yard{
		Switches:           map[string]*model.Switch{},
		Signals:            map[string]*model.Signal{},
		Sections:           map[string]*model.Section{},
		Routes:             routeplan.NewTable(),
		SectionOnceOccupied: map[string]map[string]bool{},
		Trains:             map[string]*model.Train{},
	}
}

// TestCheckEstablishRejectsOccupiedSection: a route over an occupied section
// fails with ErrSectionOccupied.
func TestCheckEstablishRejectsOccupiedSection(t *testing.T) {
	y := mkYard()
	y.Signals["X"] = &model.Signal{ID: "X"}
	y.Sections["S1"] = &model.Section{ID: "S1", Occupied: true}
	r := &model.Route{ID: "R", StationID: "ST", SourceSignalID: "X", Terminal: "T",
		Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}, State: model.RoutePending}
	_, err := CheckEstablish(y, r)
	if !errors.Is(err, ErrSectionOccupied) {
		t.Fatalf("want ErrSectionOccupied, got %v", err)
	}
}

// TestCheckEstablishRejectsConflictingRoute: a conflicting established route
// blocks establishment.
func TestCheckEstablishRejectsConflictingRoute(t *testing.T) {
	y := mkYard()
	y.Signals["X"] = &model.Signal{ID: "X"}
	y.Sections["S1"] = &model.Section{ID: "S1"}
	a := &model.Route{ID: "A", StationID: "ST", SourceSignalID: "X", Terminal: "T",
		Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}, State: model.RouteEstablished}
	b := &model.Route{ID: "B", StationID: "ST", SourceSignalID: "X", Terminal: "T",
		Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}, State: model.RoutePending}
	y.Routes.Add(a)
	y.Routes.Add(b)
	// Mark a as locked-by-route so the section is "locked by another".
	y.Sections["S1"].Locked = true
	y.Sections["S1"].LockedByRoute = "A"
	_, err := CheckEstablish(y, b)
	if !errors.Is(err, ErrConflictingRoute) {
		t.Fatalf("want ErrConflictingRoute, got %v", err)
	}
}

// TestCheckEstablishRejectsLostIndication.
func TestCheckEstablishRejectsLostIndication(t *testing.T) {
	y := mkYard()
	y.Signals["X"] = &model.Signal{ID: "X"}
	y.Sections["S1"] = &model.Section{ID: "S1"}
	y.Switches["W1"] = &model.Switch{ID: "W1", SectionID: "S1", HasIndication: false}
	r := &model.Route{ID: "R", StationID: "ST", SourceSignalID: "X", Terminal: "T",
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionNormal}},
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}}, State: model.RoutePending}
	_, err := CheckEstablish(y, r)
	if !errors.Is(err, ErrSwitchLostIndication) {
		t.Fatalf("want ErrSwitchLostIndication, got %v", err)
	}
}

// TestCheckEstablishPlansSwitchMove: a switch not at its required position but
// movable yields a SwitchMove.
func TestCheckEstablishPlansSwitchMove(t *testing.T) {
	y := mkYard()
	y.Signals["X"] = &model.Signal{ID: "X"}
	y.Sections["S1"] = &model.Section{ID: "S1"}
	y.Switches["W1"] = &model.Switch{ID: "W1", SectionID: "S1", HasIndication: true, CurrentPosition: model.PositionNormal}
	r := &model.Route{ID: "R", StationID: "ST", SourceSignalID: "X", Terminal: "T",
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionReverse}},
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}}, State: model.RoutePending}
	res, err := CheckEstablish(y, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.SwitchMoves) != 1 || res.SwitchMoves[0].To != model.PositionReverse {
		t.Fatalf("expected one reverse move, got %v", res.SwitchMoves)
	}
}

// TestCheckSwitchMoveRejectsOccupied.
func TestCheckSwitchMoveRejectsOccupied(t *testing.T) {
	y := mkYard()
	y.Sections["S1"] = &model.Section{ID: "S1", Occupied: true}
	y.Switches["W1"] = &model.Switch{ID: "W1", SectionID: "S1", HasIndication: true}
	if err := CheckSwitchMove(y, "W1", model.PositionReverse); !errors.Is(err, ErrSwitchInOccupiedSection) {
		t.Fatalf("want ErrSwitchInOccupiedSection, got %v", err)
	}
}

// TestCheckSwitchMoveRejectsLocked.
func TestCheckSwitchMoveRejectsLocked(t *testing.T) {
	y := mkYard()
	y.Sections["S1"] = &model.Section{ID: "S1"}
	y.Switches["W1"] = &model.Switch{ID: "W1", SectionID: "S1", HasIndication: true, Locked: true}
	if err := CheckSwitchMove(y, "W1", model.PositionReverse); !errors.Is(err, ErrSwitchLocked) {
		t.Fatalf("want ErrSwitchLocked, got %v", err)
	}
}

// TestDecideCancelImmediateForFreeApproach: established route with a free
// approach → immediate cancel.
func TestDecideCancelImmediateForFreeApproach(t *testing.T) {
	y := mkYard()
	y.Sections["AP"] = &model.Section{ID: "AP"}
	r := &model.Route{ID: "R", State: model.RouteEstablished, ApproachSectionID: "AP", Kind: model.RouteKindTrain}
	dec, err := DecideCancel(y, r)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !dec.Immediate || dec.NeedTimedDelay {
		t.Fatalf("want immediate, got %+v", dec)
	}
}

// TestDecideCancelTimedForOccupiedApproach.
func TestDecideCancelTimedForOccupiedApproach(t *testing.T) {
	y := mkYard()
	y.Sections["AP"] = &model.Section{ID: "AP", Occupied: true}
	r := &model.Route{ID: "R", State: model.RouteEstablished, ApproachSectionID: "AP", Kind: model.RouteKindTrain}
	dec, err := DecideCancel(y, r)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !dec.NeedTimedDelay || dec.Delay.Seconds() != 180 {
		t.Fatalf("want timed 180s, got %+v", dec)
	}
}

// TestDecideCancelRejectsCancelling.
func TestDecideCancelRejectsCancelling(t *testing.T) {
	y := mkYard()
	r := &model.Route{ID: "R", State: model.RouteCancelling}
	if _, err := DecideCancel(y, r); !errors.Is(err, ErrApproachLocked) {
		t.Fatalf("want ErrApproachLocked, got %v", err)
	}
}

// TestDecideCancelImmediateForNoApproach: an established route with no
// approach section cancels immediately (nothing can be approach-locked).
func TestDecideCancelImmediateForNoApproach(t *testing.T) {
	y := mkYard()
	r := &model.Route{ID: "R", State: model.RouteEstablished, Kind: model.RouteKindTrain}
	dec, err := DecideCancel(y, r)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !dec.Immediate || dec.NeedTimedDelay {
		t.Fatalf("want immediate, got %+v", dec)
	}
}

// TestDecideCancelTimedForApproachLocked: a route already in ApproachLocked
// (train in the approach section) cancels via the timed delay, never
// immediately — the regression this guards against.
func TestDecideCancelTimedForApproachLocked(t *testing.T) {
	y := mkYard()
	y.Sections["AP"] = &model.Section{ID: "AP", Occupied: true}
	r := &model.Route{ID: "R", State: model.RouteApproachLocked, ApproachSectionID: "AP", Kind: model.RouteKindShunt}
	dec, err := DecideCancel(y, r)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !dec.NeedTimedDelay || dec.Delay.Seconds() != 30 {
		t.Fatalf("want timed 30s, got %+v", dec)
	}
	if dec.Immediate {
		t.Fatal("approach-locked route must not cancel immediately")
	}
}

// TestOnApproachOccupied covers the Established → ApproachLocked and the
// Cancelling-abandon transitions.
func TestOnApproachOccupied(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RouteEstablished}
	if res := OnApproachOccupied(r); !res.TransitionsToApproachLocked {
		t.Fatal("established route should transition to approach-locked")
	}
	r2 := &model.Route{ID: "R", State: model.RouteCancelling}
	if res := OnApproachOccupied(r2); !res.AbandonsCancel {
		t.Fatal("cancelling route should abandon the cancel")
	}
}

// TestThreePointReleaseTerminalClear completes the route.
func TestThreePointReleaseTerminalClear(t *testing.T) {
	y := mkYard()
	y.Sections["S1"] = &model.Section{ID: "S1", Locked: true, LockedByRoute: "R"}
	y.Sections["S2"] = &model.Section{ID: "S2", Locked: true, LockedByRoute: "R"}
	y.SectionOnceOccupied["R"] = map[string]bool{"S1": true, "S2": true}
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}, {SectionID: "S2", Seq: 1}}}
	res := ThreePointRelease(r, y, "S2", time.Now(), false)
	if !res.RouteComplete {
		t.Fatal("terminal clear should complete the route")
	}
}

// TestThreePointReleaseTrailingSectionOnOccupy: entering s[2] releases s[0]
// when s[0] is clear and once-occupied.
func TestThreePointReleaseTrailingSectionOnOccupy(t *testing.T) {
	y := mkYard()
	y.Sections["S0"] = &model.Section{ID: "S0"}
	y.Sections["S1"] = &model.Section{ID: "S1"}
	y.Sections["S2"] = &model.Section{ID: "S2", Occupied: true}
	y.SectionOnceOccupied["R"] = map[string]bool{"S0": true, "S1": true}
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		Sections: []model.RouteSection{{SectionID: "S0", Seq: 0}, {SectionID: "S1", Seq: 1}, {SectionID: "S2", Seq: 2}}}
	res := ThreePointRelease(r, y, "S2", time.Now(), true)
	// Entering the last section: s[0] is clear + once-occupied → release.
	found := false
	for _, s := range res.SectionsToUnlock {
		if s == "S0" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected S0 in unlocks, got %v", res.SectionsToUnlock)
	}
}

// TestFaultUnlockTarget returns every lock held by the route.
func TestFaultUnlockTarget(t *testing.T) {
	y := mkYard()
	y.Sections["S1"] = &model.Section{ID: "S1", Locked: true, LockedByRoute: "R"}
	y.Switches["W1"] = &model.Switch{ID: "W1", Locked: true, LockedByRoute: "R"}
	r := &model.Route{ID: "R",
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}},
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionNormal}}}
	secs, sws := FaultUnlockTarget(y, r)
	if len(secs) != 1 || secs[0] != "S1" {
		t.Fatalf("sections = %v", secs)
	}
	if len(sws) != 1 || sws[0] != "W1" {
		t.Fatalf("switches = %v", sws)
	}
}
