package store

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
)

// CreateSection inserts a track section row. The store is the only writer to
// sections; occupancy/lock columns are mutated by the service layer inside a
// transaction via the Tx* variants.
func (s *Store) CreateSection(ctx context.Context, sec *model.Section) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sections(id,station_id,name,kind,occupied,occupied_by_train,locked,locked_by_route,created_at)
		 VALUES(?,?,?,?,0,'',0,'',?)`,
		sec.ID, sec.StationID, sec.Name, sec.Kind, sec.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

// GetSection loads a section by ID, outside a transaction.
func (s *Store) GetSection(ctx context.Context, id string) (*model.Section, error) {
	return scanSection(s.db.QueryRowContext(ctx, sectionSelect+` WHERE id=?`, id))
}

// TxGetSection loads a section by ID using the given transaction. Use this
// inside InTx: with SetMaxOpenConns(1) calling GetSection (which uses s.db)
// while a tx holds the single connection deadlocks.
func (s *Store) TxGetSection(ctx context.Context, tx *sql.Tx, id string) (*model.Section, error) {
	return scanSection(tx.QueryRowContext(ctx, sectionSelect+` WHERE id=?`, id))
}

const sectionSelect = `SELECT id,station_id,name,kind,occupied,occupied_by_train,locked,locked_by_route,created_at FROM sections`

func scanSection(row *sql.Row) (*model.Section, error) {
	var sec model.Section
	var created string
	if err := row.Scan(&sec.ID, &sec.StationID, &sec.Name, &sec.Kind,
		&sec.Occupied, &sec.OccupiedByTrain, &sec.Locked, &sec.LockedByRoute, &created); err != nil {
		return nil, err
	}
	t, _ := time.Parse(time.RFC3339Nano, created)
	sec.CreatedAt = t
	return &sec, nil
}

// ListSectionsByStation loads every section of a station.
func (s *Store) ListSectionsByStation(ctx context.Context, stationID string) ([]*model.Section, error) {
	rows, err := s.db.QueryContext(ctx, sectionSelect+` WHERE station_id=? ORDER BY name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSections(rows)
}

// ListSections loads every section.
func (s *Store) ListSections(ctx context.Context) ([]*model.Section, error) {
	rows, err := s.db.QueryContext(ctx, sectionSelect+` ORDER BY station_id, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSections(rows)
}

func scanSections(rows *sql.Rows) ([]*model.Section, error) {
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

// SetSectionOccupied updates a section's occupancy, within tx. It records the
// occupying train id (cleared when occupied=false).
func (s *Store) SetSectionOccupied(ctx context.Context, tx *sql.Tx, id string, occupied bool, trainID string) error {
	v := 0
	if occupied {
		v = 1
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE sections SET occupied=?, occupied_by_train=? WHERE id=?`, v, trainID, id)
	return err
}

// SetSectionLocked updates a section's lock, within tx.
func (s *Store) SetSectionLocked(ctx context.Context, tx *sql.Tx, id string, locked bool, routeID string) error {
	v := 0
	if locked {
		v = 1
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE sections SET locked=?, locked_by_route=? WHERE id=?`, v, routeID, id)
	return err
}
