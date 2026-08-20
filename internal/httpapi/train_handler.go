package httpapi

import (
	"net/http"
	"time"

	"task138-railblock/internal/idlib"
	"task138-railblock/internal/model"
)

// createTrain: POST /trains {id,code,station_id}
func (h *handlers) createTrain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID        string `json:"id"`
		Code      string `json:"code"`
		StationID string `json:"station_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Code == "" || req.StationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and station_id are required"})
		return
	}
	tr := &model.Train{
		ID:        idlib.Prefixed("TR", req.ID),
		Code:      req.Code,
		StationID: req.StationID,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.svc.Store().CreateTrain(r.Context(), tr); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tr)
}

// getTrain: GET /trains/{id}
func (h *handlers) getTrain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	tr, err := h.svc.Store().GetTrain(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

// trainOccupy: POST /trains/{id}/occupy {section_id} → ok
func (h *handlers) trainOccupy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		SectionID string `json:"section_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "section_id is required"})
		return
	}
	if err := h.svc.Train().Occupy(r.Context(), id, req.SectionID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "occupied"})
}

// trainClear: POST /trains/{id}/clear {section_id} → ok
func (h *handlers) trainClear(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		SectionID string `json:"section_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.SectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "section_id is required"})
		return
	}
	if err := h.svc.Train().Clear(r.Context(), id, req.SectionID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

// setBlockSection: POST /block-sections {id,station_id,name,adjacent_signal,occupied}
func (h *handlers) setBlockSection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID             string `json:"id"`
		StationID      string `json:"station_id"`
		Name           string `json:"name"`
		AdjacentSignal string `json:"adjacent_signal"`
		Occupied       bool   `json:"occupied"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	b := &model.BlockSection{
		ID:             idlib.Prefixed("BK", req.ID),
		StationID:      req.StationID,
		Name:           req.Name,
		AdjacentSignal: req.AdjacentSignal,
		Occupied:       req.Occupied,
	}
	if err := h.svc.Store().CreateBlockSection(r.Context(), b); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// rebuild: POST /admin/rebuild → {corrections}
func (h *handlers) rebuild(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.Reconcile().ReconcileAll(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"corrections": n})
}

// reconcileReport: GET /admin/reconcile-report → {corrections}
func (h *handlers) reconcileReport(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.Reconcile().ReconcileAll(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"corrections": n})
}
