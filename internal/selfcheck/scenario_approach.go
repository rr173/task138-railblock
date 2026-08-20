package selfcheck

import (
	"fmt"
	"net/http/httptest"
	"time"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
)

// smokeApproachLockAndTimedCancel establishes a route, drives a train into the
// approach section (→ approach-locked), requests a manual cancel and asserts it
// enters the timed-cancel (Cancelling) state rather than releasing at once.
// Then it completes the timed cancel and asserts the route is released.
func smokeApproachLockAndTimedCancel(srv *httptest.Server, clk *clock.Fake) error {
	stID, approach, swSection, swID, plain, bay, sig, err := buildSimpleStation(srv)
	if err != nil {
		return err
	}
	swPos := []model.RouteSwitchPosition{{SwitchID: swID, RequiredPosition: model.PositionNormal}}
	secs := []model.RouteSection{{SectionID: swSection, Seq: 0}, {SectionID: plain, Seq: 1}, {SectionID: bay, Seq: 2}}
	rt, err := defineRoute(srv, stID, "X-5G", "train", sig, bay, approach, swPos, secs)
	if err != nil {
		return err
	}
	if _, err := establishRoute(srv, rt.ID); err != nil {
		return err
	}
	// Train.
	var tr model.Train
	if err := mustDo(srv, "POST", "/trains", map[string]string{"code": "G3", "station_id": stID}, false, &tr); err != nil {
		return err
	}
	// Occupy the approach section → approach-locked.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/occupy", map[string]string{"section_id": approach}, false, nil); err != nil {
		return err
	}
	var view struct {
		Routes []*model.Route `json:"routes"`
	}
	if err := mustDo(srv, "GET", "/stations/"+stID, nil, false, &view); err != nil {
		return err
	}
	var got *model.Route
	for _, r := range view.Routes {
		if r.ID == rt.ID {
			got = r
		}
	}
	if got == nil || got.State != model.RouteApproachLocked {
		return fmt.Errorf("approach-locked expected, got %v", stateOr(got))
	}
	// Manual cancel → timed delay (Cancelling), not immediate.
	var cancelled *model.Route
	if err := mustDo(srv, "POST", "/routes/"+rt.ID+"/cancel", nil, false, &cancelled); err != nil {
		return err
	}
	if cancelled.State != model.RouteCancelling {
		return fmt.Errorf("cancel: want cancelling got %s", cancelled.State)
	}
	// Advance the fake clock past the train-route delay (180s).
	clk.Advance(200 * time.Second)
	if _, err := completeCancel(srv, rt.ID); err != nil {
		return fmt.Errorf("complete-cancel: %w", err)
	}
	// Now the route should be Cancelled and the switch/sections released.
	if err := mustDo(srv, "GET", "/stations/"+stID, nil, false, &view); err != nil {
		return err
	}
	for _, r := range view.Routes {
		if r.ID == rt.ID {
			got = r
		}
	}
	if got == nil || got.State != model.RouteCancelled {
		return fmt.Errorf("after complete-cancel: want cancelled got %v", stateOr(got))
	}
	return nil
}

func stateOr(r *model.Route) model.RouteState {
	if r == nil {
		return ""
	}
	return r.State
}

// completeCancel helper.
func completeCancel(srv *httptest.Server, id string) (*model.Route, error) {
	var rt model.Route
	if err := mustDo(srv, "POST", "/routes/"+id+"/complete-cancel", nil, false, &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}
