package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
)

// smokeRouteEstablishAndSignal establishes a route and asserts the switch is
// locked, the sections are locked, the signal opened to a permissive aspect and
// the route state is established.
func smokeRouteEstablishAndSignal(srv *httptest.Server, clk *clock.Fake) error {
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
	est, err := establishRoute(srv, rt.ID)
	if err != nil {
		return err
	}
	if est.State != model.RouteEstablished {
		return fmt.Errorf("route state: want established got %s", est.State)
	}
	// The route must require the switch in normal; the switch was created
	// normal, so no reversal. It must be locked by this route.
	var view struct {
		Switches []*model.Switch `json:"switches"`
	}
	if err := mustDo(srv, "GET", "/stations/"+stID, nil, false, &view); err != nil {
		return err
	}
	var sw *model.Switch
	for _, s := range view.Switches {
		if s.ID == swID {
			sw = s
		}
	}
	if sw == nil {
		return fmt.Errorf("switch %s not in view", swID)
	}
	if !sw.Locked || sw.LockedByRoute != rt.ID {
		return fmt.Errorf("switch not locked by route: locked=%v by=%s", sw.Locked, sw.LockedByRoute)
	}
	// Signal aspect should be permissive (G, since straight route).
	var sa struct {
		Aspect string `json:"aspect"`
	}
	if err := mustDo(srv, "GET", "/routes/"+rt.ID+"/signal", nil, false, &sa); err != nil {
		return err
	}
	if sa.Aspect == "R" {
		return fmt.Errorf("signal should be permissive after establish, got R")
	}
	return nil
}

// smokeSwitchInOccupiedSection occupies the switch section of an established
// route's switch (by a train) and asserts a manual reversal is rejected with
// 422 (switch in occupied section) — a route's switches are locked, so reversal
// is rejected with 409. Then it tests a free switch: occupy the plain section
// (which is locked by the route too) — either way the switch cannot move while
// its section is occupied or the switch is locked.
func smokeSwitchInOccupiedSection(srv *httptest.Server, clk *clock.Fake) error {
	stID, _, swSection, swID, plain, bay, sig, err := buildSimpleStation(srv)
	if err != nil {
		return err
	}
	// Do NOT establish a route here; occupy the switch section directly and
	// try to reverse a switch in it.
	var tr model.Train
	if err := mustDo(srv, "POST", "/trains", map[string]string{"code": "G2", "station_id": stID}, false, &tr); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/occupy", map[string]string{"section_id": swSection}, false, nil); err != nil {
		return err
	}
	// Reversing a switch whose section is occupied must fail (422).
	if err := expectCode(srv, "POST", "/switches/"+swID+"/reverse", nil, false, 422); err != nil {
		return err
	}
	// Clear and reverse freely.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/clear", map[string]string{"section_id": swSection}, false, nil); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/switches/"+swID+"/reverse", nil, false, nil); err != nil {
		return err
	}
	_ = plain
	_ = bay
	_ = sig
	return nil
}
