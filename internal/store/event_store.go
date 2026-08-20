package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"task138-railblock/internal/model"
)

// AppendEvent inserts one row into the append-only event log. The event log is
// the source of truth for restart recovery.
func (s *Store) AppendEvent(ctx context.Context, tx *sql.Tx, e *model.Event) error {
	payload := e.Payload
	if payload == "" && e.Payload == "" {
		payload = ""
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO events(ts,station_id,kind,entity_type,entity_id,route_id,payload)
		 VALUES(?,?,?,?,?,?,?)`,
		e.TS.Format(time.RFC3339Nano), e.StationID, e.Kind, e.EntityType, e.EntityID, e.RouteID, payload)
	return err
}

// AppendEventJSON is AppendEvent that JSON-encodes a payload map.
func (s *Store) AppendEventJSON(ctx context.Context, tx *sql.Tx, stationID string, kind model.EventKind, entityType, entityID, routeID string, payload any) error {
	var p string
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		p = string(b)
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO events(ts,station_id,kind,entity_type,entity_id,route_id,payload)
		 VALUES(?,?,?,?,?,?,?)`,
		time.Now().Format(time.RFC3339Nano), stationID, kind, entityType, entityID, routeID, p)
	return err
}

// ListEventsByStation loads every event for a station in timestamp order.
func (s *Store) ListEventsByStation(ctx context.Context, stationID string) ([]*model.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,ts,station_id,kind,entity_type,entity_id,route_id,payload FROM events
		 WHERE station_id=? ORDER BY ts, id`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ListAllEvents loads every event in timestamp order across all stations.
func (s *Store) ListAllEvents(ctx context.Context) ([]*model.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,ts,station_id,kind,entity_type,entity_id,route_id,payload FROM events
		 ORDER BY ts, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows *sql.Rows) ([]*model.Event, error) {
	var out []*model.Event
	for rows.Next() {
		var e model.Event
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.StationID, &e.Kind, &e.EntityType, &e.EntityID, &e.RouteID, &e.Payload); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, &e)
	}
	return out, rows.Err()
}

// CreateDelayTimer inserts a delay-unlock timer row, within tx.
func (s *Store) CreateDelayTimer(ctx context.Context, tx *sql.Tx, routeID, stationID string, kind model.RouteKind, startedAt time.Time, delay time.Duration) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO delay_unlock_timers(route_id,station_id,kind,started_at,delay_secs,completed)
		 VALUES(?,?,?,?,?,0)
		 ON CONFLICT(route_id) DO UPDATE SET station_id=excluded.station_id, kind=excluded.kind, started_at=excluded.started_at, delay_secs=excluded.delay_secs, completed=0`,
		routeID, stationID, kind, startedAt.Format(time.RFC3339Nano), int(delay.Seconds()))
	return err
}

// CompleteDelayTimer marks a delay timer completed, within tx.
func (s *Store) CompleteDelayTimer(ctx context.Context, tx *sql.Tx, routeID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE delay_unlock_timers SET completed=1 WHERE route_id=?`, routeID)
	return err
}

// DeleteDelayTimer removes a delay timer, within tx. Called when a timed
// cancel is abandoned (train entered the route).
func (s *Store) DeleteDelayTimer(ctx context.Context, tx *sql.Tx, routeID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM delay_unlock_timers WHERE route_id=?`, routeID)
	return err
}

// GetDelayTimer loads a delay timer by route ID.
func (s *Store) GetDelayTimer(ctx context.Context, routeID string) (startedAt time.Time, delay time.Duration, completed bool, err error) {
	var started string
	var delaySecs int
	var comp int
	err = s.db.QueryRowContext(ctx,
		`SELECT started_at,delay_secs,completed FROM delay_unlock_timers WHERE route_id=?`, routeID).
		Scan(&started, &delaySecs, &comp)
	if err != nil {
		return time.Time{}, 0, false, err
	}
	startedAt, _ = time.Parse(time.RFC3339Nano, started)
	delay = time.Duration(delaySecs) * time.Second
	completed = comp == 1
	return startedAt, delay, completed, nil
}

// ListPendingDelayTimers returns all in-flight (not completed) delay timers.
func (s *Store) ListPendingDelayTimers(ctx context.Context) (routeIDs []string, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT route_id FROM delay_unlock_timers WHERE completed=0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		routeIDs = append(routeIDs, s)
	}
	return routeIDs, rows.Err()
}
