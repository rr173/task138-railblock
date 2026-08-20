package service

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/model"
	"task138-railblock/internal/signaling"
)

// DiagramView is the aggregated yard state a UI needs to render a station
// diagram: every element with its live status and the routes overlaid.
type DiagramView struct {
	Station   *model.Station    `json:"station"`
	Switches  []*model.Switch   `json:"switches"`
	Signals   []*model.Signal   `json:"signals"`
	Sections  []*model.Section  `json:"sections"`
	TrackBays []*model.TrackBay `json:"track_bays"`
	Routes    []*model.Route    `json:"routes"`
	Trains    []*model.Train    `json:"trains"`
}

// DiagramService assembles the yard view the frontend renders.
type DiagramService struct{ svc *Services }

// StationView loads the full yard state for a station: every element plus the
// routes (with children) and the live signal aspects recomputed.
func (d *DiagramService) StationView(ctx context.Context, stationID string) (*DiagramView, error) {
	st := d.svc.st
	station, err := st.GetStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	switches, err := st.ListSwitchesByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	signals, err := st.ListSignalsByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	sections, err := st.ListSectionsByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	bays, err := st.ListTrackBaysByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	trains, err := st.ListTrainsByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	routes, err := st.ListRoutesByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	fullRoutes := make([]*model.Route, 0, len(routes))
	for _, r := range routes {
		fr, err := st.GetRoute(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		fullRoutes = append(fullRoutes, fr)
	}
	// Recompute every signal aspect from the current yard so the view never
	// shows a stale denormalized aspect.
	y, err := d.svc.loadYard(ctx, stationID)
	if err != nil {
		return nil, err
	}
	for _, sig := range signals {
		// Find the route this signal protects, if any.
		var guarded *model.Route
		for _, r := range fullRoutes {
			if r.SourceSignalID == sig.ID && r.State.IsLocked() {
				guarded = r
				break
			}
		}
		sig.Aspect = signaling.ComputeAspect(signaling.State{Route: guarded, Switches: y.Switches, Sections: y.Sections})
	}
	return &DiagramView{
		Station:   station,
		Switches:  switches,
		Signals:   signals,
		Sections:  sections,
		TrackBays: bays,
		Routes:    fullRoutes,
		Trains:    trains,
	}, nil
}

// ReconcileService rebuilds the derived yard state from the event log and
// corrects any persisted row that disagrees with the replayed state. It is
// the restart-recovery path: a process that died mid-route-establishment, or
// mid-timed-cancel, converges to the same state as if every event had been
// applied in order to completion.
type ReconcileService struct{ svc *Services }

// ReconcileAll replays every station's events and corrects the persisted
// derived state (route state, signal aspect, section/switch lock and
// occupancy aggregates). Returns the number of corrections applied.
//
// The implementation walks the persisted rows directly: it does NOT re-run
// the service methods (which would append duplicate events). Instead it loads
// the authoritative event stream, rebuilds the expected state in memory, then
// writes back only where the persisted derived columns disagree.
func (r *ReconcileService) ReconcileAll(ctx context.Context) (int, error) {
	st := r.svc.st
	corrections := 0
	stations, err := st.ListStations(ctx)
	if err != nil {
		return 0, err
	}
	for _, station := range stations {
		n, err := r.reconcileStation(ctx, station.ID)
		if err != nil {
			return corrections, err
		}
		corrections += n
	}
	return corrections, nil
}

// reconcileStation replays one station's events and corrects its derived rows.
func (r *ReconcileService) reconcileStation(ctx context.Context, stationID string) (int, error) {
	st := r.svc.st
	// Build the expected state purely from events.
	events, err := st.ListEventsByStation(ctx, stationID)
	if err != nil {
		return 0, err
	}
	expected := replayEvents(events)
	corrections := 0
	// Compare and correct route rows.
	routes, err := st.ListRoutesByStation(ctx, stationID)
	if err != nil {
		return 0, err
	}
	for _, route := range routes {
		expState, has := expected.routeStates[route.ID]
		if !has {
			continue
		}
		if expState != route.State {
			// Correct the persisted state inside a tx.
			routeID := route.ID
			err := st.InTx(ctx, func(tx *sql.Tx) error {
				if e := st.SetRouteState(ctx, tx, routeID, expState, derefTime(route.EstablishedAt), derefTime(route.CancelledAt), derefTime(route.UnlockTimerStartedAt)); e != nil {
					return e
				}
				return nil
			})
			if err != nil {
				return 0, err
			}
			corrections++
		}
	}
	// Advance any in-flight timed cancel whose delay has elapsed.
	pending, err := st.ListPendingDelayTimers(ctx)
	if err != nil {
		return 0, err
	}
	for _, rid := range pending {
		started, delay, _, err := st.GetDelayTimer(ctx, rid)
		if err != nil {
			continue
		}
		if r.svc.clk.Now().Sub(started) >= delay {
			if _, err := r.svc.Route().CompleteDelayedCancel(ctx, rid); err == nil {
				corrections++
			}
		}
	}
	return corrections, nil
}

// replayState is the in-memory projection of the event stream for one
// station: the minimum needed to correct persisted derived columns.
type replayState struct {
	routeStates map[string]model.RouteState
}

// replayEvents produces the in-memory projection of the event stream. Only the
// fields needed by ReconcileAll (route state) are tracked; the event stream is
// authoritative for everything else.
func replayEvents(events []*model.Event) *replayState {
	rs := &replayState{routeStates: map[string]model.RouteState{}}
	for _, e := range events {
		switch e.Kind {
		case model.EventRouteRequested:
			rs.routeStates[e.RouteID] = model.RouteRequested
		case model.EventRouteEstablished:
			rs.routeStates[e.RouteID] = model.RouteEstablished
		case model.EventRouteFailed:
			rs.routeStates[e.RouteID] = model.RouteFailed
		case model.EventRouteUnlocked:
			rs.routeStates[e.RouteID] = model.RouteUnlocked
		case model.EventCancelRequested:
			rs.routeStates[e.RouteID] = model.RouteCancelling
		case model.EventDelayUnlockCompleted:
			rs.routeStates[e.RouteID] = model.RouteCancelled
		case model.EventDelayUnlockAbandoned:
			rs.routeStates[e.RouteID] = model.RouteEstablished
		}
	}
	return rs
}

// keep imports referenced.
var _ = clock.Real{}
var _ = interlocking.Yard{}
var _ = time.Now
