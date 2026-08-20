package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
)

// smokeStationAndYardElements builds a station with a switch section, a plain
// section, a bay, an approach section and a home signal, then asserts the
// station view returns them all.
func smokeStationAndYardElements(srv *httptest.Server, clk *clock.Fake) error {
	// Station.
	var st model.Station
	if err := mustDo(srv, "POST", "/stations", map[string]string{"code": "XN", "name": "虹桥"}, false, &st); err != nil {
		return err
	}
	if st.Code != "XN" {
		return fmt.Errorf("station code: want XN got %s", st.Code)
	}
	// Sections: an approach section, a switch section, a plain section and a bay.
	secReq := func(name, kind string) map[string]any {
		return map[string]any{"name": name, "kind": kind}
	}
	var approach, swSection, plain, bay model.Section
	for i, s := range []struct {
		name string
		kind string
		out  *model.Section
	}{
		{"1AG", "approach", &approach},
		{"1DG", "switch", &swSection},
		{"3G", "plain", &plain},
		{"5G", "bay", &bay},
	} {
		if err := mustDo(srv, "POST", "/stations/"+st.ID+"/sections", secReq(s.name, s.kind), false, s.out); err != nil {
			return fmt.Errorf("section %d: %w", i, err)
		}
	}
	// Switch in the switch section.
	var sw model.Switch
	if err := mustDo(srv, "POST", "/stations/"+st.ID+"/switches", map[string]any{"name": "1#", "normal_position": 0, "section_id": swSection.ID}, false, &sw); err != nil {
		return err
	}
	if !sw.HasIndication {
		return fmt.Errorf("switch should have indication by default")
	}
	// Home signal.
	var sig model.Signal
	if err := mustDo(srv, "POST", "/stations/"+st.ID+"/signals", map[string]any{"name": "X", "direction": "arrival", "home": true}, false, &sig); err != nil {
		return err
	}
	if sig.Aspect != model.AspectRed {
		return fmt.Errorf("new signal aspect: want R got %s", sig.Aspect)
	}
	// Track bay.
	var tb model.TrackBay
	if err := mustDo(srv, "POST", "/stations/"+st.ID+"/tracks", map[string]any{"name": "5G股道", "section_id": bay.ID}, false, &tb); err != nil {
		return err
	}
	// Verify the view.
	var view struct {
		Switches  []*model.Switch  `json:"switches"`
		Signals   []*model.Signal `json:"signals"`
		Sections  []*model.Section `json:"sections"`
		TrackBays []*model.TrackBay `json:"track_bays"`
	}
	if err := mustDo(srv, "GET", "/stations/"+st.ID, nil, false, &view); err != nil {
		return err
	}
	if len(view.Switches) != 1 || len(view.Signals) != 1 || len(view.Sections) != 4 || len(view.TrackBays) != 1 {
		return fmt.Errorf("view counts: sw=%d sig=%d sec=%d bay=%d", len(view.Switches), len(view.Signals), len(view.Sections), len(view.TrackBays))
	}
	return nil
}
