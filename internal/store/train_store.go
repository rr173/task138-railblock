package store

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/model"
)

// CreateTrackBay inserts a track-bay row.
func (s *Store) CreateTrackBay(ctx context.Context, bay *model.TrackBay) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO track_bays(id,station_id,name,section_id) VALUES(?,?,?,?)`,
		bay.ID, bay.StationID, bay.Name, bay.SectionID)
	return err
}

// ListTrackBaysByStation loads every track bay of a station.
func (s *Store) ListTrackBaysByStation(ctx context.Context, stationID string) ([]*model.TrackBay, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,station_id,name,section_id FROM track_bays WHERE station_id=? ORDER BY name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.TrackBay
	for rows.Next() {
		var b model.TrackBay
		if err := rows.Scan(&b.ID, &b.StationID, &b.Name, &b.SectionID); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

// CreateTrain inserts a train row.
func (s *Store) CreateTrain(ctx context.Context, tr *model.Train) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trains(id,code,station_id,position_section,created_at) VALUES(?,?,?,?,?)`,
		tr.ID, tr.Code, tr.StationID, tr.PositionSection, tr.CreatedAt.Format(time.RFC3339Nano))
	return err
}

// GetTrain loads a train by ID.
func (s *Store) GetTrain(ctx context.Context, id string) (*model.Train, error) {
	var tr model.Train
	var created string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,code,station_id,position_section,created_at FROM trains WHERE id=?`, id).
		Scan(&tr.ID, &tr.Code, &tr.StationID, &tr.PositionSection, &created)
	if err != nil {
		return nil, err
	}
	t, _ := time.Parse(time.RFC3339Nano, created)
	tr.CreatedAt = t
	return &tr, nil
}

// SetTrainPosition updates a train's current section, within tx.
func (s *Store) SetTrainPosition(ctx context.Context, tx *sql.Tx, id, sectionID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE trains SET position_section=? WHERE id=?`, sectionID, id)
	return err
}

// ListTrainsByStation loads every train of a station.
func (s *Store) ListTrainsByStation(ctx context.Context, stationID string) ([]*model.Train, error) {
	rows, err := s.db.QueryContext(ctx,
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

// CreateBlockSection inserts a block-section row.
func (s *Store) CreateBlockSection(ctx context.Context, b *model.BlockSection) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO block_sections(id,station_id,name,adjacent_signal,occupied) VALUES(?,?,?,?,0)`,
		b.ID, b.StationID, b.Name, b.AdjacentSignal)
	return err
}

// SetBlockSectionOccupied updates a block section's occupancy, within tx.
func (s *Store) SetBlockSectionOccupied(ctx context.Context, tx *sql.Tx, id string, occupied bool) error {
	v := 0
	if occupied {
		v = 1
	}
	_, err := tx.ExecContext(ctx, `UPDATE block_sections SET occupied=? WHERE id=?`, v, id)
	return err
}

// ListBlockSectionsByStation loads every block section of a station.
func (s *Store) ListBlockSectionsByStation(ctx context.Context, stationID string) ([]*model.BlockSection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,station_id,name,adjacent_signal,occupied FROM block_sections WHERE station_id=? ORDER BY name`, stationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.BlockSection
	for rows.Next() {
		var b model.BlockSection
		var occ int
		if err := rows.Scan(&b.ID, &b.StationID, &b.Name, &b.AdjacentSignal, &occ); err != nil {
			return nil, err
		}
		b.Occupied = occ == 1
		out = append(out, &b)
	}
	return out, rows.Err()
}
