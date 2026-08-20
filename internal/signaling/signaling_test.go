package signaling

import (
	"testing"

	"task138-railblock/internal/model"
)

func mkState(r *model.Route, sws map[string]*model.Switch, secs map[string]*model.Section) State {
	return State{Route: r, Switches: sws, Sections: secs}
}

// TestComputeAspectNoRouteIsRed: no route → red.
func TestComputeAspectNoRouteIsRed(t *testing.T) {
	got := ComputeAspect(State{})
	if got != model.AspectRed {
		t.Fatalf("no route: got %s want R", got)
	}
}

// TestComputeAspectNonEstablishedStateIsRed.
func TestComputeAspectNonEstablishedStateIsRed(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RoutePending}
	if got := ComputeAspect(mkState(r, nil, nil)); got != model.AspectRed {
		t.Fatalf("pending route: got %s want R", got)
	}
}

// TestComputeAspectLostIndicationIsRed.
func TestComputeAspectLostIndicationIsRed(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionNormal}},
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	sws := map[string]*model.Switch{"W1": {ID: "W1", HasIndication: false, CurrentPosition: model.PositionNormal}}
	secs := map[string]*model.Section{"S1": {ID: "S1", Occupied: false}}
	if got := ComputeAspect(mkState(r, sws, secs)); got != model.AspectRed {
		t.Fatalf("lost indication: got %s want R", got)
	}
}

// TestComputeAspectWrongSwitchPositionIsRed.
func TestComputeAspectWrongSwitchPositionIsRed(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionReverse}},
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	sws := map[string]*model.Switch{"W1": {ID: "W1", HasIndication: true, CurrentPosition: model.PositionNormal}}
	secs := map[string]*model.Section{"S1": {ID: "S1", Occupied: false}}
	if got := ComputeAspect(mkState(r, sws, secs)); got != model.AspectRed {
		t.Fatalf("wrong switch pos: got %s want R", got)
	}
}

// TestComputeAspectOccupiedRouteSectionIsRed: a train inside the route forces
// red; the approach section may be occupied without forcing red.
func TestComputeAspectOccupiedRouteSectionIsRed(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionNormal}},
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}, {SectionID: "S2", Seq: 1}},
		ApproachSectionID: "AP"}
	sws := map[string]*model.Switch{"W1": {ID: "W1", HasIndication: true, CurrentPosition: model.PositionNormal}}
	secs := map[string]*model.Section{
		"S1": {ID: "S1", Occupied: true},
		"S2": {ID: "S2", Occupied: false},
		"AP": {ID: "AP", Occupied: true},
	}
	if got := ComputeAspect(mkState(r, sws, secs)); got != model.AspectRed {
		t.Fatalf("occupied route section: got %s want R", got)
	}
}

// TestComputeAspectStraightRouteIsGreen: straight established route → green.
func TestComputeAspectStraightRouteIsGreen(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionNormal}},
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}},
	}
	sws := map[string]*model.Switch{"W1": {ID: "W1", HasIndication: true, CurrentPosition: model.PositionNormal}}
	secs := map[string]*model.Section{"S1": {ID: "S1", Occupied: false}}
	if got := ComputeAspect(mkState(r, sws, secs)); got != model.AspectGreen {
		t.Fatalf("straight route: got %s want G", got)
	}
}

// TestComputeAspectDivergingRouteIsDoubleYellow: reverse switch → YY.
func TestComputeAspectDivergingRouteIsDoubleYellow(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionReverse}},
		Sections:        []model.RouteSection{{SectionID: "S1", Seq: 0}},
	}
	sws := map[string]*model.Switch{"W1": {ID: "W1", HasIndication: true, CurrentPosition: model.PositionReverse}}
	secs := map[string]*model.Section{"S1": {ID: "S1", Occupied: false}}
	if got := ComputeAspect(mkState(r, sws, secs)); got != model.AspectDoubleYellow {
		t.Fatalf("diverging route: got %s want YY", got)
	}
}

// TestAspectForOpen matches ComputeAspect on a freshly established route.
func TestAspectForOpen(t *testing.T) {
	r := &model.Route{ID: "R", State: model.RouteEstablished,
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionNormal}}}
	sws := map[string]*model.Switch{"W1": {ID: "W1", HasIndication: true, CurrentPosition: model.PositionNormal}}
	if got := AspectForOpen(r, sws); got != model.AspectGreen {
		t.Fatalf("AspectForOpen: got %s want G", got)
	}
}
