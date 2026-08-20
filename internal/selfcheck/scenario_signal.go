package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/model"
)

// smokeSignalAutoCloseAndReopen establishes a route (signal open), loses a
// switch indication (signal must close to red), then restores it (signal must
// reopen to permissive) — asserting the signal aspect is always a recompute,
// never a free input.
func smokeSignalAutoCloseAndReopen(srv *httptest.Server, clk *clock.Fake) error {
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
	// Signal should be permissive.
	var sa struct {
		Aspect string `json:"aspect"`
	}
	if err := mustDo(srv, "GET", "/routes/"+rt.ID+"/signal", nil, false, &sa); err != nil {
		return err
	}
	if sa.Aspect == "R" {
		return fmt.Errorf("signal should be permissive after establish")
	}
	// Lose indication → signal red.
	if _, err := setIndication(srv, swID, false); err != nil {
		return err
	}
	if err := mustDo(srv, "GET", "/routes/"+rt.ID+"/signal", nil, false, &sa); err != nil {
		return err
	}
	if sa.Aspect != "R" {
		return fmt.Errorf("signal should be red after indication loss, got %s", sa.Aspect)
	}
	// Restore indication → signal permissive again.
	if _, err := setIndication(srv, swID, true); err != nil {
		return err
	}
	if err := mustDo(srv, "GET", "/routes/"+rt.ID+"/signal", nil, false, &sa); err != nil {
		return err
	}
	if sa.Aspect == "R" {
		return fmt.Errorf("signal should be permissive after indication restored")
	}
	return nil
}

// setIndication helper.
func setIndication(srv *httptest.Server, id string, ok bool) (*model.Switch, error) {
	var sw model.Switch
	if err := mustDo(srv, "POST", "/switches/"+id+"/indication", map[string]any{"ok": ok}, false, &sw); err != nil {
		return nil, err
	}
	return &sw, nil
}
