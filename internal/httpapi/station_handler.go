package httpapi

import (
	"net/http"
	"time"

	"task138-railblock/internal/idlib"
	"task138-railblock/internal/model"
)

// createStation: POST /stations {code,name} → station
func (h *handlers) createStation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   string `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Code == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and name are required"})
		return
	}
	st := &model.Station{
		ID:        idlib.Prefixed("ST", req.ID),
		Code:      req.Code,
		Name:      req.Name,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.svc.Store().CreateStation(r.Context(), st); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

// listStations: GET /stations → []station
func (h *handlers) listStations(w http.ResponseWriter, r *http.Request) {
	sts, err := h.svc.Store().ListStations(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stations": sts})
}

// getStation: GET /stations/{id} → station + yard diagram
func (h *handlers) getStation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	view, err := h.svc.Diagram().StationView(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// createSwitch: POST /stations/{id}/switches {id,name,normal_position,section_id}
func (h *handlers) createSwitch(w http.ResponseWriter, r *http.Request) {
	stationID := r.PathValue("id")
	var req struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		NormalPosition int    `json:"normal_position"`
		SectionID      string `json:"section_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.SectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and section_id are required"})
		return
	}
	sw := &model.Switch{
		ID:              idlib.Prefixed("SW", req.ID),
		StationID:       stationID,
		Name:            req.Name,
		NormalPosition:  model.SwitchPosition(req.NormalPosition),
		CurrentPosition: model.SwitchPosition(req.NormalPosition),
		SectionID:       req.SectionID,
		HasIndication:   false,
		CreatedAt:       time.Now().UTC(),
	}
	if err := h.svc.Store().CreateSwitch(r.Context(), sw); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sw)
}

// listSwitches: GET /switches?station=...
func (h *handlers) listSwitches(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station")
	var list []*model.Switch
	var err error
	if stationID != "" {
		list, err = h.svc.Store().ListSwitchesByStation(r.Context(), stationID)
	} else {
		// Without a station filter, enumerate every station's switches.
		sts, lerr := h.svc.Store().ListStations(r.Context())
		if lerr != nil {
			writeError(w, lerr)
			return
		}
		for _, s := range sts {
			sw, lerr := h.svc.Store().ListSwitchesByStation(r.Context(), s.ID)
			if lerr != nil {
				writeError(w, lerr)
				return
			}
			list = append(list, sw...)
		}
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"switches": list})
}

// reverseSwitch: POST /switches/{id}/reverse → switch
func (h *handlers) reverseSwitch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sw, err := h.svc.Switch().Reverse(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sw)
}

// setIndication: POST /switches/{id}/indication {ok} → switch
func (h *handlers) setIndication(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		OK bool `json:"ok"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sw, err := h.svc.Switch().SetIndication(r.Context(), id, req.OK)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sw)
}

// createSignal: POST /stations/{id}/signals
func (h *handlers) createSignal(w http.ResponseWriter, r *http.Request) {
	stationID := r.PathValue("id")
	var req struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Direction string `json:"direction"`
		Home      bool   `json:"home"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.Direction == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and direction are required"})
		return
	}
	sig := &model.Signal{
		ID:        idlib.Prefixed("SG", req.ID),
		StationID: stationID,
		Name:      req.Name,
		Direction: model.SignalDirection(req.Direction),
		Aspect:    model.AspectRed,
		Home:      req.Home,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.svc.Store().CreateSignal(r.Context(), sig); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sig)
}

// listSignals: GET /signals?station=...
func (h *handlers) listSignals(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station")
	list, err := h.svc.Store().ListSignalsByStation(r.Context(), stationID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"signals": list})
}

// createSection: POST /stations/{id}/sections
func (h *handlers) createSection(w http.ResponseWriter, r *http.Request) {
	stationID := r.PathValue("id")
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.Kind == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and kind are required"})
		return
	}
	sec := &model.Section{
		ID:        idlib.Prefixed("SC", req.ID),
		StationID: stationID,
		Name:      req.Name,
		Kind:      model.SectionKind(req.Kind),
		CreatedAt: time.Now().UTC(),
	}
	if err := h.svc.Store().CreateSection(r.Context(), sec); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sec)
}

// listSections: GET /sections?station=...
func (h *handlers) listSections(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station")
	list, err := h.svc.Store().ListSectionsByStation(r.Context(), stationID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": list})
}

// createTrackBay: POST /stations/{id}/tracks
func (h *handlers) createTrackBay(w http.ResponseWriter, r *http.Request) {
	stationID := r.PathValue("id")
	var req struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		SectionID string `json:"section_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || req.SectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and section_id are required"})
		return
	}
	bay := &model.TrackBay{
		ID:        idlib.Prefixed("TB", req.ID),
		StationID: stationID,
		Name:      req.Name,
		SectionID: req.SectionID,
	}
	if err := h.svc.Store().CreateTrackBay(r.Context(), bay); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, bay)
}
