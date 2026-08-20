package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
)

// buildSimpleStation builds a small station with: an approach section, a
// switch section containing one switch, a plain section and a bay, plus an
// arrival home signal. Returns the IDs the scenario needs.
func buildSimpleStation(srv *httptest.Server) (stationID, approachSec, swSection, swID, plainSec, baySec, sigID string, err error) {
	var st model.Station
	if err = mustDo(srv, "POST", "/stations", map[string]string{"code": "XN", "name": "虹桥"}, false, &st); err != nil {
		return
	}
	stationID = st.ID
	var approach, swS, plain, bay model.Section
	secs := []struct {
		name, kind string
		out        *model.Section
	}{
		{"1AG", "approach", &approach},
		{"1DG", "switch", &swS},
		{"3G", "plain", &plain},
		{"5G", "bay", &bay},
	}
	for _, s := range secs {
		if err = mustDo(srv, "POST", "/stations/"+st.ID+"/sections", map[string]any{"name": s.name, "kind": s.kind}, false, s.out); err != nil {
			return
		}
	}
	approachSec, swSection, plainSec, baySec = approach.ID, swS.ID, plain.ID, bay.ID
	var sw model.Switch
	if err = mustDo(srv, "POST", "/stations/"+st.ID+"/switches", map[string]any{"name": "1#", "normal_position": 0, "section_id": swS.ID}, false, &sw); err != nil {
		return
	}
	swID = sw.ID
	var sig model.Signal
	if err = mustDo(srv, "POST", "/stations/"+st.ID+"/signals", map[string]any{"name": "X", "direction": "arrival", "home": true}, false, &sig); err != nil {
		return
	}
	sigID = sig.ID
	return
}

// defineRoute helper: posts a route definition and returns the route.
func defineRoute(srv *httptest.Server, stID, code, kind, sourceSig, terminal, approach string, swPos []model.RouteSwitchPosition, secs []model.RouteSection) (*model.Route, error) {
	var rt model.Route
	body := map[string]any{
		"station_id":          stID,
		"code":                code,
		"kind":                kind,
		"source_signal_id":    sourceSig,
		"terminal":            terminal,
		"approach_section_id": approach,
		"switch_positions":    swPos,
		"sections":            secs,
	}
	if err := mustDo(srv, "POST", "/routes", body, false, &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}

// smokeRouteConflictGraph defines two routes sharing the switch section, then
// asserts the conflict graph reports them as mutually conflicting, and that
// establishing one blocks the other.
func smokeRouteConflictGraph(srv *httptest.Server, clk *clock.Fake) error {
	stID, approach, swSection, swID, plain, bay, sig, err := buildSimpleStation(srv)
	if err != nil {
		return err
	}
	swPos := []model.RouteSwitchPosition{{SwitchID: swID, RequiredPosition: model.PositionNormal}}
	secs := []model.RouteSection{{SectionID: swSection, Seq: 0}, {SectionID: plain, Seq: 1}, {SectionID: bay, Seq: 2}}
	rtA, err := defineRoute(srv, stID, "X-5G", "train", sig, bay, approach, swPos, secs)
	if err != nil {
		return err
	}
	rtB, err := defineRoute(srv, stID, "S-5G", "train", sig, bay, approach, swPos, secs)
	if err != nil {
		return err
	}
	// rtA and rtB share every section → conflict.
	var cf struct {
		Conflicts []string `json:"conflicts"`
	}
	if err := mustDo(srv, "GET", "/routes/"+rtA.ID+"/conflicts", nil, false, &cf); err != nil {
		return err
	}
	if len(cf.Conflicts) == 0 {
		return fmt.Errorf("expected rtA to conflict with rtB, got none")
	}
	found := false
	for _, c := range cf.Conflicts {
		if c == rtB.ID {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("rtB not in rtA conflicts: %v", cf.Conflicts)
	}
	// Establish A, then B should fail with 409.
	if _, err := establishRoute(srv, rtA.ID); err != nil {
		return fmt.Errorf("establish A: %w", err)
	}
	if err := expectCode(srv, "POST", "/routes/"+rtB.ID+"/establish", nil, false, 409); err != nil {
		return fmt.Errorf("establish B should conflict: %w", err)
	}
	return nil
}

// establishRoute helper: POSTs establish and returns the refreshed route.
func establishRoute(srv *httptest.Server, id string) (*model.Route, error) {
	var rt model.Route
	if err := mustDo(srv, "POST", "/routes/"+id+"/establish", nil, false, &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}
