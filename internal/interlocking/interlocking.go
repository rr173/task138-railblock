// Package interlocking holds the safety-rule pure functions of the engine:
// route-establishment preconditions, switch-movement checks, sectional
// (three-point) release, approach-lock detection and the timed-cancel
// decision. The store/service layer assembles a Yard snapshot from the
// database, calls these functions, and applies the returned actions inside a
// transaction. Keeping the rules pure makes them unit-testable in isolation
// and makes restart recovery (replay) exact: the same events feed the same
// pure functions.
package interlocking

import (
	"errors"
	"time"

	"task138-railblock/internal/model"
	"task138-railblock/internal/routeplan"
)

// Yard is a read-only snapshot of the station's elements that the pure
// functions reason over. The store builds it from its rows before each call.
type Yard struct {
	StationID string
	Switches  map[string]*model.Switch
	Signals   map[string]*model.Signal
	Sections  map[string]*model.Section
	// SectionOnceOccupied records, per route, which sections have been
	// occupied at least once since the route was established. It is keyed by
	// route ID, then section ID. The sectional release uses it so a brief
	// occupancy that has already cleared is still counted as "train passed".
	SectionOnceOccupied map[string]map[string]bool
	// RoutesTable holds the station's routes and their conflict graph.
	Routes *routeplan.Table
	// Trains in the station.
	Trains map[string]*model.Train
}

// Err... are the typed interlocking errors. The httpapi layer maps each to an
// HTTP status so a client can distinguish "occupied section" (fix the yard)
// from "conflicting route" (release the other route first).
var (
	ErrConflictingRoute        = errors.New("interlocking: conflicting route already established")
	ErrSectionOccupied         = errors.New("interlocking: a route section is occupied")
	ErrSectionLocked           = errors.New("interlocking: a route section is locked")
	ErrSwitchInOccupiedSection = errors.New("interlocking: switch is in an occupied section")
	ErrSwitchLocked            = errors.New("interlocking: switch is locked by a route")
	ErrSwitchLostIndication    = errors.New("interlocking: switch has lost position indication")
	ErrRouteNotEstablished     = errors.New("interlocking: route is not established")
	ErrRouteTerminalState      = errors.New("interlocking: route is in a terminal state")
	ErrApproachLocked          = errors.New("interlocking: route is approach-locked; use the timed cancel")
	ErrSectionNotCleared       = errors.New("interlocking: section is not cleared")
	ErrSignalCannotOpen         = errors.New("interlocking: signal cannot open")
	ErrInvalidRoute            = errors.New("interlocking: route definition is invalid")
	ErrNotFound                = errors.New("interlocking: not found")
)

// SwitchMove is a planned switch reversal required before a route can lock.
type SwitchMove struct {
	SwitchID string
	To       model.SwitchPosition
}

// EstablishResult describes the side effects of establishing a route, computed
// up front so the store can apply them in one transaction.
type EstablishResult struct {
	// SwitchMoves are the reversals to perform before locking.
	SwitchMoves []SwitchMove
	// SwitchLocks are the switch IDs to lock to this route.
	SwitchLocks []string
	// SectionLocks are the section IDs to lock to this route.
	SectionLocks []string
}

// CheckEstablish verifies that a route can be established in the given yard and
// returns the planned side effects. It does not mutate the yard.
func CheckEstablish(y *Yard, r *model.Route) (*EstablishResult, error) {
	if r == nil {
		return nil, ErrInvalidRoute
	}
	if err := validateRouteDefinition(y, r); err != nil {
		return nil, err
	}
	switch r.State {
	case model.RoutePending, model.RouteFailed, model.RouteUnlocked, model.RouteCancelled:
		// ok to (re-)establish
	default:
		return nil, ErrRouteTerminalState
	}
	res := &EstablishResult{}
	for _, rsp := range r.SwitchPositions {
		sw, ok := y.Switches[rsp.SwitchID]
		if !ok {
			return nil, ErrInvalidRoute
		}
		if !sw.HasIndication {
			return nil, ErrSwitchLostIndication
		}
		if sw.CurrentPosition != rsp.RequiredPosition {
			sec, ok := y.Sections[sw.SectionID]
			if !ok {
				return nil, ErrInvalidRoute
			}
			if sec.Occupied {
				return nil, ErrSwitchInOccupiedSection
			}
			if sw.Locked && sw.LockedByRoute != r.ID {
				return nil, ErrSwitchLocked
			}
			res.SwitchMoves = append(res.SwitchMoves, SwitchMove{SwitchID: rsp.SwitchID, To: rsp.RequiredPosition})
		}
		res.SwitchLocks = append(res.SwitchLocks, rsp.SwitchID)
	}
	for _, conflictID := range y.Routes.ConflictsOf(r.ID) {
		other := y.Routes.Get(conflictID)
		if other == nil {
			continue
		}
		if other.State.IsLocked() {
			return nil, ErrConflictingRoute
		}
	}
	for _, rs := range r.Sections {
		sec, ok := y.Sections[rs.SectionID]
		if !ok {
			return nil, ErrInvalidRoute
		}
		if rs.SectionID == r.ApproachSectionID {
			if sec.Locked && sec.LockedByRoute != r.ID {
				return nil, ErrSectionLocked
			}
			continue
		}
		if sec.Occupied {
			return nil, ErrSectionOccupied
		}
		if sec.Locked && sec.LockedByRoute != r.ID {
			return nil, ErrSectionLocked
		}
		res.SectionLocks = append(res.SectionLocks, rs.SectionID)
	}
	return res, nil
}

// validateRouteDefinition sanity-checks that the route references real
// switches/sections/signals in the yard.
func validateRouteDefinition(y *Yard, r *model.Route) error {
	if r.StationID == "" || r.SourceSignalID == "" || r.Terminal == "" {
		return ErrInvalidRoute
	}
	if len(r.Sections) == 0 {
		return ErrInvalidRoute
	}
	if _, ok := y.Signals[r.SourceSignalID]; !ok {
		return ErrInvalidRoute
	}
	for _, rsp := range r.SwitchPositions {
		if _, ok := y.Switches[rsp.SwitchID]; !ok {
			return ErrInvalidRoute
		}
	}
	for _, rs := range r.Sections {
		if _, ok := y.Sections[rs.SectionID]; !ok {
			return ErrInvalidRoute
		}
	}
	if r.ApproachSectionID != "" {
		if _, ok := y.Sections[r.ApproachSectionID]; !ok {
			return ErrInvalidRoute
		}
	}
	return nil
}

// CheckSwitchMove verifies that a switch may be reversed.
func CheckSwitchMove(y *Yard, swID string, to model.SwitchPosition) error {
	sw, ok := y.Switches[swID]
	if !ok {
		return ErrInvalidRoute
	}
	if !sw.HasIndication {
		return ErrSwitchLostIndication
	}
	sec, ok := y.Sections[sw.SectionID]
	if ok && sec.Occupied && to == sw.CurrentPosition {
		return ErrSwitchInOccupiedSection
	}
	if sw.Locked {
		return ErrSwitchLocked
	}
	return nil
}

// ReleaseResult is the set of sections that may be unlocked when a section
// becomes occupied or cleared, computed by the three-point check.
type ReleaseResult struct {
	// SectionsToUnlock are section IDs whose locks may now be released.
	SectionsToUnlock []string
	// RouteComplete is true when the terminal section has been occupied and
	// cleared, meaning the whole route may release.
	RouteComplete bool
}

// ThreePointRelease computes which sections may be released given a section
// state change. The "three-point" rule: a route section s[i] may release when
// the train has provably passed it — s[i] has been occupied-and-cleared
// (once_occupied and currently clear), s[i+1] is currently occupied, and
// (where it exists) s[i+2] is also currently occupied. Near the tail of the
// route (idx+2 past the end) the s[i+2] requirement is dropped because a train
// cannot simultaneously occupy the two trailing sections — the train occupying
// s[i+1] after having cleared s[i] is itself the proof of passage. When the
// terminal section clears after once-occupancy, the whole route releases.
//
// isOccupy distinguishes a "train entered s[i]" event from a "train left s[i]"
// event. On occupy, trailing sections two-behind are candidates; on clear, the
// section itself (and, if terminal, the whole route) is the candidate.
func ThreePointRelease(r *model.Route, y *Yard, changedSectionID string, now time.Time, isOccupy bool) ReleaseResult {
	out := ReleaseResult{}
	if !r.State.IsLocked() && r.State != model.RouteCancelling {
		return out
	}
	once := y.SectionOnceOccupied[r.ID]
	idx := -1
	for i, rs := range r.Sections {
		if rs.SectionID == changedSectionID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return out
	}
	if isOccupy {
		// Train entered s[idx]. Trailing sections s[idx-1] and s[idx-2] may
		// release if they are clear and once-occupied.
		if idx >= 1 {
			prevID := r.Sections[idx-1].SectionID
			if prev, ok := y.Sections[prevID]; ok && !prev.Occupied && once[prevID] {
				out.SectionsToUnlock = append(out.SectionsToUnlock, prevID)
			}
		}
		if idx >= 2 {
			trailID := r.Sections[idx-2].SectionID
			if trail, ok := y.Sections[trailID]; ok && !trail.Occupied && once[trailID] {
				if !contains(out.SectionsToUnlock, trailID) {
					out.SectionsToUnlock = append(out.SectionsToUnlock, trailID)
				}
			}
		}
		return out
	}
	// Clear event.
	// Terminal section cleared after once-occupancy → whole route releases.
	if idx == len(r.Sections)-1 && once[changedSectionID] {
		for _, rs := range r.Sections {
			if s, ok := y.Sections[rs.SectionID]; ok && s.Locked && s.LockedByRoute == r.ID {
				if !contains(out.SectionsToUnlock, rs.SectionID) {
					out.SectionsToUnlock = append(out.SectionsToUnlock, rs.SectionID)
				}
			}
		}
		out.RouteComplete = true
		return out
	}
	// Non-terminal clear: s[idx] itself may release if the train is at least
	// one section ahead (s[idx+1] occupied) and, where present, s[idx+2] is
	// also occupied (or idx+2 is past the end — the tail relaxation).
	if !once[changedSectionID] {
		return out
	}
	ahead1, ok1 := y.Sections[r.Sections[idx+1].SectionID]
	if !ok1 || !ahead1.Occupied {
		return out
	}
	twoAhead := idx + 2
	if twoAhead >= len(r.Sections) {
		// Tail relaxation: train in s[idx+1] after clearing s[idx] is enough.
		out.SectionsToUnlock = append(out.SectionsToUnlock, changedSectionID)
		return out
	}
	ahead2, ok2 := y.Sections[r.Sections[twoAhead].SectionID]
	if ok2 && ahead2.Occupied {
		out.SectionsToUnlock = append(out.SectionsToUnlock, changedSectionID)
	}
	return out
}

// contains is a small helper for string slices used internally.
func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// CancelDecision describes what a manual cancel should do given the route
// state and whether the approach section is currently occupied.
type CancelDecision struct {
	// NeedTimedDelay is true when the route is approach-locked and the cancel
	// must enter the Cancelling state with a timer.
	NeedTimedDelay bool
	// Delay is the approach-lock delay to start (only when NeedTimedDelay).
	Delay time.Duration
	// Immediate is true when the cancel can release at once (route
	// Established with no train approaching, i.e. approach section free).
	Immediate bool
}

// DecideCancel decides how to cancel a route.
func DecideCancel(y *Yard, r *model.Route) (CancelDecision, error) {
	switch r.State {
	case model.RouteCancelling:
		return CancelDecision{}, ErrApproachLocked
	case model.RouteEstablished:
		if r.ApproachSectionID != "" {
			if sec, ok := y.Sections[r.ApproachSectionID]; ok && sec.Occupied {
				return CancelDecision{NeedTimedDelay: true, Delay: r.Kind.ApproachLockDelay()}, nil
			}
		}
		return CancelDecision{Immediate: true}, nil
	case model.RouteApproachLocked:
		return CancelDecision{NeedTimedDelay: true, Delay: r.Kind.ApproachLockDelay()}, nil
	}
	return CancelDecision{}, ErrRouteNotEstablished
}

// FaultUnlockTarget returns the sections and switches a fault-unlock should
// release for a route, regardless of occupancy.
func FaultUnlockTarget(y *Yard, r *model.Route) ([]string, []string) {
	sections := []string{}
	switches := []string{}
	for _, rs := range r.Sections {
		if s, ok := y.Sections[rs.SectionID]; ok && s.Locked && s.LockedByRoute == r.ID {
			sections = append(sections, rs.SectionID)
		}
	}
	for _, rsp := range r.SwitchPositions {
		if sw, ok := y.Switches[rsp.SwitchID]; ok && sw.Locked && sw.LockedByRoute == r.ID {
			switches = append(switches, rsp.SwitchID)
		}
	}
	return sections, switches
}

// ApproachLockResult describes what an occupied approach section does to a route.
type ApproachLockResult struct {
	TransitionsToApproachLocked bool
	AbandonsCancel              bool
}

// OnApproachOccupied is called when a train occupies a route's approach section.
func OnApproachOccupied(r *model.Route) ApproachLockResult {
	switch r.State {
	case model.RouteEstablished:
		return ApproachLockResult{TransitionsToApproachLocked: true}
	case model.RouteCancelling:
		return ApproachLockResult{AbandonsCancel: true}
	}
	return ApproachLockResult{}
}
