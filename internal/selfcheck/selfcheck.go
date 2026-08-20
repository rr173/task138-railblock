// Package selfcheck runs the --smoke-test for the railway-interlocking engine.
// Each scenario builds its own temporary SQLite file and httptest server so
// contract assertions don't trip over state left by an earlier scenario. The
// restart scenario seeds state, closes the store, reopens the same file and
// asserts the recomputed interlocking state is unchanged.
//
// The smoke test never sleeps and never touches the network; it talks to the
// real mux over httptest.NewServer so the HTTP/JSON contract itself is
// exercised end to end, including the embedded frontend route.
package selfcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/httpapi"
	"task138-railblock/internal/service"
	"task138-railblock/internal/store"
	"task138-railblock/internal/webfs"
)

// adminToken used by the smoke test for the admin endpoints.
const adminToken = "railblock-secret"

// Run executes every smoke scenario. Returns the first failure.
func Run() error {
	dir, err := os.MkdirTemp("", "railblock-smoke-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	// Shared deterministic fake clock pinned to a fixed instant. Scenarios that
	// need to advance the approach-lock delay call clk.Advance.
	clk := clock.NewFake(parseTime("2026-01-10T09:00:00Z"))

	cases := []struct {
		name string
		fn   func(srv *httptest.Server, clk *clock.Fake) error
	}{
		{"station-and-yard-elements", smokeStationAndYardElements},
		{"route-conflict-graph", smokeRouteConflictGraph},
		{"route-establish-and-signal", smokeRouteEstablishAndSignal},
		{"switch-in-occupied-section", smokeSwitchInOccupiedSection},
		{"approach-lock-and-timed-cancel", smokeApproachLockAndTimedCancel},
		{"three-point-sectional-release", smokeThreePointSectionalRelease},
		{"signal-auto-close-and-reopen", smokeSignalAutoCloseAndReopen},
		{"fault-unlock-residual-locks", smokeFaultUnlockResidualLocks},
		{"frontend-page-served", smokeFrontend},
	}
	for i, c := range cases {
		dbPath := filepath.Join(dir, fmt.Sprintf("smoke-%02d.db", i))
		srv, err := newServer(dbPath, clk)
		if err != nil {
			return fmt.Errorf("%s: new server: %w", c.name, err)
		}
		if err := c.fn(srv, clk); err != nil {
			srv.Close()
			return fmt.Errorf("%s: %w", c.name, err)
		}
		srv.Close()
	}

	// Restart-recovery runs separately because it controls the store lifecycle.
	if err := smokeRestartRecovery(filepath.Join(dir, "smoke-recover.db"), clk); err != nil {
		return fmt.Errorf("restart-recovery: %w", err)
	}
	return nil
}

// newServer opens a fresh SQLite file, builds the service (with the injected
// clock) + mux and returns an httptest server over the real HTTP handler tree.
func newServer(dbPath string, clk clock.Clock) (*httptest.Server, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	svc := service.NewWithClock(st, clk)
	webFS := webfs.FS()
	mux := httpapi.NewMux(httpapi.Services{Svc: svc}, adminToken, webFS)
	srv := httptest.NewServer(mux)
	srv.Config.RegisterOnShutdown(func() { _ = st.Close() })
	return srv, nil
}

// --- HTTP helpers ---

func doJSON(srv *httptest.Server, method, path string, body any, admin bool) (int, []byte, error) {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(buf))
		r.Header.Set("Content-Type", "application/json")
	}
	if admin {
		r.Header.Set("Authorization", "Bearer "+adminToken)
	}
	rec := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(rec, r)
	return rec.Code, rec.Body.Bytes(), nil
}

// mustDo performs an HTTP call and returns the decoded body; it fails the
// scenario immediately on a non-2xx status (success-expecting helpers must
// surface non-200s, not swallow them).
func mustDo(srv *httptest.Server, method, path string, body any, admin bool, out any) error {
	code, respBody, err := doJSON(srv, method, path, body, admin)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, code, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		if e := json.Unmarshal(respBody, out); e != nil {
			return fmt.Errorf("%s %s: decode: %w", method, path, e)
		}
	}
	return nil
}

// expectCode asserts the HTTP status without decoding the body.
func expectCode(srv *httptest.Server, method, path string, body any, admin bool, wantCode int) error {
	code, respBody, err := doJSON(srv, method, path, body, admin)
	if err != nil {
		return err
	}
	if code != wantCode {
		return fmt.Errorf("%s %s: want status %d, got %d: %s", method, path, wantCode, code, string(respBody))
	}
	return nil
}

// parseTime parses an RFC3339 timestamp; it panics on error because a malformed
// fixed base time is a programmer mistake, not a runtime condition.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("selfcheck: bad base time " + s + ": " + err.Error())
	}
	return t
}

// restartServer is a helper for the restart scenario: open a fresh store+service
// over an existing dbPath and return a test server, plus the underlying store so
// the scenario can close it to simulate a crash.
func restartServer(dbPath string, clk clock.Clock) (*httptest.Server, *store.Store, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	svc := service.NewWithClock(st, clk)
	webFS := webfs.FS()
	mux := httpapi.NewMux(httpapi.Services{Svc: svc}, adminToken, webFS)
	srv := httptest.NewServer(mux)
	return srv, st, nil
}

// ctx returns a background context.
func ctx() context.Context { return context.Background() }
