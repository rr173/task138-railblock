package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
)

// smokeThreePointSectionalRelease drives a train through an established route
// section by section and asserts the trailing sections release as the train
// moves ahead, and the whole route unlocks once the terminal section clears.
func smokeThreePointSectionalRelease(srv *httptest.Server, clk *clock.Fake) error {
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
	var tr model.Train
	if err := mustDo(srv, "POST", "/trains", map[string]string{"code": "G4", "station_id": stID}, false, &tr); err != nil {
		return err
	}
	// Occupy approach → approach-locked.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/occupy", map[string]string{"section_id": approach}, false, nil); err != nil {
		return err
	}
	// Enter swSection (first route section): signal closes.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/occupy", map[string]string{"section_id": swSection}, false, nil); err != nil {
		return err
	}
	// Clear approach, enter plain.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/clear", map[string]string{"section_id": approach}, false, nil); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/occupy", map[string]string{"section_id": plain}, false, nil); err != nil {
		return err
	}
	// Clear swSection, enter bay.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/clear", map[string]string{"section_id": swSection}, false, nil); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/occupy", map[string]string{"section_id": bay}, false, nil); err != nil {
		return err
	}
	// Clear plain.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/clear", map[string]string{"section_id": plain}, false, nil); err != nil {
		return err
	}
	// Clear bay (terminal) → whole route unlocks.
	if err := mustDo(srv, "POST", "/trains/"+tr.ID+"/clear", map[string]string{"section_id": bay}, false, nil); err != nil {
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
	if got == nil || got.State != model.RouteUnlocked {
		return fmt.Errorf("after full pass: want unlocked got %v", stateOr(got))
	}
	return nil
}
