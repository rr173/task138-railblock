package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
)

// smokeFaultUnlockResidualLocks establishes a route and then uses the fault
// unlock endpoint to force-release its locks even though no train has passed,
// asserting the route ends in Cancelled and every lock is released.
func smokeFaultUnlockResidualLocks(srv *httptest.Server, clk *clock.Fake) error {
	stID, _, swSection, swID, plain, bay, sig, err := buildSimpleStation(srv)
	if err != nil {
		return err
	}
	swPos := []model.RouteSwitchPosition{{SwitchID: swID, RequiredPosition: model.PositionNormal}}
	secs := []model.RouteSection{{SectionID: swSection, Seq: 0}, {SectionID: plain, Seq: 1}, {SectionID: bay, Seq: 2}}
	rt, err := defineRoute(srv, stID, "X-5G", "train", sig, bay, "", swPos, secs)
	if err != nil {
		return err
	}
	if _, err := establishRoute(srv, rt.ID); err != nil {
		return err
	}
	// Fault-unlock (admin) while the route is still established.
	if err := expectCode(srv, "POST", "/routes/"+rt.ID+"/fault-unlock", nil, true, 200); err != nil {
		return err
	}
	var view struct {
		Switches []*model.Switch `json:"switches"`
		Sections []*model.Section `json:"sections"`
		Routes   []*model.Route  `json:"routes"`
	}
	if err := mustDo(srv, "GET", "/stations/"+stID, nil, false, &view); err != nil {
		return err
	}
	for _, sw := range view.Switches {
		if sw.ID == swID && sw.Locked {
			return fmt.Errorf("switch still locked after fault-unlock")
		}
	}
	for _, sec := range view.Sections {
		if sec.Locked {
			return fmt.Errorf("section %s still locked after fault-unlock", sec.ID)
		}
	}
	for _, r := range view.Routes {
		if r.ID == rt.ID && r.State != model.RouteCancelled {
			return fmt.Errorf("route state after fault-unlock: want cancelled got %s", r.State)
		}
	}
	return nil
}
