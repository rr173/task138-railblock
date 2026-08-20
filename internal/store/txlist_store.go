package store

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
)

// txListSwitchesByStation loads a station's switches through the given DBTX
// (the transaction), so an in-tx caller does not reach for s.db and deadlock
// under SetMaxOpenConns(1).
func txListSwitchesByStation(ctx context.Context, q DBTX, stationID string) ([]*model.Switch, error) {
	rows, err := q.QueryContext(ctx, switchSelect+` WHERE station_id=? ORDER BY name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Switch
	for rows.Next() {
		var sw model.Switch
		var normal, current, locked, hasInd int
		var created string
		if err := rows.Scan(&sw.ID, &sw.StationID, &sw.Name, &normal, &current, &sw.SectionID,
			&locked, &sw.LockedByRoute, &hasInd, &created); err != nil {
			return nil, err
		}
		sw.NormalPosition = model.SwitchPosition(normal)
		sw.CurrentPosition = model.SwitchPosition(current)
		sw.Locked = locked == 1
		sw.HasIndication = hasInd == 1
		t, _ := time.Parse(time.RFC3339Nano, created)
		sw.CreatedAt = t
		out = append(out, &sw)
	}
	return out, rows.Err()
}

// txListSignalsByStation is the tx-aware variant of ListSignalsByStation.
func txListSignalsByStation(ctx context.Context, q DBTX, stationID string) ([]*model.Signal, error) {
	rows, err := q.QueryContext(ctx, signalSelect+` WHERE station_id=? ORDER BY name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Signal
	for rows.Next() {
		var sig model.Signal
		var home int
		var created string
		if err := rows.Scan(&sig.ID, &sig.StationID, &sig.Name, &sig.Direction,
			&sig.Aspect, &home, &sig.ProtectsRouteID, &created); err != nil {
			return nil, err
		}
		sig.Home = home == 1
		t, _ := time.Parse(time.RFC3339Nano, created)
		sig.CreatedAt = t
		out = append(out, &sig)
	}
	return out, rows.Err()
}

// txListSectionsByStation is the tx-aware variant of ListSectionsByStation.
func txListSectionsByStation(ctx context.Context, q DBTX, stationID string) ([]*model.Section, error) {
	rows, err := q.QueryContext(ctx, sectionSelect+` WHERE station_id=? ORDER BY name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSectionsRows(ctx, rows)
}

// txListRoutesByStation is the tx-aware variant of ListRoutesByStation.
func txListRoutesByStation(ctx context.Context, q DBTX, stationID string) ([]*model.Route, error) {
	rows, err := q.QueryContext(ctx, routeSelect+` WHERE station_id=? ORDER BY code`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoutes(rows)
}

// txListTrainsByStation is the tx-aware variant of ListTrainsByStation.
func txListTrainsByStation(ctx context.Context, q DBTX, stationID string) ([]*model.Train, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id,code,station_id,position_section,created_at FROM trains WHERE station_id=? ORDER BY code`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Train
	for rows.Next() {
		var tr model.Train
		var created string
		if err := rows.Scan(&tr.ID, &tr.Code, &tr.StationID, &tr.PositionSection, &created); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339Nano, created)
		tr.CreatedAt = t
		out = append(out, &tr)
	}
	return out, rows.Err()
}

// scanSectionsRows is shared by scanSections and the tx list helper.
func scanSectionsRows(ctx context.Context, rows *sql.Rows) ([]*model.Section, error) {
	var out []*model.Section
	for rows.Next() {
		var sec model.Section
		var created string
		if err := rows.Scan(&sec.ID, &sec.StationID, &sec.Name, &sec.Kind,
			&sec.Occupied, &sec.OccupiedByTrain, &sec.Locked, &sec.LockedByRoute, &created); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339Nano, created)
		sec.CreatedAt = t
		out = append(out, &sec)
	}
	return out, rows.Err()
}

// ListSwitchesByStationTx is the exported tx-aware variant.
func (s *Store) ListSwitchesByStationTx(ctx context.Context, tx *sql.Tx, stationID string) ([]*model.Switch, error) {
	return txListSwitchesByStation(ctx, tx, stationID)
}

// ListSignalsByStationTx is the exported tx-aware variant.
func (s *Store) ListSignalsByStationTx(ctx context.Context, tx *sql.Tx, stationID string) ([]*model.Signal, error) {
	return txListSignalsByStation(ctx, tx, stationID)
}

// ListSectionsByStationTx is the exported tx-aware variant.
func (s *Store) ListSectionsByStationTx(ctx context.Context, tx *sql.Tx, stationID string) ([]*model.Section, error) {
	return txListSectionsByStation(ctx, tx, stationID)
}

// ListRoutesByStationTx is the exported tx-aware variant.
func (s *Store) ListRoutesByStationTx(ctx context.Context, tx *sql.Tx, stationID string) ([]*model.Route, error) {
	return txListRoutesByStation(ctx, tx, stationID)
}

// ListTrainsByStationTx is the exported tx-aware variant.
func (s *Store) ListTrainsByStationTx(ctx context.Context, tx *sql.Tx, stationID string) ([]*model.Train, error) {
	return txListTrainsByStation(ctx, tx, stationID)
}
