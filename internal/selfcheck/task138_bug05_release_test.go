package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestPassedTrainReleasesRouteOnlyAfterSectionalEvidenceAndTerminalClear(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-05T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "release.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeThreePointSectionalRelease(srv, clk); err != nil { t.Fatal(err) }
}
