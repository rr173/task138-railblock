package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestApproachingTrainKeepsCancellationLockedUntilItsConfiguredDelayExpires(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-04T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "approach.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeApproachLockAndTimedCancel(srv, clk); err != nil { t.Fatal(err) }
}
