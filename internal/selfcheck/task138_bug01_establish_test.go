package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestEstablishmentRetainsLockAndPermissiveSignalAcrossSnapshotAndStore(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-01T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "establish.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeRouteEstablishAndSignal(srv, clk); err != nil { t.Fatal(err) }
}
