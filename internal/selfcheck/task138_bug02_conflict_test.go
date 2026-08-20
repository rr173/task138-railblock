package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestSharedRouteResourcesStayMutuallyExclusiveInPlanAndEstablishment(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-02T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "conflict.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeRouteConflictGraph(srv, clk); err != nil { t.Fatal(err) }
}
