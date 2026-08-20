package httpapi

import (
	"net/http"
	"time"

	"task138-railblock/internal/idlib"
	"task138-railblock/internal/model"
)

// createRoute: POST /routes
// Body: a route definition (id, station_id, code, kind, source_signal_id,
// terminal, approach_section_id, switch_positions[], sections[]). The route is
// created in the "pending" state; call POST /routes/{id}/establish to lock it.
func (h *handlers) createRoute(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID                string                   `json:"id"`
		StationID         string                   `json:"station_id"`
		Code              string                   `json:"code"`
		Kind              string                   `json:"kind"`
		SourceSignalID    string                   `json:"source_signal_id"`
		Terminal          string                   `json:"terminal"`
		ApproachSectionID string                   `json:"approach_section_id"`
		SwitchPositions   []model.RouteSwitchPosition `json:"switch_positions"`
		Sections          []model.RouteSection     `json:"sections"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.StationID == "" || req.SourceSignalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "station_id and source_signal_id are required"})
		return
	}
	rt := &model.Route{
		ID:                idlib.Prefixed("RT", req.ID),
		StationID:         req.StationID,
		Code:              req.Code,
		Kind:              model.RouteKind(req.Kind),
		SourceSignalID:    req.SourceSignalID,
		Terminal:          req.Terminal,
		ApproachSectionID: req.ApproachSectionID,
		SwitchPositions:   req.SwitchPositions,
		Sections:          req.Sections,
		State:             model.RoutePending,
		CreatedAt:         time.Now().UTC(),
	}
	if err := h.svc.Route().DefineRoute(r.Context(), rt); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rt)
}

// listRoutes: GET /routes?station=&state=
func (h *handlers) listRoutes(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station")
	state := r.URL.Query().Get("state")
	var list []*model.Route
	var err error
	if stationID != "" {
		list, err = h.svc.Store().ListRoutesByStation(r.Context(), stationID)
	} else if state != "" {
		list, err = h.svc.Store().ListRoutesByState(r.Context(), model.RouteState(state))
	} else {
		list, err = h.svc.Store().ListAllRoutes(r.Context())
	}
	if err != nil {
		writeError(w, err)
		return
	}
	// Load children for each route so the response is self-describing.
	out := make([]*model.Route, 0, len(list))
	for _, rt := range list {
		full, err := h.svc.Store().GetRoute(r.Context(), rt.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		out = append(out, full)
	}
	writeJSON(w, http.StatusOK, map[string]any{"routes": out})
}

// getRoute: GET /routes/{id}
func (h *handlers) getRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rt, err := h.svc.Store().GetRoute(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

// establishRoute: POST /routes/{id}/establish → route
func (h *handlers) establishRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rt, err := h.svc.Route().Establish(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

// cancelRoute: POST /routes/{id}/cancel → route
func (h *handlers) cancelRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rt, err := h.svc.Route().Cancel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

// completeCancel: POST /routes/{id}/complete-cancel → route (finishes a timed cancel)
func (h *handlers) completeCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rt, err := h.svc.Route().CompleteDelayedCancel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

// faultUnlockRoute: POST /routes/{id}/fault-unlock (admin) → route
func (h *handlers) faultUnlockRoute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rt, err := h.svc.Route().FaultUnlock(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt)
}

// routeConflicts: GET /routes/{id}/conflicts → []route_id
func (h *handlers) routeConflicts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conflicts, err := h.svc.Route().ConflictsOf(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"route_id": id, "conflicts": conflicts})
}

// routeSignal: GET /routes/{id}/signal → aspect
func (h *handlers) routeSignal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	aspect, err := h.svc.Route().SignalAspect(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"route_id": id, "aspect": string(aspect)})
}
