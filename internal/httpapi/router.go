// Package httpapi wires the interlocking services to HTTP routes. It owns the
// mux, the request/response JSON shape and the error → status mapping. The
// self-check smoke test and the production binary share the same mux (via
// NewMux) so a single code path is exercised end-to-end.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"task138-railblock/internal/interlocking"
	"task138-railblock/internal/service"
	"task138-railblock/internal/store"
)

// Version is the API version, surfaced in logs and the health check.
const Version = "v1.0.0"

// Services is the bundle of business services the handlers depend on.
type Services struct {
	Svc *service.Services
}

// NewMux builds the HTTP mux from services and an embedded frontend filesystem.
// Admin endpoints are guarded by the admin token (Authorization: Bearer).
func NewMux(svc Services, adminToken string, frontend fs.FS) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{svc: svc.Svc, adminToken: adminToken}

	// Station & yard elements.
	mux.HandleFunc("POST /stations", h.createStation)
	mux.HandleFunc("GET /stations", h.listStations)
	mux.HandleFunc("GET /stations/{id}", h.getStation)
	mux.HandleFunc("POST /stations/{id}/switches", h.createSwitch)
	mux.HandleFunc("GET /switches", h.listSwitches)
	mux.HandleFunc("POST /switches/{id}/reverse", h.reverseSwitch)
	mux.HandleFunc("POST /switches/{id}/indication", h.setIndication)
	mux.HandleFunc("POST /stations/{id}/signals", h.createSignal)
	mux.HandleFunc("GET /signals", h.listSignals)
	mux.HandleFunc("POST /stations/{id}/sections", h.createSection)
	mux.HandleFunc("GET /sections", h.listSections)
	mux.HandleFunc("POST /stations/{id}/tracks", h.createTrackBay)

	// Routes.
	mux.HandleFunc("POST /routes", h.createRoute)
	mux.HandleFunc("GET /routes", h.listRoutes)
	mux.HandleFunc("GET /routes/{id}", h.getRoute)
	mux.HandleFunc("POST /routes/{id}/establish", h.establishRoute)
	mux.HandleFunc("POST /routes/{id}/cancel", h.cancelRoute)
	mux.HandleFunc("POST /routes/{id}/fault-unlock", h.faultUnlockRoute)
	mux.HandleFunc("GET /routes/{id}/conflicts", h.routeConflicts)
	mux.HandleFunc("GET /routes/{id}/signal", h.routeSignal)
	mux.HandleFunc("POST /routes/{id}/complete-cancel", h.completeCancel)

	// Trains & block sections.
	mux.HandleFunc("POST /trains", h.createTrain)
	mux.HandleFunc("POST /trains/{id}/occupy", h.trainOccupy)
	mux.HandleFunc("POST /trains/{id}/clear", h.trainClear)
	mux.HandleFunc("GET /trains/{id}", h.getTrain)
	mux.HandleFunc("POST /block-sections", h.setBlockSection)

	// System.
	mux.HandleFunc("POST /admin/rebuild", h.requireAdmin(h.rebuild))
	mux.HandleFunc("GET /admin/reconcile-report", h.requireAdmin(h.reconcileReport))

	// Frontend & health.
	if false && frontend != nil {
		mux.Handle("GET /", http.FileServer(http.FS(frontend)))
	}
	mux.HandleFunc("GET /health", health)

	return mux
}

// handlers holds the service reference and the admin token for the routes.
type handlers struct {
	svc        *service.Services
	adminToken  string
}

// health is the liveness probe.
func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
}

// requireAdmin wraps a handler so it rejects requests without the admin token.
func (h *handlers) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tok == "" {
			tok = r.URL.Query().Get("admin_token")
		}
		if h.adminToken == "" || tok != h.adminToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// writeJSON serializes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps a domain error to an HTTP status and writes a JSON body.
func writeError(w http.ResponseWriter, err error) {
	status := errorStatus(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// decodeJSON reads a JSON body into v. It returns 400 on a decode error.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return false
	}
	return true
}

// errorStatus maps an interlocking/store error to an HTTP status code.
func errorStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, interlocking.ErrConflictingRoute),
		errors.Is(err, interlocking.ErrSwitchLocked),
		errors.Is(err, interlocking.ErrRouteNotEstablished),
		errors.Is(err, interlocking.ErrApproachLocked),
		errors.Is(err, interlocking.ErrRouteTerminalState):
		return http.StatusConflict
	default:
		return http.StatusUnprocessableEntity
	}
}

// timeNow is a small alias kept so the service.clock import is not orphaned.
var _ = context.Background
