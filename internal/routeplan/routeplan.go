// Package routeplan owns the route table and the conflict graph. It answers
// two questions the interlocking layer needs repeatedly: "given a source
// signal and a terminal, what route is that?" and "given a route, which other
// routes conflict with it?".
//
// A conflict between two routes arises when they cannot both be established at
// once because either (a) they share at least one track section, or (b) they
// require the same switch in opposite positions. The conflict relation is
// symmetric and irreflexive. Computing it once at definition time keeps the
// hot path (route establishment) cheap: a single set lookup.
package routeplan

import "task138-railblock/internal/model"

// Table holds a station's routes plus the precomputed conflict adjacency.
type Table struct {
	// routes indexed by route ID.
	routes map[string]*model.Route
	// conflicts[r] = set of route IDs that conflict with r.
	conflicts map[string]map[string]struct{}
}

// NewTable returns an empty route table.
func NewTable() *Table {
	return &Table{
		routes:    make(map[string]*model.Route),
		conflicts: make(map[string]map[string]struct{}),
	}
}

// Add registers a route and recomputes its conflict edges against all existing
// routes. Adding a duplicate route ID replaces the prior route and recomputes
// its edges.
func (t *Table) Add(r *model.Route) {
	t.routes[r.ID] = r
	if _, ok := t.conflicts[r.ID]; !ok {
		t.conflicts[r.ID] = make(map[string]struct{})
	}
	// Recompute edges to every other route.
	for otherID, other := range t.routes {
		if otherID == r.ID {
			continue
		}
		if Conflicts(r, other) {
			t.conflicts[r.ID][otherID] = struct{}{}
			t.conflicts[otherID][r.ID] = struct{}{}
		} else {
			delete(t.conflicts[r.ID], otherID)
			delete(t.conflicts[otherID], r.ID)
		}
	}
}

// Remove drops a route and its conflict edges.
func (t *Table) Remove(id string) {
	delete(t.routes, id)
	for other := range t.conflicts[id] {
		delete(t.conflicts[other], id)
	}
	delete(t.conflicts, id)
}

// Get returns the route with the given ID, or nil.
func (t *Table) Get(id string) *model.Route {
	return t.routes[id]
}

// All returns every route in the table.
func (t *Table) All() []*model.Route {
	out := make([]*model.Route, 0, len(t.routes))
	for _, r := range t.routes {
		out = append(out, r)
	}
	return out
}

// ConflictsOf returns the route IDs that conflict with id.
func (t *Table) ConflictsOf(id string) []string {
	set := t.conflicts[id]
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}

// FindByTerminal returns the route whose source signal and terminal match. If
// several routes share a source+terminal (e.g. a train and a shunt route to
// the same bay), the first added wins; callers that need disambiguation pass
// the route code instead.
func (t *Table) FindByTerminal(sourceSignal, terminal string) *model.Route {
	for _, r := range t.routes {
		if r.SourceSignalID == sourceSignal && r.Terminal == terminal {
			return r
		}
	}
	return nil
}

// Conflicts reports whether two routes conflict: they share a section, or they
// require the same switch in opposite positions. A route never conflicts with
// itself.
func Conflicts(a, b *model.Route) bool {
	if a == nil || b == nil || a.ID == b.ID {
		return false
	}
	// (a) shared section.
	for _, sa := range a.Sections {
		for _, sb := range b.Sections {
			if sa.SectionID == sb.SectionID && sa.Seq != sb.Seq {
				return true
			}
		}
	}
	// (b) shared switch in opposite required positions.
	for _, pa := range a.SwitchPositions {
		pb, ok := b.RequiredPositionOf(pa.SwitchID)
		if !ok {
			continue
		}
		if pb != pa.RequiredPosition {
			return true
		}
	}
	return false
}
