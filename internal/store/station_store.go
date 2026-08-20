package store

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
)

// stationStore is the persistence of stations. A station is a thin row; the
// yard elements (switches/signals/sections) are loaded by the other stores.
type stationStore struct{ s *Store }

// CreateStation inserts a station row.
func (s *Store) CreateStation(ctx context.Context, st *model.Station) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO stations(id,code,name,created_at) VALUES(?,?,?,?)`,
		st.ID, st.Code, st.Name, st.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

// GetStation loads a station by ID.
func (s *Store) GetStation(ctx context.Context, id string) (*model.Station, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,code,name,created_at FROM stations WHERE id=?`, id)
	var st model.Station
	var created string
	if err := row.Scan(&st.ID, &st.Code, &st.Name, &created); err != nil {
		return nil, err
	}
	t, _ := time.Parse(time.RFC3339Nano, created)
	st.CreatedAt = t
	return &st, nil
}

// ListStations loads every station.
func (s *Store) ListStations(ctx context.Context) ([]*model.Station, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,code,name,created_at FROM stations ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Station
	for rows.Next() {
		var st model.Station
		var created string
		if err := rows.Scan(&st.ID, &st.Code, &st.Name, &created); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339Nano, created)
		st.CreatedAt = t
		out = append(out, &st)
	}
	return out, rows.Err()
}

// StationExists reports whether a station row exists.
func (s *Store) StationExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM stations WHERE id=?`, id).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
