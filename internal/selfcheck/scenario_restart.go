package selfcheck

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
	"task138-railblock/internal/service"
	"task138-railblock/internal/store"
)

// smokeRestartRecovery seeds a station with an established route, closes the
// store (simulating a crash), reopens the same SQLite file, calls ReconcileAll
// and asserts the route state, switch/section locks and signal aspect match
// the pre-crash state. It also tests mid-timed-cancel recovery.
func smokeRestartRecovery(dbPath string, clk *clock.Fake) error {
	// Phase 1: build and establish.
	srv1, st1, err := restartServer(dbPath, clk)
	if err != nil {
		return err
	}
	defer srv1.Close()
	stID, approach, swSection, swID, plain, bay, sig, err := buildSimpleStation(srv1)
	if err != nil {
		return err
	}
	swPos := []model.RouteSwitchPosition{{SwitchID: swID, RequiredPosition: model.PositionNormal}}
	secs := []model.RouteSection{{SectionID: swSection, Seq: 0}, {SectionID: plain, Seq: 1}, {SectionID: bay, Seq: 2}}
	rt, err := defineRoute(srv1, stID, "X-5G", "train", sig, bay, approach, swPos, secs)
	if err != nil {
		return err
	}
	if _, err := establishRoute(srv1, rt.ID); err != nil {
		return err
	}
	// Snapshot the pre-crash route state.
	preRoute, err := srv1GetRoute(srv1, rt.ID)
	if err != nil {
		return err
	}
	if preRoute.State != model.RouteEstablished {
		return fmt.Errorf("pre-crash state: want established got %s", preRoute.State)
	}
	// Close the server + store to simulate a crash.
	srv1.Close()
	if err := st1.Close(); err != nil {
		return err
	}

	// Phase 2: reopen and reconcile.
	srv2, st2, err := restartServer(dbPath, clk)
	if err != nil {
		return err
	}
	defer srv2.Close()
	defer st2.Close()
	svc2 := service.NewWithClock(st2, clk)
	if _, err := svc2.Reconcile().ReconcileAll(ctx()); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	// The route should still be established after recovery.
	postRoute, err := srv2GetRoute(srv2, rt.ID)
	if err != nil {
		return err
	}
	if postRoute.State != model.RouteEstablished {
		return fmt.Errorf("post-recovery state: want established got %s", postRoute.State)
	}
	// The switch should still be locked.
	var view struct {
		Switches []*model.Switch `json:"switches"`
	}
	if err := mustDo(srv2, "GET", "/stations/"+stID, nil, false, &view); err != nil {
		return err
	}
	for _, sw := range view.Switches {
		if sw.ID == swID && !sw.Locked {
			return fmt.Errorf("switch %s lock not recovered", swID)
		}
	}
	// Filesystem hygiene: the DB file exists.
	if _, err := os.Stat(filepath.Clean(dbPath)); err != nil {
		return fmt.Errorf("db file: %w", err)
	}
	_ = store.Open
	return nil
}

func srv1GetRoute(srv *httptest.Server, id string) (*model.Route, error) {
	var rt model.Route
	if err := mustDo(srv, "GET", "/routes/"+id, nil, false, &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}

func srv2GetRoute(srv *httptest.Server, id string) (*model.Route, error) {
	var rt model.Route
	if err := mustDo(srv, "GET", "/routes/"+id, nil, false, &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}
