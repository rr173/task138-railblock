package selfcheck

import (
	"net/http/httptest"
	"testing"

	"task138-railblock/internal/clock"
)

// Run is exercised via the --smoke-test flag and also via this test entry so a
// plain `go test ./...` runs the whole contract.
func TestSmokeAll(t *testing.T) {
	if err := Run(); err != nil {
		t.Fatalf("smoke: %v", err)
	}
}

// guard against unused-import lints while scenarios land.
var _ = httptest.NewServer
var _ = func(*clock.Fake) {}
