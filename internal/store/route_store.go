package store

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
)

// routeStore persists routes and their switch-position / section / conflict
// child rows. A route is created in the "pending" state; the service moves it
// through requested → established → (approach_locked | cancelling) →
// (unlocked | cancelled) and records every transition in the events table.

// CreateRoute inserts a route and its child rows in one transaction.
func (s *Store) CreateRoute(ctx context.Context, r *model.Route) error {
	return s.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO routes(id,station_id,code,kind,source_signal_id,terminal,approach_section_id,state,train_id,established_at,cancelled_at,unlock_timer_started_at,created_at)
			 VALUES(?,?,?,?,?,?,?,?,?  ,NULL,NULL,NULL,?)`,
			r.ID, r.StationID, r.Code, r.Kind, r.SourceSignalID, r.Terminal, r.ApproachSectionID,
			r.State, r.TrainID, r.CreatedAt.Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
		for _, rsp := range r.SwitchPositions {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO route_switch_positions(route_id,switch_id,required_position) VALUES(?,?,?)`,
				r.ID, rsp.SwitchID, int(rsp.RequiredPosition)); err != nil {
				return err
			}
		}
		for _, rs := range r.Sections {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO route_sections(route_id,section_id,seq) VALUES(?,?,?)`,
				r.ID, rs.SectionID, rs.Seq); err != nil {
				return err
			}
		}
		return nil
	})
}

const routeSelect = `SELECT id,station_id,code,kind,source_signal_id,terminal,approach_section_id,state,train_id,established_at,cancelled_at,unlock_timer_started_at,created_at FROM routes`

// GetRoute loads a route by ID, including its child rows.
func (s *Store) GetRoute(ctx context.Context, id string) (*model.Route, error) {
	r := &model.Route{}
	var established, cancelled, timer, created sql.NullString
	err := s.db.QueryRowContext(ctx, routeSelect+` WHERE id=?`, id).Scan(
		&r.ID, &r.StationID, &r.Code, &r.Kind, &r.SourceSignalID, &r.Terminal, &r.ApproachSectionID,
		&r.State, &r.TrainID, &established, &cancelled, &timer, &created,
	)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = parseRFC(created.String)
	r.EstablishedAt = parseRFCPtr(established)
	r.CancelledAt = parseRFCPtr(cancelled)
	r.UnlockTimerStartedAt = parseRFCPtr(timer)
	if err := s.loadRouteChildren(ctx, s.querier(), r); err != nil {
		return nil, err
	}
	return r, nil
}

// TxGetRoute loads a route inside a transaction.
func (s *Store) TxGetRoute(ctx context.Context, tx *sql.Tx, id string) (*model.Route, error) {
	r := &model.Route{}
	var established, cancelled, timer, created sql.NullString
	err := tx.QueryRowContext(ctx, routeSelect+` WHERE id=?`, id).Scan(
		&r.ID, &r.StationID, &r.Code, &r.Kind, &r.SourceSignalID, &r.Terminal, &r.ApproachSectionID,
		&r.State, &r.TrainID, &established, &cancelled, &timer, &created,
	)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = parseRFC(created.String)
	r.EstablishedAt = parseRFCPtr(established)
	r.CancelledAt = parseRFCPtr(cancelled)
	r.UnlockTimerStartedAt = parseRFCPtr(timer)
	if err := s.loadRouteChildren(ctx, tx, r); err != nil {
		return nil, err
	}
	return r, nil
}

// loadRouteChildren loads the switch positions, sections and conflicts for r.
func (s *Store) loadRouteChildren(ctx context.Context, q DBTX, r *model.Route) error {
	swRows, err := q.QueryContext(ctx,
		`SELECT switch_id,required_position FROM route_switch_positions WHERE route_id=?`, r.ID)
	if err != nil {
		return err
	}
	for swRows.Next() {
		var rsp model.RouteSwitchPosition
		var pos int
		if err := swRows.Scan(&rsp.SwitchID, &pos); err != nil {
			swRows.Close()
			return err
		}
		rsp.RequiredPosition = model.SwitchPosition(pos)
		r.SwitchPositions = append(r.SwitchPositions, rsp)
	}
	swRows.Close()

	secRows, err := q.QueryContext(ctx,
		`SELECT section_id,seq FROM route_sections WHERE route_id=? ORDER BY seq`, r.ID)
	if err != nil {
		return err
	}
	for secRows.Next() {
		var rs model.RouteSection
		if err := secRows.Scan(&rs.SectionID, &rs.Seq); err != nil {
			secRows.Close()
			return err
		}
		r.Sections = append(r.Sections, rs)
	}
	secRows.Close()
	return nil
}

// ListRoutesByStation loads every route of a station (without children).
func (s *Store) ListRoutesByStation(ctx context.Context, stationID string) ([]*model.Route, error) {
	rows, err := s.db.QueryContext(ctx, routeSelect+` WHERE station_id=? ORDER BY code`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoutes(rows)
}

// ListRoutesByState loads every route in a given state across all stations.
func (s *Store) ListRoutesByState(ctx context.Context, state model.RouteState) ([]*model.Route, error) {
	rows, err := s.db.QueryContext(ctx, routeSelect+` WHERE state=? ORDER BY id`, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoutes(rows)
}

// ListAllRoutes loads every route (without children).
func (s *Store) ListAllRoutes(ctx context.Context) ([]*model.Route, error) {
	rows, err := s.db.QueryContext(ctx, routeSelect+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRoutes(rows)
}

func scanRoutes(rows *sql.Rows) ([]*model.Route, error) {
	var out []*model.Route
	for rows.Next() {
		r := &model.Route{}
		var established, cancelled, timer, created sql.NullString
		if err := rows.Scan(
			&r.ID, &r.StationID, &r.Code, &r.Kind, &r.SourceSignalID, &r.Terminal, &r.ApproachSectionID,
			&r.State, &r.TrainID, &established, &cancelled, &timer, &created,
		); err != nil {
			return nil, err
		}
		r.CreatedAt = parseRFC(created.String)
		r.EstablishedAt = parseRFCPtr(established)
		r.CancelledAt = parseRFCPtr(cancelled)
		r.UnlockTimerStartedAt = parseRFCPtr(timer)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetRouteState updates a route's state and the relevant timestamp, within tx.
// Pass empty time.Time to leave a timestamp NULL.
func (s *Store) SetRouteState(ctx context.Context, tx *sql.Tx, id string, state model.RouteState, established, cancelled, timer time.Time) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE routes SET state=?, established_at=?, cancelled_at=?, unlock_timer_started_at=? WHERE id=?`,
		state, nullTime(established), nullTime(cancelled), nullTime(timer), id)
	return err
}

// SetRouteTrain binds a train to a route, within tx.
func (s *Store) SetRouteTrain(ctx context.Context, tx *sql.Tx, id, trainID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE routes SET train_id=? WHERE id=?`, trainID, id)
	return err
}

// parseRFC parses a stored RFC3339Nano timestamp.
func parseRFC(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	return t
}

// parseRFCPtr parses a nullable stored timestamp.
func parseRFCPtr(n sql.NullString) *time.Time {
	if !n.Valid || n.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, n.String)
	if err != nil {
		return nil
	}
	return &t
}

// nullTime returns NULL for the zero time, else an RFC3339Nano string.
func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Format(time.RFC3339Nano)
}

// MarkSectionOnceOccupied records that a route section has seen a train.
func (s *Store) MarkSectionOnceOccupied(ctx context.Context, tx *sql.Tx, routeID, sectionID string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO route_section_occupied(route_id,section_id,once) VALUES(?,?,1)
		 ON CONFLICT(route_id,section_id) DO UPDATE SET once=1`,
		routeID, sectionID)
	return err
}

// OnceOccupiedSections returns the section IDs of a route that have been
// occupied at least once since establishment.
func (s *Store) OnceOccupiedSections(ctx context.Context, routeID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT section_id FROM route_section_occupied WHERE route_id=? AND once=1`, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s] = true
	}
	return out, rows.Err()
}

// OnceOccupiedSectionsTx is the in-tx variant.
func (s *Store) OnceOccupiedSectionsTx(ctx context.Context, tx *sql.Tx, routeID string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT section_id FROM route_section_occupied WHERE route_id=? AND once=1`, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s] = true
	}
	return out, rows.Err()
}

// ClearOnceOccupied removes the once-occupied bookkeeping for a route, within
// tx. Called when a route releases to its terminal state.
func (s *Store) ClearOnceOccupied(ctx context.Context, tx *sql.Tx, routeID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM route_section_occupied WHERE route_id=?`, routeID)
	return err
}
