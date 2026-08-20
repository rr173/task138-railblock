package service

import (
	"context"
	"database/sql"
	"time"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/model"
	"task138-railblock/internal/routeplan"
	"task138-railblock/internal/store"
)

// loadYard assembles a read-only interlocking.Yard for a station from the
// store, using the bare connection (outside a transaction). It is the bridge
// between persistence and the pure rule functions.
func loadYard(ctx context.Context, st *store.Store, stationID string) (*interlocking.Yard, error) {
	switches, err := st.ListSwitchesByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	signals, err := st.ListSignalsByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	sections, err := st.ListSectionsByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	routes, err := st.ListRoutesByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	trains, err := st.ListTrainsByStation(ctx, stationID)
	if err != nil {
		return nil, err
	}
	tbl, once, err := buildTableAndOnce(ctx, st, routes)
	if err != nil {
		return nil, err
	}
	return assembleYard(stationID, switches, signals, sections, trains, tbl, once), nil
}

// loadYardTx is the in-tx variant: reads go through tx so it sees uncommitted
// writes and never reaches for the pooled connection the tx holds (which would
// deadlock under SetMaxOpenConns(1)).
func loadYardTx(ctx context.Context, st *store.Store, tx *sql.Tx, stationID string) (*interlocking.Yard, error) {
	switches, err := st.ListSwitchesByStationTx(ctx, tx, stationID)
	if err != nil {
		return nil, err
	}
	signals, err := st.ListSignalsByStationTx(ctx, tx, stationID)
	if err != nil {
		return nil, err
	}
	sections, err := st.ListSectionsByStationTx(ctx, tx, stationID)
	if err != nil {
		return nil, err
	}
	routes, err := st.ListRoutesByStationTx(ctx, tx, stationID)
	if err != nil {
		return nil, err
	}
	trains, err := st.ListTrainsByStationTx(ctx, tx, stationID)
	if err != nil {
		return nil, err
	}
	tbl, once, err := buildTableAndOnceTx(ctx, st, tx, routes)
	if err != nil {
		return nil, err
	}
	return assembleYard(stationID, switches, signals, sections, trains, tbl, once), nil
}

// assembleYard packs loaded slices into a Yard snapshot.
func assembleYard(stationID string, switches []*model.Switch, signals []*model.Signal, sections []*model.Section, trains []*model.Train, tbl *routeplan.Table, once map[string]map[string]bool) *interlocking.Yard {
	swMap := make(map[string]*model.Switch, len(switches))
	for _, sw := range switches {
		swMap[sw.ID] = sw
	}
	sigMap := make(map[string]*model.Signal, len(signals))
	for _, sig := range signals {
		sigMap[sig.ID] = sig
	}
	secMap := make(map[string]*model.Section, len(sections))
	for _, sec := range sections {
		secMap[sec.ID] = sec
	}
	trainMap := make(map[string]*model.Train, len(trains))
	for _, tr := range trains {
		trainMap[tr.ID] = tr
	}
	return &interlocking.Yard{
		StationID:           stationID,
		Switches:            swMap,
		Signals:             sigMap,
		Sections:            secMap,
		Trains:              trainMap,
		Routes:              tbl,
		SectionOnceOccupied: once,
	}
}

// buildTableAndOnce builds the route table from a list of routes (loading each
// route's children) and the once-occupied map for active routes.
func buildTableAndOnce(ctx context.Context, st *store.Store, routes []*model.Route) (*routeplan.Table, map[string]map[string]bool, error) {
	tbl := routeplan.NewTable()
	for _, r := range routes {
		full, err := st.GetRoute(ctx, r.ID)
		if err != nil {
			return nil, nil, err
		}
		tbl.Add(full)
	}
	once := map[string]map[string]bool{}
	for _, r := range tbl.All() {
		if !r.State.IsLocked() && r.State != model.RouteCancelling {
			continue
		}
		m, err := st.OnceOccupiedSections(ctx, r.ID)
		if err != nil {
			return nil, nil, err
		}
		once[r.ID] = m
	}
	return tbl, once, nil
}

// buildTableAndOnceTx is the in-tx variant.
func buildTableAndOnceTx(ctx context.Context, st *store.Store, tx *sql.Tx, routes []*model.Route) (*routeplan.Table, map[string]map[string]bool, error) {
	tbl := routeplan.NewTable()
	for _, r := range routes {
		full, err := st.TxGetRoute(ctx, tx, r.ID)
		if err != nil {
			return nil, nil, err
		}
		tbl.Add(full)
	}
	once := map[string]map[string]bool{}
	for _, r := range tbl.All() {
		if !r.State.IsLocked() && r.State != model.RouteCancelling {
			continue
		}
		m, err := st.OnceOccupiedSectionsTx(ctx, tx, r.ID)
		if err != nil {
			return nil, nil, err
		}
		once[r.ID] = m
	}
	return tbl, once, nil
}

// now returns the service clock's current time.
func now(clk clock.Clock) time.Time { return clk.Now() }
