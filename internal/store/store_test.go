package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"task138-railblock/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// TestCreateAndGetStation round-trips a station row.
func TestCreateAndGetStation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	in := &model.Station{ID: "ST-1", Code: "XN", Name: "虹桥", CreatedAt: time.Now().UTC()}
	if err := st.CreateStation(ctx, in); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := st.GetStation(ctx, "ST-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Code != "XN" || got.Name != "虹桥" {
		t.Fatalf("round-trip: %+v", got)
	}
}

// TestStationExists covers the existence check.
func TestStationExists(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	exists, err := st.StationExists(ctx, "nope")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Fatal("absent station should not exist")
	}
	_ = st.CreateStation(ctx, &model.Station{ID: "ST-2", Code: "BJ", Name: "北京", CreatedAt: time.Now().UTC()})
	exists, _ = st.StationExists(ctx, "ST-2")
	if !exists {
		t.Fatal("created station should exist")
	}
}

// TestCreateAndGetSection round-trips a section.
func TestCreateAndGetSection(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_ = st.CreateStation(ctx, &model.Station{ID: "ST-9", Code: "C", Name: "C", CreatedAt: time.Now().UTC()})
	sec := &model.Section{ID: "SC-1", StationID: "ST-9", Name: "1DG", Kind: model.SectionSwitch, CreatedAt: time.Now().UTC()}
	if err := st.CreateSection(ctx, sec); err != nil {
		t.Fatalf("create section: %v", err)
	}
	got, err := st.GetSection(ctx, "SC-1")
	if err != nil {
		t.Fatalf("get section: %v", err)
	}
	if got.Kind != model.SectionSwitch || got.Occupied {
		t.Fatalf("section round-trip: %+v", got)
	}
}

// TestSetSectionOccupiedAndLocked exercises the in-tx mutators.
func TestSetSectionOccupiedAndLocked(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_ = st.CreateStation(ctx, &model.Station{ID: "ST-7", Code: "Z", Name: "Z", CreatedAt: time.Now().UTC()})
	_ = st.CreateSection(ctx, &model.Section{ID: "SC-7", StationID: "ST-7", Name: "7G", Kind: model.SectionPlain, CreatedAt: time.Now().UTC()})
	err := st.InTx(ctx, func(tx *sql.Tx) error {
		if e := st.SetSectionOccupied(ctx, tx, "SC-7", true, "TR1"); e != nil {
			return e
		}
		return st.SetSectionLocked(ctx, tx, "SC-7", true, "RT-7")
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	got, err := st.GetSection(ctx, "SC-7")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Occupied || got.OccupiedByTrain != "TR1" || !got.Locked || got.LockedByRoute != "RT-7" {
		t.Fatalf("after mutate: %+v", got)
	}
}

// TestCreateRouteWithChildren round-trips a route and its child rows.
func TestCreateRouteWithChildren(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_ = st.CreateStation(ctx, &model.Station{ID: "ST-3", Code: "X", Name: "X", CreatedAt: time.Now().UTC()})
	_ = st.CreateSection(ctx, &model.Section{ID: "SC-30", StationID: "ST-3", Name: "30G", Kind: model.SectionPlain, CreatedAt: time.Now().UTC()})
	_ = st.CreateSwitch(ctx, &model.Switch{ID: "SW-30", StationID: "ST-3", Name: "30#", NormalPosition: model.PositionNormal, CurrentPosition: model.PositionNormal, SectionID: "SC-30", HasIndication: true, CreatedAt: time.Now().UTC()})
	_ = st.CreateSignal(ctx, &model.Signal{ID: "SG-30", StationID: "ST-3", Name: "S30", Direction: model.DirectionArrival, Aspect: model.AspectRed, Home: true, CreatedAt: time.Now().UTC()})
	rt := &model.Route{
		ID: "RT-30", StationID: "ST-3", Code: "X-30G", Kind: model.RouteKindTrain,
		SourceSignalID: "SG-30", Terminal: "SC-30", State: model.RoutePending, CreatedAt: time.Now().UTC(),
		SwitchPositions: []model.RouteSwitchPosition{{SwitchID: "SW-30", RequiredPosition: model.PositionNormal}},
		Sections:        []model.RouteSection{{SectionID: "SC-30", Seq: 0}},
	}
	if err := st.CreateRoute(ctx, rt); err != nil {
		t.Fatalf("create route: %v", err)
	}
	got, err := st.GetRoute(ctx, "RT-30")
	if err != nil {
		t.Fatalf("get route: %v", err)
	}
	if len(got.SwitchPositions) != 1 || len(got.Sections) != 1 {
		t.Fatalf("route children: sw=%d sec=%d", len(got.SwitchPositions), len(got.Sections))
	}
	if got.SwitchPositions[0].SwitchID != "SW-30" || got.Sections[0].SectionID != "SC-30" {
		t.Fatalf("route child values: %+v", got)
	}
}

// TestEventAppendAndList round-trips the event log.
func TestEventAppendAndList(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_ = st.CreateStation(ctx, &model.Station{ID: "ST-5", Code: "Y", Name: "Y", CreatedAt: time.Now().UTC()})
	err := st.InTx(ctx, func(tx *sql.Tx) error {
		return st.AppendEventJSON(ctx, tx, "ST-5", model.EventRouteEstablished, "route", "RT-5", "RT-5", map[string]string{"k": "v"})
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	evs, err := st.ListEventsByStation(ctx, "ST-5")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(evs) != 1 || evs[0].Kind != model.EventRouteEstablished {
		t.Fatalf("events: %+v", evs)
	}
}

// TestDelayTimerCreateAndGet round-trips the approach-lock timer.
func TestDelayTimerCreateAndGet(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_ = st.CreateStation(ctx, &model.Station{ID: "ST-6", Code: "W", Name: "W", CreatedAt: time.Now().UTC()})
	_ = st.CreateSignal(ctx, &model.Signal{ID: "SG-6", StationID: "ST-6", Name: "S6", Direction: model.DirectionArrival, Aspect: model.AspectRed, Home: true, CreatedAt: time.Now().UTC()})
	_ = st.CreateRoute(ctx, &model.Route{ID: "RT-6", StationID: "ST-6", Code: "W-6", Kind: model.RouteKindTrain, SourceSignalID: "SG-6", Terminal: "SG-6", State: model.RouteCancelling, CreatedAt: time.Now().UTC()})
	started := time.Now().UTC().Truncate(time.Second)
	err := st.InTx(ctx, func(tx *sql.Tx) error {
		return st.CreateDelayTimer(ctx, tx, "RT-6", "ST-6", model.RouteKindTrain, started, 180*time.Second)
	})
	if err != nil {
		t.Fatalf("create timer: %v", err)
	}
	got, delay, completed, err := st.GetDelayTimer(ctx, "RT-6")
	if err != nil {
		t.Fatalf("get timer: %v", err)
	}
	if delay.Seconds() != 180 || completed {
		t.Fatalf("timer: delay=%v completed=%v", delay, completed)
	}
	if !got.Equal(started) {
		t.Fatalf("timer started: got %v want %v", got, started)
	}
}

// TestOnceOccupiedSections round-trips the once-occupied bookkeeping.
func TestOnceOccupiedSections(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	_ = st.CreateStation(ctx, &model.Station{ID: "ST-8", Code: "V", Name: "V", CreatedAt: time.Now().UTC()})
	_ = st.CreateSection(ctx, &model.Section{ID: "SC-80", StationID: "ST-8", Name: "80G", Kind: model.SectionPlain, CreatedAt: time.Now().UTC()})
	_ = st.CreateSignal(ctx, &model.Signal{ID: "SG-8", StationID: "ST-8", Name: "S8", Direction: model.DirectionArrival, Aspect: model.AspectRed, Home: true, CreatedAt: time.Now().UTC()})
	_ = st.CreateRoute(ctx, &model.Route{ID: "RT-80", StationID: "ST-8", Code: "V-80", Kind: model.RouteKindTrain, SourceSignalID: "SG-8", Terminal: "SC-80", State: model.RouteEstablished, CreatedAt: time.Now().UTC(), Sections: []model.RouteSection{{SectionID: "SC-80", Seq: 0}}})
	err := st.InTx(ctx, func(tx *sql.Tx) error {
		return st.MarkSectionOnceOccupied(ctx, tx, "RT-80", "SC-80")
	})
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	m, err := st.OnceOccupiedSections(ctx, "RT-80")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !m["SC-80"] {
		t.Fatalf("once-occupied should include SC-80, got %v", m)
	}
}
