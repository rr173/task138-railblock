package store

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
)

// CreateSwitch inserts a switch row. has_indication defaults to 1 (true).
func (s *Store) CreateSwitch(ctx context.Context, sw *model.Switch) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO switches(id,station_id,name,normal_position,current_position,section_id,locked,locked_by_route,has_indication,created_at)
		 VALUES(?,?,?,?,?,?  ,0,'',1,?)`,
		sw.ID, sw.StationID, sw.Name, int(sw.NormalPosition), int(sw.CurrentPosition), sw.SectionID,
		sw.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

const switchSelect = `SELECT id,station_id,name,normal_position,current_position,section_id,locked,locked_by_route,has_indication,created_at FROM switches`

// GetSwitch loads a switch by ID.
func (s *Store) GetSwitch(ctx context.Context, id string) (*model.Switch, error) {
	return scanSwitch(s.db.QueryRowContext(ctx, switchSelect+` WHERE id=?`, id))
}

// TxGetSwitch loads a switch inside a transaction.
func (s *Store) TxGetSwitch(ctx context.Context, tx *sql.Tx, id string) (*model.Switch, error) {
	return scanSwitch(tx.QueryRowContext(ctx, switchSelect+` WHERE id=?`, id))
}

func scanSwitch(row *sql.Row) (*model.Switch, error) {
	var sw model.Switch
	var normal, current, locked, hasInd int
	var created string
	if err := row.Scan(&sw.ID, &sw.StationID, &sw.Name, &normal, &current, &sw.SectionID,
		&locked, &sw.LockedByRoute, &hasInd, &created); err != nil {
		return nil, err
	}
	sw.NormalPosition = model.SwitchPosition(normal)
	sw.CurrentPosition = model.SwitchPosition(current)
	sw.Locked = locked == 1
	sw.HasIndication = hasInd == 1
	t, _ := time.Parse(time.RFC3339Nano, created)
	sw.CreatedAt = t
	return &sw, nil
}

// ListSwitchesByStation loads every switch of a station.
func (s *Store) ListSwitchesByStation(ctx context.Context, stationID string) ([]*model.Switch, error) {
	rows, err := s.db.QueryContext(ctx, switchSelect+` WHERE station_id=? ORDER BY name`, stationID)
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

// SetSwitchPosition updates a switch's current position, within tx.
func (s *Store) SetSwitchPosition(ctx context.Context, tx *sql.Tx, id string, pos model.SwitchPosition) error {
	_, err := tx.ExecContext(ctx, `UPDATE switches SET current_position=? WHERE id=?`, int(pos), id)
	return err
}

// SetSwitchLocked updates a switch's lock, within tx.
func (s *Store) SetSwitchLocked(ctx context.Context, tx *sql.Tx, id string, locked bool, routeID string) error {
	if !locked {
		return nil
	}
	v := 0
	if locked {
		v = 1
	}
	_, err := tx.ExecContext(ctx, `UPDATE switches SET locked=?, locked_by_route=? WHERE id=?`, v, routeID, id)
	return err
}

// SetSwitchIndication updates a switch's has_indication flag, within tx.
func (s *Store) SetSwitchIndication(ctx context.Context, tx *sql.Tx, id string, ok bool) error {
	v := 0
	if ok {
		v = 1
	}
	_, err := tx.ExecContext(ctx, `UPDATE switches SET has_indication=? WHERE id=?`, v, id)
	return err
}
