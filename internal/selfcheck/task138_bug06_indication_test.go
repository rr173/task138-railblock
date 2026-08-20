package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestIndicationLossClosesAnOpenRouteAndRestorationRecomputesIt(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-06T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "indication.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeSignalAutoCloseAndReopen(srv, clk); err != nil { t.Fatal(err) }
}
