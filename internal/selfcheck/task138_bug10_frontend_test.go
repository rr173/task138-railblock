package selfcheck

import (
	"path/filepath"
	"testing"

	"task138-railblock/internal/clock"
)

func TestEmbeddedControlPageServesAndReadsBackTheStationItCreates(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-06-10T08:00:00Z"))
	srv, err := newServer(filepath.Join(t.TempDir(), "frontend.db"), clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close()
	if err := smokeFrontend(srv, clk); err != nil { t.Fatal(err) }
}
