package trainops

import (
	"testing"
	"time"

	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/model"
	"task138-railblock/internal/routeplan"
)

func mkYard2() *interlocking.Yard {
	return &interlocking.Yard{
		Switches:           map[string]*model.Switch{},
		Signals:            map[string]*model.Signal{},
		Sections:           map[string]*model.Section{},
		Routes:             routeplan.NewTable(),
		SectionOnceOccupied: map[string]map[string]bool{},
		Trains:             map[string]*model.Train{},
	}
}

// TestApplyMoveApproachLock: occupying a route's approach section sets the
// ApproachLockRoute effect.
func TestApplyMoveApproachLock(t *testing.T) {
	y := mkYard2()
	y.Sections["AP"] = &model.Section{ID: "AP"}
	r := &model.Route{ID: "R", State: model.RouteEstablished, ApproachSectionID: "AP"}
	y.Routes.Add(r)
	eff := ApplyMove(y, Move{TrainID: "T", SectionID: "AP", Kind: MoveOccupy, TS: time.Now()})
	if eff.ApproachLockRoute != "R" {
		t.Fatalf("ApproachLockRoute = %s, want R", eff.ApproachLockRoute)
	}
	if !eff.NowOccupied {
		t.Fatal("NowOccupied should be true for an occupy move")
	}
	if eff.AbandonCancelRoute != "" {
		t.Fatalf("AbandonCancelRoute = %s, want empty for an established route", eff.AbandonCancelRoute)
	}
}

// TestApplyMoveApproachOccupyAbortsCancelling: occupying the approach section
// of a route already in Cancelling (timed release running) must set
// AbandonCancelRoute so the train's arrival abandons the timed cancel rather
// than letting it silently release behind a moving train.
func TestApplyMoveApproachOccupyAbortsCancelling(t *testing.T) {
	y := mkYard2()
	y.Sections["AP"] = &model.Section{ID: "AP"}
	r := &model.Route{ID: "R", State: model.RouteCancelling, ApproachSectionID: "AP"}
	y.Routes.Add(r)
	eff := ApplyMove(y, Move{TrainID: "T", SectionID: "AP", Kind: MoveOccupy, TS: time.Now()})
	if eff.AbandonCancelRoute != "R" {
		t.Fatalf("AbandonCancelRoute = %s, want R", eff.AbandonCancelRoute)
	}
	if eff.ApproachLockRoute != "" {
		t.Fatalf("ApproachLockRoute = %s, want empty for a cancelling route", eff.ApproachLockRoute)
	}
}

// TestApplyMoveApproachOccupyNoOpOnInactive: occupying the approach section of
// a pending route has no lock effect (nothing to lock or abandon).
func TestApplyMoveApproachOccupyNoOpOnInactive(t *testing.T) {
	y := mkYard2()
	y.Sections["AP"] = &model.Section{ID: "AP"}
	r := &model.Route{ID: "R", State: model.RoutePending, ApproachSectionID: "AP"}
	y.Routes.Add(r)
	eff := ApplyMove(y, Move{TrainID: "T", SectionID: "AP", Kind: MoveOccupy, TS: time.Now()})
	if eff.ApproachLockRoute != "" || eff.AbandonCancelRoute != "" {
		t.Fatalf("pending route approach occupy should be a no-op, got %+v", eff)
	}
}

// TestApplyMoveOccupyRouteSection: occupying a route section marks
// once-occupied and closes the signal.
func TestApplyMoveOccupyRouteSection(t *testing.T) {
	y := mkYard2()
	y.Sections["S1"] = &model.Section{ID: "S1"}
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	y.Routes.Add(r)
	eff := ApplyMove(y, Move{TrainID: "T", SectionID: "S1", Kind: MoveOccupy, TS: time.Now()})
	if eff.SectionOnceOccupiedForRoute != "R" {
		t.Fatalf("once-occupied route = %s, want R", eff.SectionOnceOccupiedForRoute)
	}
	if len(eff.SignalCloseRoutes) != 1 || eff.SignalCloseRoutes[0] != "R" {
		t.Fatalf("SignalCloseRoutes = %v", eff.SignalCloseRoutes)
	}
}

// TestApplyMoveClearTerminal: clearing the terminal (once-occupied) route
// section completes the route.
func TestApplyMoveClearTerminal(t *testing.T) {
	y := mkYard2()
	y.Sections["S1"] = &model.Section{ID: "S1"}
	y.Sections["S2"] = &model.Section{ID: "S2"}
	y.SectionOnceOccupied["R"] = map[string]bool{"S1": true, "S2": true}
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}, {SectionID: "S2", Seq: 1}}}
	y.Routes.Add(r)
	eff := ApplyMove(y, Move{TrainID: "T", SectionID: "S2", Kind: MoveClear, TS: time.Now()})
	if eff.RouteComplete != "R" {
		t.Fatalf("RouteComplete = %s, want R", eff.RouteComplete)
	}
}

// TestApplyMoveNoEffectOnInactiveRoute: a move on a pending route's section has
// no lock effect.
func TestApplyMoveNoEffectOnInactiveRoute(t *testing.T) {
	y := mkYard2()
	y.Sections["S1"] = &model.Section{ID: "S1"}
	r := &model.Route{ID: "R", State: model.RoutePending,
		Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	y.Routes.Add(r)
	eff := ApplyMove(y, Move{TrainID: "T", SectionID: "S1", Kind: MoveOccupy, TS: time.Now()})
	if eff.SectionOnceOccupiedForRoute != "" {
		t.Fatalf("pending route should not set once-occupied, got %s", eff.SectionOnceOccupiedForRoute)
	}
	if len(eff.SignalCloseRoutes) != 0 {
		t.Fatalf("pending route should not close signals, got %v", eff.SignalCloseRoutes)
	}
}

// TestApplyMoveUnknownSectionNoOp: a move on a section not in the yard is a
// no-op apart from recording the section id.
func TestApplyMoveUnknownSectionNoOp(t *testing.T) {
	y := mkYard2()
	eff := ApplyMove(y, Move{TrainID: "T", SectionID: "ZZ", Kind: MoveOccupy, TS: time.Now()})
	if eff.SectionID != "ZZ" {
		t.Fatalf("SectionID = %s, want ZZ", eff.SectionID)
	}
	if eff.RouteComplete != "" || len(eff.SectionsToUnlock) != 0 {
		t.Fatalf("expected no-op, got %+v", eff)
	}
}
