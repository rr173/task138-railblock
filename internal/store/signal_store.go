package store

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
)

// CreateSignal inserts a signal row with default aspect Red.
func (s *Store) CreateSignal(ctx context.Context, sig *model.Signal) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO signals(id,station_id,name,direction,aspect,home,protects_route_id,created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		sig.ID, sig.StationID, sig.Name, sig.Direction, sig.Aspect, boolToInt(sig.Home), sig.ProtectsRouteID,
		sig.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

const signalSelect = `SELECT id,station_id,name,direction,aspect,home,protects_route_id,created_at FROM signals`

// GetSignal loads a signal by ID.
func (s *Store) GetSignal(ctx context.Context, id string) (*model.Signal, error) {
	return scanSignal(s.db.QueryRowContext(ctx, signalSelect+` WHERE id=?`, id))
}

// TxGetSignal loads a signal inside a transaction.
func (s *Store) TxGetSignal(ctx context.Context, tx *sql.Tx, id string) (*model.Signal, error) {
	return scanSignal(tx.QueryRowContext(ctx, signalSelect+` WHERE id=?`, id))
}

func scanSignal(row *sql.Row) (*model.Signal, error) {
	var sig model.Signal
	var home int
	var created string
	if err := row.Scan(&sig.ID, &sig.StationID, &sig.Name, &sig.Direction,
		&sig.Aspect, &home, &sig.ProtectsRouteID, &created); err != nil {
		return nil, err
	}
	sig.Home = home == 1
	t, _ := time.Parse(time.RFC3339Nano, created)
	sig.CreatedAt = t
	return &sig, nil
}

// ListSignalsByStation loads every signal of a station.
func (s *Store) ListSignalsByStation(ctx context.Context, stationID string) ([]*model.Signal, error) {
	rows, err := s.db.QueryContext(ctx, signalSelect+` WHERE station_id=? ORDER BY name`, stationID)
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

// SetSignalAspect updates a signal's aspect, within tx. The service computes
// the aspect via signaling.ComputeAspect and persists the result here; the
// column is a denormalized recompute, never a free input.
func (s *Store) SetSignalAspect(ctx context.Context, tx *sql.Tx, id string, aspect model.SignalAspect) error {
	_, err := tx.ExecContext(ctx, `UPDATE signals SET aspect=? WHERE id=?`, aspect, id)
	return err
}

// SetSignalProtectsRoute updates which route a signal protects, within tx.
func (s *Store) SetSignalProtectsRoute(ctx context.Context, tx *sql.Tx, id, routeID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE signals SET protects_route_id=? WHERE id=?`, routeID, id)
	return err
}

// boolToInt is a tiny helper to map bool to the SQLite INTEGER convention.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
