package service

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
	"task138-railblock/internal/signaling"
	"task138-railblock/internal/store"
)

// refreshSignal recomputes a route's home-signal aspect from the current yard
// state and persists it, appending signal_opened / signal_closed events. Used
// by Establish and SwitchService.SetIndication to keep the denormalized aspect
// column consistent with the interlocking state.
func refreshSignal(ctx context.Context, tx *sql.Tx, st *store.Store, r *model.Route) error {
	y, err := loadYardTx(ctx, st, tx, r.StationID)
	if err != nil {
		return err
	}
	cur := y.Signals[r.SourceSignalID]
	if cur == nil {
		return nil
	}
	aspect := signaling.ComputeAspect(signaling.State{Route: y.Routes.Get(r.ID), Switches: y.Switches, Sections: y.Sections})
	if aspect == cur.Aspect {
		return nil
	}
	now := time.Now().UTC()
	if err := st.SetSignalAspect(ctx, tx, r.SourceSignalID, aspect); err != nil {
		return err
	}
	if aspect == model.AspectRed {
		_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSignalClosed, "signal", r.SourceSignalID, r.ID, map[string]string{"at": now.Format(time.RFC3339Nano)})
	} else {
		_ = st.AppendEventJSON(ctx, tx, r.StationID, model.EventSignalOpened, "signal", r.SourceSignalID, r.ID, map[string]string{"aspect": string(aspect), "at": now.Format(time.RFC3339Nano)})
	}
	return nil
}
