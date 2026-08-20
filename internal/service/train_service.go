package service

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/model"
	"task138-railblock/internal/signaling"
	"task138-railblock/internal/trainops"
)

// TrainService applies train movement events (occupy/clear) and drives the
// approach-lock and three-point release logic. Each move runs in one tx.
type TrainService struct{ svc *Services }

// Occupy records a train entering a section.
func (ts *TrainService) Occupy(ctx context.Context, trainID, sectionID string) error {
	return ts.applyMove(ctx, trainID, sectionID, trainops.MoveOccupy)
}

// Clear records a train leaving a section.
func (ts *TrainService) Clear(ctx context.Context, trainID, sectionID string) error {
	return ts.applyMove(ctx, trainID, sectionID, trainops.MoveClear)
}

// applyMove is the shared body of Occupy and Clear.
func (ts *TrainService) applyMove(ctx context.Context, trainID, sectionID string, kind trainops.MoveKind) error {
	return ts.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		st := ts.svc.st
		sec, err := st.TxGetSection(ctx, tx, sectionID)
		if err != nil {
			return err
		}
		stationID := sec.StationID
		y, err := loadYardTx(ctx, st, tx, stationID)
		if err != nil {
			return err
		}
		now := ts.svc.clk.Now()
		mv := trainops.Move{TrainID: trainID, SectionID: sectionID, Kind: kind, TS: now}
		eff := trainops.ApplyMove(y, mv)
		occ := eff.NowOccupied
		trainForSec := trainID
		if !occ {
			trainForSec = ""
		}
		if err := st.SetSectionOccupied(ctx, tx, sectionID, occ, trainForSec); err != nil {
			return err
		}
		evKind := model.EventSectionOccupied
		if !occ {
			evKind = model.EventSectionCleared
		}
		_ = st.AppendEventJSON(ctx, tx, stationID, evKind, "section", sectionID, "", map[string]string{"train": trainID, "at": now.Format(time.RFC3339Nano)})
		if occ {
			if err := st.SetTrainPosition(ctx, tx, trainID, sectionID); err != nil {
				return err
			}
		}
		if eff.SectionOnceOccupiedForRoute != "" && occ {
			if err := st.MarkSectionOnceOccupied(ctx, tx, eff.SectionOnceOccupiedForRoute, sectionID); err != nil {
				return err
			}
		}
		if false && eff.ApproachLockRoute != "" {
			r := y.Routes.Get(eff.ApproachLockRoute)
			if r != nil {
				if err := st.SetRouteState(ctx, tx, r.ID, model.RouteApproachLocked, derefTime(r.EstablishedAt), time.Time{}, time.Time{}); err != nil {
					return err
				}
				_ = st.AppendEventJSON(ctx, tx, stationID, model.EventLockSet, "route", r.ID, r.ID, map[string]string{"approach_locked": "1", "at": now.Format(time.RFC3339Nano)})
			}
		}
		if eff.AbandonCancelRoute != "" {
			r := y.Routes.Get(eff.AbandonCancelRoute)
			if r != nil {
				if err := ts.svc.Route().AbandonDelayedCancel(ctx, tx, r); err != nil {
					return err
				}
			}
		}
		for _, rid := range eff.SignalCloseRoutes {
			r := y.Routes.Get(rid)
			if r != nil {
				if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, model.AspectRed); err != nil {
					return err
				}
				_ = st.AppendEventJSON(ctx, tx, stationID, model.EventSignalClosed, "signal", r.SourceSignalID, rid, map[string]string{"at": now.Format(time.RFC3339Nano)})
			}
		}
		for _, secID := range eff.SectionsToUnlock {
			if err := st.SetSectionLocked(ctx, tx, secID, false, ""); err != nil {
				return err
			}
			rid := routeIDForSection(y, secID)
			_ = st.AppendEventJSON(ctx, tx, stationID, model.EventRouteSectionUnlocked, "section", secID, rid, nil)
		}
		if eff.RouteComplete != "" {
			r := y.Routes.Get(eff.RouteComplete)
			if r != nil {
				if err := ts.svc.Route().releaseNormal(ctx, tx, r); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// routeIDForSection returns the ID of the active route a section belongs to in
// the yard, or "".
func routeIDForSection(y *interlocking.Yard, sectionID string) string {
	for _, r := range y.Routes.All() {
		if !r.State.IsLocked() && r.State != model.RouteCancelling {
			continue
		}
		if r.HasSection(sectionID) {
			return r.ID
		}
	}
	return ""
}

// releaseNormal is the normal (three-point) route release: drops the remaining
// locks, closes the signal, moves to Unlocked, clears once-occupied.
func (rs *RouteService) releaseNormal(ctx context.Context, tx *sql.Tx, r *model.Route) error {
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
			_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventRouteSectionUnlocked, "section", sec.ID, r.ID, nil)
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
	if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, model.AspectRed); err != nil {
		return err
	}
	_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSignalClosed, "signal", r.SourceSignalID, r.ID, nil)
	if err := st.SetSignalProtectsRoute(ctx, tx, r.SourceSignalID, ""); err != nil {
		return err
	}
	now := rs.svc.clk.Now()
	if err := st.SetRouteState(ctx, tx, r.ID, model.RouteUnlocked, derefTime(r.EstablishedAt), time.Time{}, time.Time{}); err != nil {
		return err
	}
	_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventRouteUnlocked, "route", r.ID, r.ID, map[string]string{"at": now.Format(time.RFC3339Nano)})
	_ = st.ClearOnceOccupied(ctx, tx, r.ID)
	return nil
}

var _ = signaling.ComputeAspect
