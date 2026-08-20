package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestOccupiedSwitchSectionRejectsMovementBeforePersistingAnyReversal(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-03T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "switch.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeSwitchInOccupiedSection(srv, clk); err != nil { t.Fatal(err) }
}
