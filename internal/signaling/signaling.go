// Package signaling computes the displayed aspect of a home signal from the
// interlocking state. The aspect is a pure function of (route state, switch
// positions, section occupancy, indication health) and is never a free input:
// a signal shows red unless its route is established with switches locked in
// the required positions, all sections un-occupied (except the approach) and
// all switches showing indication.
package signaling

import "task138-railblock/internal/model"

// State is the read-only yard state ComputeAspect needs. The store assembles it
// from its rows so the signaling package does not depend on persistence.
type State struct {
	// Route being guarded by the signal (nil if the signal is not protecting
	// any established route).
	Route *model.Route
	// Switches indexed by ID, for indication/position checks.
	Switches map[string]*model.Switch
	// Sections indexed by ID, for occupancy checks along the route.
	Sections map[string]*model.Section
}

// ComputeAspect returns the aspect a home signal should display given the
// yard state. Rules (in priority order):
//
//  1. No route, or route not Established/ApproachLocked/Cancelling → Red.
//  2. Any required switch lost indication → Red (safety: never open over a
//     switch whose position is unknown).
//  3. Any required switch not at its required position → Red.
//  4. Any route section (other than the approach section) occupied → Red
//     (the train is foul of the route or the section hasn't cleared).
//  5. Otherwise a permissive aspect: Green for a straight route (no switch
//     reversed), DoubleYellow when a diverging switch is required, Yellow as
//     a conservative fallback.
func ComputeAspect(st State) model.SignalAspect {
	r := st.Route
	if r == nil {
		return model.AspectRed
	}
	switch r.State {
	case model.RouteEstablished, model.RouteApproachLocked, model.RouteCancelling:
	default:
		return model.AspectRed
	}
	diverging := false
	for _, rsp := range r.SwitchPositions {
		sw, ok := st.Switches[rsp.SwitchID]
		if !ok {
			return model.AspectRed
		}
		if sw.CurrentPosition != rsp.RequiredPosition {
			return model.AspectRed
		}
		if rsp.RequiredPosition == model.PositionReverse {
			diverging = true
		}
	}
	// Any occupied section inside the route (excluding the approach, which is
	// legitimately occupied when a train is bearing down) forces red — the
	// train has entered the route.
	for _, rs := range r.Sections {
		if rs.SectionID == r.ApproachSectionID {
			continue
		}
		sec, ok := st.Sections[rs.SectionID]
		if !ok {
			return model.AspectRed
		}
		if sec.Occupied {
			return model.AspectRed
		}
	}
	if diverging {
		return model.AspectDoubleYellow
	}
	return model.AspectGreen
}

// AspectForOpen returns the permissive aspect an established route should show
// at the moment it is opened (before any train has entered). This is the same
// as ComputeAspect on a freshly established route; factored out so the caller
// can assert "the signal opened to permissive" without re-deriving the rule.
func AspectForOpen(r *model.Route, switches map[string]*model.Switch) model.SignalAspect {
	diverging := false
	for _, rsp := range r.SwitchPositions {
		sw, ok := switches[rsp.SwitchID]
		if !ok || !sw.HasIndication {
			return model.AspectRed
		}
		if sw.CurrentPosition != rsp.RequiredPosition {
			return model.AspectRed
		}
		if rsp.RequiredPosition == model.PositionReverse {
			diverging = true
		}
	}
	if diverging {
		return model.AspectDoubleYellow
	}
	return model.AspectGreen
}
