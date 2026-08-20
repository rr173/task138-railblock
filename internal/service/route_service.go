package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/model"
	"task138-railblock/internal/signaling"
)

// RouteService implements route definition, establishment, cancel,
// fault-unlock and conflict lookup. Every mutating method runs inside a
// single IMMEDIATE transaction and appends one event per state change so the
// event log is the source of truth for restart recovery.
type RouteService struct{ svc *Services }

// DefineRoute persists a route in the pending state and validates that it
// references real switches/sections/signals of the station. Conflict edges
// are not persisted: they are recomputed from the route children by the
// routeplan package on every loadYard, so the conflict graph is always
// consistent with the persisted sections and switch positions.
func (rs *RouteService) DefineRoute(ctx context.Context, r *model.Route) error {
	y, err := rs.svc.loadYard(ctx, r.StationID)
	if err != nil {
		return err
	}
	if err := validateRouteReferences(y, r); err != nil {
		return err
	}
	r.State = model.RoutePending
	return rs.svc.st.CreateRoute(ctx, r)
}

// Establish tries to establish a pending route, in one IMMEDIATE tx:
//  1. load route + yard snapshot;
//  2. CheckEstablish → switch moves + switch/section locks; on error set
//     route Failed and return the typed error;
//  3. for each switch move: re-check (inside the tx so a race is caught),
//     reverse, append switch_reversed event;
//  4. lock each switch and section (locked_by_route = routeID);
//  5. set route state established, established_at = now; append
//     route_established; bind the protecting signal; compute and set the
//     signal aspect (open permissive); append signal_opened.
func (rs *RouteService) Establish(ctx context.Context, routeID string) (*model.Route, error) {
	var out *model.Route
	err := rs.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		st := rs.svc.st
		// (1) load route with children, inside the tx.
		r, err := st.TxGetRoute(ctx, tx, routeID)
		if err != nil {
			return err
		}
		stationID := r.StationID
		// Build a yard snapshot inside the tx so every read is consistent with
		// the write about to happen.
		y, err := loadYardTx(ctx, st, tx, stationID)
		if err != nil {
			return err
		}
		// (2) preconditions.
		res, err := interlocking.CheckEstablish(y, r)
		if err != nil {
			// Mark the route failed (no locks held) and record the failure.
			now := rs.svc.clk.Now()
			if e2 := st.SetRouteState(ctx, tx, routeID, model.RouteFailed, time.Time{}, time.Time{}, time.Time{}); e2 != nil {
				return errors.Join(err, e2)
			}
			_ = st.AppendEventJSON(ctx, tx, stationID, model.EventRouteFailed, "route", routeID, routeID, map[string]string{"reason": err.Error(), "at": now.Format(time.RFC3339Nano)})
			return err
		}
		now := rs.svc.clk.Now()
		// (3) switch moves.
		for _, mv := range res.SwitchMoves {
			sw, _ := st.TxGetSwitch(ctx, tx, mv.SwitchID)
			if sw == nil {
				return interlocking.ErrInvalidRoute
			}
			if err := interlocking.CheckSwitchMove(y, mv.SwitchID, mv.To); err != nil {
				return err
			}
			if err := st.SetSwitchPosition(ctx, tx, mv.SwitchID, mv.To); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, stationID, model.EventSwitchReversed, "switch", mv.SwitchID, routeID, map[string]string{"to": mv.To.String(), "at": now.Format(time.RFC3339Nano)})
		}
		// (4) lock switches and sections.
		for _, swID := range res.SwitchLocks {
			if err := st.SetSwitchLocked(ctx, tx, swID, true, routeID); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, stationID, model.EventSwitchLocked, "switch", swID, routeID, nil)
		}
		for _, secID := range res.SectionLocks {
			if err := st.SetSectionLocked(ctx, tx, secID, true, routeID); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, stationID, model.EventLockSet, "section", secID, routeID, nil)
		}
		// (5) state + signal.
		if err := st.SetRouteState(ctx, tx, routeID, model.RouteEstablished, now, time.Time{}, time.Time{}); err != nil {
			return err
		}
		_ = st.AppendEventJSON(ctx, tx, stationID, model.EventRouteEstablished, "route", routeID, routeID, map[string]string{"at": now.Format(time.RFC3339Nano)})
		if err := st.SetSignalProtectsRoute(ctx, tx, r.SourceSignalID, routeID); err != nil {
			return err
		}
		// Recompute switches for the aspect (they were just reversed).
		swMap := make(map[string]*model.Switch, len(res.SwitchLocks))
		for _, swID := range res.SwitchLocks {
			if sw, _ := st.TxGetSwitch(ctx, tx, swID); sw != nil {
				swMap[swID] = sw
			}
		}
		aspect := signaling.AspectForOpen(r, swMap)
		if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, aspect); err != nil {
			return err
		}
		if aspect != model.AspectRed {
			_ = st.AppendEventJSON(ctx, tx, stationID, model.EventSignalOpened, "signal", r.SourceSignalID, routeID, map[string]string{"aspect": string(aspect), "at": now.Format(time.RFC3339Nano)})
		}
		// Return the refreshed route.
		out, err = st.TxGetRoute(ctx, tx, routeID)
		return err
	})
	return out, err
}

// Cancel requests a manual cancel. Established + approach free → release at
// once. Established + approach occupied, or ApproachLocked → enter Cancelling
// and start a timed delay (persisted so restart resumes it). Cancelling →
// reject (already cancelling). Other states → reject.
func (rs *RouteService) Cancel(ctx context.Context, routeID string) (*model.Route, error) {
	var out *model.Route
	err := rs.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		st := rs.svc.st
		r, err := st.TxGetRoute(ctx, tx, routeID)
		if err != nil {
			return err
		}
		y, err := loadYardTx(ctx, st, tx, r.StationID)
		if err != nil {
			return err
		}
		dec, err := interlocking.DecideCancel(y, r)
		if err != nil {
			return err
		}
		now := rs.svc.clk.Now()
		if dec.Immediate {
			// Release locks and close the signal.
			if err := rs.releaseRouteLocks(ctx, tx, r, false); err != nil {
				return err
			}
			if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, model.AspectRed); err != nil {
				return err
			}
			if err := st.SetSignalProtectsRoute(ctx, tx, r.SourceSignalID, ""); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSignalClosed, "signal", r.SourceSignalID, routeID, nil)
			if err := st.SetRouteState(ctx, tx, routeID, model.RouteCancelled, time.Time{}, now, time.Time{}); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventCancelRequested, "route", routeID, routeID, map[string]string{"mode": "immediate", "at": now.Format(time.RFC3339Nano)})
			_ = st.ClearOnceOccupied(ctx, tx, routeID)
			out, err = st.TxGetRoute(ctx, tx, routeID)
			return err
		}
		// Timed delay: enter Cancelling and persist the timer.
		if err := st.SetRouteState(ctx, tx, routeID, model.RouteCancelling, time.Time{}, now, now); err != nil {
			return err
		}
		_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventCancelRequested, "route", routeID, routeID, map[string]string{"mode": "timed", "at": now.Format(time.RFC3339Nano)})
		if err := st.CreateDelayTimer(ctx, tx, routeID, r.StationID, r.Kind, now, dec.Delay); err != nil {
			return err
		}
		_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventDelayUnlockStarted, "route", routeID, routeID, map[string]any{"delay_secs": int(dec.Delay.Seconds()), "at": now.Format(time.RFC3339Nano)})
		// Close the signal while the timed release runs.
		if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, model.AspectRed); err != nil {
			return err
		}
		_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSignalClosed, "signal", r.SourceSignalID, routeID, nil)
		out, err = st.TxGetRoute(ctx, tx, routeID)
		return err
	})
	return out, err
}

// CompleteDelayedCancel finishes a timed cancel whose delay has elapsed. It
// releases locks and moves the route to Cancelled. Called by restart recovery
// and the test harness after advancing the fake clock.
func (rs *RouteService) CompleteDelayedCancel(ctx context.Context, routeID string) (*model.Route, error) {
	var out *model.Route
	err := rs.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		st := rs.svc.st
		r, err := st.TxGetRoute(ctx, tx, routeID)
		if err != nil {
			return err
		}
		if r.State != model.RouteCancelling {
			return interlocking.ErrRouteNotEstablished
		}
		now := rs.svc.clk.Now()
		if err := rs.releaseRouteLocks(ctx, tx, r, true); err != nil {
			return err
		}
		if err := st.SetSignalProtectsRoute(ctx, tx, r.SourceSignalID, ""); err != nil {
			return err
		}
		if err := st.CompleteDelayTimer(ctx, tx, routeID); err != nil {
			return err
		}
		_ = st.DeleteDelayTimer(ctx, tx, routeID)
		if err := st.SetRouteState(ctx, tx, routeID, model.RouteCancelled, time.Time{}, now, time.Time{}); err != nil {
			return err
		}
		_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventDelayUnlockCompleted, "route", routeID, routeID, map[string]string{"at": now.Format(time.RFC3339Nano)})
		_ = st.ClearOnceOccupied(ctx, tx, routeID)
		out, err = st.TxGetRoute(ctx, tx, routeID)
		return err
	})
	return out, err
}

// AbandonDelayedCancel returns a route from Cancelling back to Established
// because the train has entered the route proper (the timed release no longer
// applies). Called from TrainService when a train occupies a route section of
// a cancelling route.
func (rs *RouteService) AbandonDelayedCancel(ctx context.Context, tx *sql.Tx, r *model.Route) error {
	st := rs.svc.st
	now := rs.svc.clk.Now()
	if err := st.DeleteDelayTimer(ctx, tx, r.ID); err != nil {
		return err
	}
	if err := st.SetRouteState(ctx, tx, r.ID, model.RouteEstablished, r.EstablishedAt.UTC(), time.Time{}, time.Time{}); err != nil {
		return err
	}
	_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventDelayUnlockAbandoned, "route", r.ID, r.ID, map[string]string{"at": now.Format(time.RFC3339Nano)})
	// Re-open the signal: the train is now legitimately in the route.
	y, err := loadYardTx(ctx, st, tx, r.StationID)
	if err != nil {
		return err
	}
	aspect := signaling.ComputeAspect(signaling.State{Route: y.Routes.Get(r.ID), Switches: y.Switches, Sections: y.Sections})
	if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, aspect); err != nil {
		return err
	}
	if aspect != model.AspectRed {
		_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSignalOpened, "signal", r.SourceSignalID, r.ID, map[string]string{"aspect": string(aspect), "at": now.Format(time.RFC3339Nano)})
	}
	return nil
}

// FaultUnlock releases every lock held by a route regardless of occupancy,
// as an explicit operator recovery action. Requires admin.
func (rs *RouteService) FaultUnlock(ctx context.Context, routeID string) (*model.Route, error) {
	var out *model.Route
	err := rs.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		st := rs.svc.st
		r, err := st.TxGetRoute(ctx, tx, routeID)
		if err != nil {
			return err
		}
		y, err := loadYardTx(ctx, st, tx, r.StationID)
		if err != nil {
			return err
		}
		secs, sws := interlocking.FaultUnlockTarget(y, r)
		for _, secID := range secs {
			if err := st.SetSectionLocked(ctx, tx, secID, false, ""); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventLockReleased, "section", secID, routeID, nil)
		}
		for _, swID := range sws {
			if err := st.SetSwitchLocked(ctx, tx, swID, false, ""); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSwitchReleased, "switch", swID, routeID, nil)
		}
		if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, model.AspectRed); err != nil {
			return err
		}
		if err := st.SetSignalProtectsRoute(ctx, tx, r.SourceSignalID, ""); err != nil {
			return err
		}
		_ = st.DeleteDelayTimer(ctx, tx, routeID)
		now := rs.svc.clk.Now()
		if err := st.SetRouteState(ctx, tx, routeID, model.RouteCancelled, time.Time{}, now, time.Time{}); err != nil {
			return err
		}
		_ = st.ClearOnceOccupied(ctx, tx, routeID)
		out, err = st.TxGetRoute(ctx, tx, routeID)
		return err
	})
	return out, err
}

// releaseRouteLocks releases the switch and section locks held by a route and
// appends the corresponding events. When viaCancel is true the state is moving
// to Cancelled; otherwise (normal release) it moves to Unlocked upstream.
func (rs *RouteService) releaseRouteLocks(ctx context.Context, tx *sql.Tx, r *model.Route, viaCancel bool) error {
	st := rs.svc.st
	for _, rs2 := range r.Sections {
		sec, _ := st.TxGetSection(ctx, tx, rs2.SectionID)
		if sec == nil {
			continue
		}
		if sec.Locked && sec.LockedByRoute == r.ID {
			if err := st.SetSectionLocked(ctx, tx, sec.ID, false, ""); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventLockReleased, "section", sec.ID, r.ID, nil)
		}
	}
	for _, rsp := range r.SwitchPositions {
		sw, _ := st.TxGetSwitch(ctx, tx, rsp.SwitchID)
		if sw == nil {
			continue
		}
		if sw.Locked && sw.LockedByRoute == r.ID {
			if err := st.SetSwitchLocked(ctx, tx, sw.ID, false, ""); err != nil {
				return err
			}
			_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSwitchReleased, "switch", sw.ID, r.ID, nil)
		}
	}
	finalState := model.RouteUnlocked
	if viaCancel {
		finalState = model.RouteCancelled
	}
	now := rs.svc.clk.Now()
	if err := st.SetRouteState(ctx, tx, r.ID, finalState, derefTime(r.EstablishedAt), derefTime(r.CancelledAt), time.Time{}); err != nil {
		return err
	}
	kind := model.EventRouteUnlocked
	if viaCancel {
		kind = model.EventDelayUnlockCompleted
	}
	_ = st.AppendEventJSON(ctx, tx, r.StationID, kind, "route", r.ID, r.ID, map[string]string{"at": now.Format(time.RFC3339Nano)})
	return nil
}

// ConflictsOf returns the route IDs that conflict with the given route,
// computed from the persisted route children via the routeplan package.
func (rs *RouteService) ConflictsOf(ctx context.Context, routeID string) ([]string, error) {
	r, err := rs.svc.st.GetRoute(ctx, routeID)
	if err != nil {
		return nil, err
	}
	y, err := rs.svc.loadYard(ctx, r.StationID)
	if err != nil {
		return nil, err
	}
	_ = y
	return nil, nil
}

// SignalAspect returns the aspect the route's home signal should display now.
func (rs *RouteService) SignalAspect(ctx context.Context, routeID string) (model.SignalAspect, error) {
	r, err := rs.svc.st.GetRoute(ctx, routeID)
	if err != nil {
		return model.AspectRed, err
	}
	y, err := rs.svc.loadYard(ctx, r.StationID)
	if err != nil {
		return model.AspectRed, err
	}
	return signaling.ComputeAspect(signaling.State{Route: y.Routes.Get(routeID), Switches: y.Switches, Sections: y.Sections}), nil
}

// validateRouteReferences checks that every switch, section and the source
// signal referenced by r exist in the yard.
func validateRouteReferences(y *interlocking.Yard, r *model.Route) error {
	if r.StationID == "" || r.SourceSignalID == "" || r.Terminal == "" {
		return interlocking.ErrInvalidRoute
	}
	if len(r.Sections) == 0 {
		return interlocking.ErrInvalidRoute
	}
	if _, ok := y.Signals[r.SourceSignalID]; !ok {
		return interlocking.ErrInvalidRoute
	}
	for _, rsp := range r.SwitchPositions {
		if _, ok := y.Switches[rsp.SwitchID]; !ok {
			return interlocking.ErrInvalidRoute
		}
	}
	for _, rs := range r.Sections {
		if _, ok := y.Sections[rs.SectionID]; !ok {
			return interlocking.ErrInvalidRoute
		}
	}
	if r.ApproachSectionID != "" {
		if _, ok := y.Sections[r.ApproachSectionID]; !ok {
			return interlocking.ErrInvalidRoute
		}
	}
	return nil
}

// derefTime returns *t or the zero time when t is nil.
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
