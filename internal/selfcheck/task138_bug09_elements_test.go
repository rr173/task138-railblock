package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestStationDiagramReturnsPersistedSignalSwitchSectionAndTrackElements(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-09T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "elements.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeStationAndYardElements(srv, clk); err != nil { t.Fatal(err) }
}
