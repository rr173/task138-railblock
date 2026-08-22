// Package trainops applies train movement events (occupy/clear a section) to
// the interlocking state and computes the side effects: approach-lock
// transitions, three-point sectional releases, signal closures and
// route-normal releases. It is a pure layer over the model + interlocking
// package; the service/store layers persist the computed effects inside a
// transaction and append the corresponding events.
package trainops

import (
	"time"

	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/model"
)

// MoveKind classifies a train move event.
type MoveKind int

const (
	// MoveOccupy: the train entered a section.
	MoveOccupy MoveKind = iota
	// MoveClear: the train left a section.
	MoveClear
)

// Move is a train movement to apply: which train, which section, occupy or
// clear, and at what time (for event timestamps).
type Move struct {
	TrainID   string
	SectionID string
	Kind      MoveKind
	TS        time.Time
}

// Effect is the set of state changes a single move produces. The service layer
// applies each slice in order inside one transaction.
type Effect struct {
	// SectionStateChange records the section whose occupancy toggled and its
	// new occupied flag. Always set.
	SectionID   string
	NowOccupied bool
	// SectionOnceOccupied marks the section as having seen a train for the
	// route it belongs to. Set when a train occupies a route section.
	SectionOnceOccupiedForRoute string
	// ApproachLockRoute is the route to transition Established →
	// ApproachLocked because the move occupied its approach section.
	ApproachLockRoute string
	// AbandonCancelRoute is a route whose in-flight timed cancel should be
	// abandoned (train occupied the approach section while cancelling, so the
	// train is now legitimately bearing down on the route).
	AbandonCancelRoute string
	// SectionsToUnlock are section IDs to unlock (three-point release).
	SectionsToUnlock []string
	// RouteComplete is the route that has fully released (terminal cleared);
	// the service moves it to Unlocked.
	RouteComplete string
	// SignalCloseRoutes are routes whose home signal must close because a
	// train entered the route (signal drops to red as the train passes).
	SignalCloseRoutes []string
}

// ApplyMove computes the effect of a train move on the yard. It neither
// mutates the yard nor persists anything; the service does both from the
// returned Effect.
func ApplyMove(y *interlocking.Yard, mv Move) Effect {
	eff := Effect{SectionID: mv.SectionID, NowOccupied: mv.Kind == MoveOccupy}
	if _, ok := y.Sections[mv.SectionID]; !ok {
		return eff
	}
	for _, r := range y.Routes.All() {
		// An approach-section occupy affects the route regardless of lock state:
		// it is not one of r.Sections (it is tracked separately), so it must be
		// examined before the HasSection short-circuit. OnApproachOccupied is the
		// pure rule that decides whether the route transitions to ApproachLocked
		// (Established) or abandons an in-flight timed cancel (Cancelling).
		if mv.SectionID == r.ApproachSectionID && mv.Kind == MoveOccupy {
			res := interlocking.OnApproachOccupied(r)
			if res.TransitionsToApproachLocked {
				eff.ApproachLockRoute = r.ID
			}
			if res.AbandonsCancel {
				eff.AbandonCancelRoute = r.ID
			}
		}
		if !r.State.IsLocked() && r.State != model.RouteCancelling {
			continue
		}
		if !r.HasSection(mv.SectionID) {
			continue
		}
		// The move is on a route section.
		if mv.Kind == MoveOccupy {
			eff.SectionOnceOccupiedForRoute = r.ID
			eff.SignalCloseRoutes = append(eff.SignalCloseRoutes, r.ID)
			rel := interlocking.ThreePointRelease(r, y, mv.SectionID, mv.TS, true)
			eff.SectionsToUnlock = append(eff.SectionsToUnlock, rel.SectionsToUnlock...)
			if rel.RouteComplete {
				eff.RouteComplete = r.ID
			}
		} else {
			rel := interlocking.ThreePointRelease(r, y, mv.SectionID, mv.TS, false)
			eff.SectionsToUnlock = append(eff.SectionsToUnlock, rel.SectionsToUnlock...)
			if rel.RouteComplete {
				eff.RouteComplete = r.ID
			}
		}
	}
	return eff
}
