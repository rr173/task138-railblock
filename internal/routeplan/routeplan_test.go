package routeplan

import (
	"testing"

	"task138-railblock/internal/model"
)

// TestConflictsBySharedSection asserts two routes sharing a section conflict.
func TestConflictsBySharedSection(t *testing.T) {
	a := &model.Route{ID: "A", Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	b := &model.Route{ID: "B", Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	if !Conflicts(a, b) {
		t.Fatal("routes sharing S1 should conflict")
	}
}

// TestConflictsByOppositeSwitchPosition asserts two routes requiring the same
// switch in opposite positions conflict even without a shared section.
func TestConflictsByOppositeSwitchPosition(t *testing.T) {
	a := &model.Route{ID: "A",
		Sections:        []model.RouteSection{{SectionID: "SA", Seq: 0}},
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionNormal}},
	}
	b := &model.Route{ID: "B",
		Sections:        []model.RouteSection{{SectionID: "SB", Seq: 0}},
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "W1", RequiredPosition: model.PositionReverse}},
	}
	if !Conflicts(a, b) {
		t.Fatal("routes requiring W1 in opposite positions should conflict")
	}
}

// TestConflictsNoneDistinctSections asserts non-overlapping, non-conflicting
// routes do not conflict.
func TestConflictsNoneDistinctSections(t *testing.T) {
	a := &model.Route{ID: "A", Sections: []model.RouteSection{{SectionID: "SA", Seq: 0}}}
	b := &model.Route{ID: "B", Sections: []model.RouteSection{{SectionID: "SB", Seq: 0}}}
	if Conflicts(a, b) {
		t.Fatal("distinct-section routes should not conflict")
	}
}

// TestConflictsNilOrSameRoute guards the irreflexive / nil cases.
func TestConflictsNilOrSameRoute(t *testing.T) {
	a := &model.Route{ID: "A", Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	if Conflicts(a, a) {
		t.Fatal("a route must not conflict with itself")
	}
	if Conflicts(nil, a) || Conflicts(a, nil) {
		t.Fatal("nil routes must not conflict")
	}
}

// TestTableAddAndConflictsOf exercises the conflict-graph build.
func TestTableAddAndConflictsOf(t *testing.T) {
	tbl := NewTable()
	a := &model.Route{ID: "A", Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	b := &model.Route{ID: "B", Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	c := &model.Route{ID: "C", Sections: []model.RouteSection{{SectionID: "S2", Seq: 0}}}
	tbl.Add(a)
	tbl.Add(b)
	tbl.Add(c)
	cf := tbl.ConflictsOf("A")
	if len(cf) != 1 || cf[0] != "B" {
		t.Fatalf("A conflicts = %v, want [B]", cf)
	}
	if len(tbl.ConflictsOf("C")) != 0 {
		t.Fatal("C should have no conflicts")
	}
}

// TestTableRemove drops edges.
func TestTableRemove(t *testing.T) {
	tbl := NewTable()
	a := &model.Route{ID: "A", Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	b := &model.Route{ID: "B", Sections: []model.RouteSection{{SectionID: "S1", Seq: 0}}}
	tbl.Add(a)
	tbl.Add(b)
	tbl.Remove("B")
	if len(tbl.ConflictsOf("A")) != 0 {
		t.Fatal("removing B should clear A's conflicts")
	}
	if tbl.Get("B") != nil {
		t.Fatal("B should be gone")
	}
}

// TestTableFindByTerminal matches source+terminal.
func TestTableFindByTerminal(t *testing.T) {
	tbl := NewTable()
	a := &model.Route{ID: "A", SourceSignalID: "X", Terminal: "T1"}
	tbl.Add(a)
	got := tbl.FindByTerminal("X", "T1")
	if got == nil || got.ID != "A" {
		t.Fatal("FindByTerminal should find A")
	}
	if tbl.FindByTerminal("X", "ZZ") != nil {
		t.Fatal("no match expected for unknown terminal")
	}
}
