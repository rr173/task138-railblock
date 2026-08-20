package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestFaultUnlockClearsEveryResidualProtectionBeforeCancellingRoute(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-07T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "fault.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeFaultUnlockResidualLocks(srv, clk); err != nil { t.Fatal(err) }
}
