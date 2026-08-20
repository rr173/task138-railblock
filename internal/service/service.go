// Package service orchestrates the interlocking business rules over the
// persistent store. Each service method assembles a read-only Yard snapshot
// from the store, calls the pure interlocking/signaling/trainops functions to
// compute the effect, then applies the effect inside a single IMMEDIATE
// transaction (recording an event per change). The transaction boundary is
// what makes the invariants atomic; the event log is what makes the state
// recoverable.
package service

import (
	"context"

	"task138-railblock/internal/clock"
	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/store"
)

// Services is the aggregate facade the HTTP layer uses. It owns a single store
// and an injected clock (Fake during the smoke-test, Real in production).
type Services struct {
	st    *store.Store
	clk   clock.Clock
	route *RouteService
	sw    *SwitchService
	train *TrainService
	dia   *DiagramService
	rec   *ReconcileService
}

// New builds the facade over st with a real wall clock.
func New(st *store.Store) *Services {
	return NewWithClock(st, clock.Real{})
}

// NewWithClock lets the self-check inject a Fake clock so approach-lock delays
// elapse without sleeping.
func NewWithClock(st *store.Store, clk clock.Clock) *Services {
	s := &Services{st: st, clk: clk}
	s.route = &RouteService{svc: s}
	s.sw = &SwitchService{svc: s}
	s.train = &TrainService{svc: s}
	s.dia = &DiagramService{svc: s}
	s.rec = &ReconcileService{svc: s}
	return s
}

// Store returns the underlying store (used by self-check and tests).
func (s *Services) Store() *store.Store { return s.st }

// Clock returns the injected clock.
func (s *Services) Clock() clock.Clock { return s.clk }

// Route returns the route service.
func (s *Services) Route() *RouteService { return s.route }

// Switch returns the switch service.
func (s *Services) Switch() *SwitchService { return s.sw }

// Train returns the train service.
func (s *Services) Train() *TrainService { return s.train }

// Diagram returns the diagram service.
func (s *Services) Diagram() *DiagramService { return s.dia }

// Reconcile returns the reconcile service.
func (s *Services) Reconcile() *ReconcileService { return s.rec }

// loadYard assembles a read-only interlocking.Yard for a station from the
// store, using the bare connection (outside a transaction). The yard snapshot
// is the bridge between persistence and the pure rule functions.
func (s *Services) loadYard(ctx context.Context, stationID string) (*interlocking.Yard, error) {
	return loadYard(ctx, s.st, stationID)
}
