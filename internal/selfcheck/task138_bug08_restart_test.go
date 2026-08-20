package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestRestartReplayRetainsEstablishedRouteProtectionAndSignalState(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-08T08:00:00Z"))
	if err := smokeRestartRecovery(filepath.Join(t.TempDir(), "restart.db"), clk); err != nil { t.Fatal(err) }
}
