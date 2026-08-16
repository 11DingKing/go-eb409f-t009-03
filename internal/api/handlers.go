package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ejinagrid/ejinagrid/internal/domain"
	"github.com/ejinagrid/ejinagrid/internal/service"
	"github.com/ejinagrid/ejinagrid/internal/store"
)

// Handlers holds the application dependencies for HTTP handlers.
type Handlers struct {
	Dispatch *service.Dispatch
}

// NewHandlers creates a handler set.
func NewHandlers(d *service.Dispatch) *Handlers { return &Handlers{Dispatch: d} }

// ----- shared helpers -----

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

type errorBody struct {
	Error string `json:"error"`
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: err.Error()})
	case errors.Is(err, domain.ErrDuplicate):
		writeJSON(w, http.StatusConflict, errorBody{Error: err.Error()})
	case errors.Is(err, domain.ErrInvalidState),
		errors.Is(err, domain.ErrCabinFrozen),
		errors.Is(err, domain.ErrCabinBusy),
		errors.Is(err, domain.ErrInspectorUncertified),
		errors.Is(err, domain.ErrOutOfInspectionWindow),
		errors.Is(err, domain.ErrAnomalyEscalated),
		errors.Is(err, domain.ErrWorkOrderNotReviewed),
		errors.Is(err, domain.ErrInspectionNotSigned),
		errors.Is(err, domain.ErrBlackStartNotAuthed),
		errors.Is(err, domain.ErrBlackStartConflict),
		errors.Is(err, domain.ErrSyncNotVerified),
		errors.Is(err, domain.ErrSyncFaultActive):
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{Error: err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
	}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// ----- health -----

func (h *Handlers) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ----- cabins -----

type seedCabinReq struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SiteID      string `json:"site_id"`
	CapacityKWh int    `json:"capacity_kwh"`
}

func (h *Handlers) seedCabin(w http.ResponseWriter, r *http.Request) {
	var req seedCabinReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	c := domain.NewCabin(req.ID, req.Name, req.SiteID, req.CapacityKWh, h.Dispatch.Clock.Now())
	if err := h.Dispatch.Store.SaveCabin(c); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *Handlers) listCabins(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Dispatch.Cabins())
}

func (h *Handlers) getCabin(w http.ResponseWriter, r *http.Request) {
	c, err := h.Dispatch.Cabin(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// ----- inspections -----

type dispatchInspectionReq struct {
	CabinID       string `json:"cabin_id"`
	InspectorID   string `json:"inspector_id"`
	InspectorName string `json:"inspector_name"`
	CertNo        string `json:"cert_no"`
	CertExpiry    string `json:"cert_expiry"`
}

func (h *Handlers) dispatchInspection(w http.ResponseWriter, r *http.Request) {
	var req dispatchInspectionReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	insp := domain.Inspector{ID: req.InspectorID, Name: req.InspectorName, CertNo: req.CertNo}
	if req.CertExpiry != "" {
		if t, err := parseTime(req.CertExpiry); err == nil {
			insp.CertExpiry = t
		}
	}
	in, err := h.Dispatch.DispatchInspection(req.CabinID, insp)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, in)
}

func (h *Handlers) listInspections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Dispatch.Inspections())
}

func (h *Handlers) getInspection(w http.ResponseWriter, r *http.Request) {
	in, err := h.Dispatch.Inspection(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (h *Handlers) startInspection(w http.ResponseWriter, r *http.Request) {
	in, err := h.Dispatch.StartInspection(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

type photoReq struct {
	Ref string `json:"ref"`
}

func (h *Handlers) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	var req photoReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	in, err := h.Dispatch.UploadPhoto(r.PathValue("id"), req.Ref)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

type reviewReq struct {
	InspectorID string `json:"inspector_id"`
	Note        string `json:"note"`
}

func (h *Handlers) reviewInspection(w http.ResponseWriter, r *http.Request) {
	var req reviewReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	in, err := h.Dispatch.ReviewInspection(r.PathValue("id"), req.InspectorID, req.Note)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (h *Handlers) completeInspection(w http.ResponseWriter, r *http.Request) {
	in, err := h.Dispatch.CompleteInspection(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// ----- anomalies -----

type reportAnomalyReq struct {
	CabinID     string `json:"cabin_id"`
	Level       int    `json:"level"`
	Description string `json:"description"`
	Reporter    string `json:"reporter"`
}

func (h *Handlers) reportAnomaly(w http.ResponseWriter, r *http.Request) {
	var req reportAnomalyReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	a, err := h.Dispatch.ReportAnomaly(req.CabinID, domain.AnomalyLevel(req.Level), req.Description, req.Reporter)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (h *Handlers) listAnomalies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Dispatch.Anomalies())
}

func (h *Handlers) getAnomaly(w http.ResponseWriter, r *http.Request) {
	a, err := h.Dispatch.Anomaly(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type ackReq struct {
	By string `json:"by"`
}

func (h *Handlers) ackAnomaly(w http.ResponseWriter, r *http.Request) {
	var req ackReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	a, err := h.Dispatch.AcknowledgeAnomaly(r.PathValue("id"), req.By)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type isolateReq struct {
	FaultCode string `json:"fault_code"`
}

func (h *Handlers) isolateAnomaly(w http.ResponseWriter, r *http.Request) {
	var req isolateReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	a, err := h.Dispatch.IsolateAnomaly(r.PathValue("id"), req.FaultCode)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handlers) resolveAnomaly(w http.ResponseWriter, r *http.Request) {
	a, err := h.Dispatch.ResolveAnomaly(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// ----- work orders -----

type openWorkOrderReq struct {
	CabinID       string `json:"cabin_id"`
	AnomalyID     string `json:"anomaly_id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Assignee      string `json:"assignee"`
	InspectorID   string `json:"inspector_id"`
	InspectorName string `json:"inspector_name"`
	CertNo        string `json:"cert_no"`
	CertExpiry    string `json:"cert_expiry"`
}

func (h *Handlers) openWorkOrder(w http.ResponseWriter, r *http.Request) {
	var req openWorkOrderReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	insp := domain.Inspector{ID: req.InspectorID, Name: req.InspectorName, CertNo: req.CertNo}
	if req.CertExpiry != "" {
		if t, err := parseTime(req.CertExpiry); err == nil {
			insp.CertExpiry = t
		}
	}
	wo, err := h.Dispatch.OpenWorkOrder(req.CabinID, req.AnomalyID, req.Title, req.Description, req.Assignee, insp)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, wo)
}

func (h *Handlers) listWorkOrders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Dispatch.WorkOrders())
}

func (h *Handlers) getWorkOrder(w http.ResponseWriter, r *http.Request) {
	wo, err := h.Dispatch.WorkOrder(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wo)
}

func (h *Handlers) startWorkOrder(w http.ResponseWriter, r *http.Request) {
	wo, err := h.Dispatch.StartWorkOrder(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wo)
}

func (h *Handlers) reviewWorkOrder(w http.ResponseWriter, r *http.Request) {
	var req reviewReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	wo, err := h.Dispatch.ReviewWorkOrder(r.PathValue("id"), req.InspectorID, req.Note)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wo)
}

func (h *Handlers) closeWorkOrder(w http.ResponseWriter, r *http.Request) {
	wo, err := h.Dispatch.CloseWorkOrder(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wo)
}

// ----- black starts -----

type requestBlackStartReq struct {
	CabinIDs  []string `json:"cabin_ids"`
	Requester string   `json:"requester"`
}

func (h *Handlers) requestBlackStart(w http.ResponseWriter, r *http.Request) {
	var req requestBlackStartReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	bs, err := h.Dispatch.RequestBlackStart(req.CabinIDs, req.Requester)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, bs)
}

func (h *Handlers) listBlackStarts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Dispatch.BlackStarts())
}

func (h *Handlers) getBlackStart(w http.ResponseWriter, r *http.Request) {
	bs, err := h.Dispatch.BlackStart(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

type authorizeReq struct {
	Role      string `json:"role"`
	Principal string `json:"principal"`
}

func (h *Handlers) authorizeBlackStart(w http.ResponseWriter, r *http.Request) {
	var req authorizeReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	bs, err := h.Dispatch.AuthorizeBlackStart(r.PathValue("id"), domain.AuthRole(req.Role), req.Principal)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

func (h *Handlers) executeBlackStart(w http.ResponseWriter, r *http.Request) {
	bs, err := h.Dispatch.ExecuteBlackStart(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

type rollbackReq struct {
	Reason string `json:"reason"`
}

func (h *Handlers) rollbackBlackStart(w http.ResponseWriter, r *http.Request) {
	var req rollbackReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	bs, err := h.Dispatch.RollbackBlackStart(r.PathValue("id"), req.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, bs)
}

// ----- grid sync -----

type initiateGridSyncReq struct {
	BlackStartID         string  `json:"black_start_id"`
	MaxVoltageDeltaVolts float64 `json:"max_voltage_delta_volts"`
	MaxFrequencyDeltaHz  float64 `json:"max_frequency_delta_hz"`
	MaxPhaseDeltaDeg     float64 `json:"max_phase_delta_deg"`
}

func (h *Handlers) initiateGridSync(w http.ResponseWriter, r *http.Request) {
	var req initiateGridSyncReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	th := domain.DefaultSyncThresholds
	if req.MaxVoltageDeltaVolts > 0 {
		th.MaxVoltageDeltaVolts = req.MaxVoltageDeltaVolts
	}
	if req.MaxFrequencyDeltaHz > 0 {
		th.MaxFrequencyDeltaHz = req.MaxFrequencyDeltaHz
	}
	if req.MaxPhaseDeltaDeg > 0 {
		th.MaxPhaseDeltaDeg = req.MaxPhaseDeltaDeg
	}
	g, err := h.Dispatch.InitiateGridSync(req.BlackStartID, th)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *Handlers) listGridSyncs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Dispatch.GridSyncs())
}

func (h *Handlers) getGridSync(w http.ResponseWriter, r *http.Request) {
	g, err := h.Dispatch.GridSync(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handlers) beginSyncing(w http.ResponseWriter, r *http.Request) {
	g, err := h.Dispatch.BeginSyncing(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

type verifyReq struct {
	GridVoltageVolts  float64 `json:"grid_voltage_volts"`
	LocalVoltageVolts float64 `json:"local_voltage_volts"`
	GridFrequencyHz   float64 `json:"grid_frequency_hz"`
	LocalFrequencyHz  float64 `json:"local_frequency_hz"`
	PhaseDeltaDeg     float64 `json:"phase_delta_deg"`
}

func (h *Handlers) verifySync(w http.ResponseWriter, r *http.Request) {
	var req verifyReq
	if err := decode(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	m := domain.SyncMeasurement{
		GridVoltageVolts:  req.GridVoltageVolts,
		LocalVoltageVolts: req.LocalVoltageVolts,
		GridFrequencyHz:   req.GridFrequencyHz,
		LocalFrequencyHz:  req.LocalFrequencyHz,
		PhaseDeltaDeg:     req.PhaseDeltaDeg,
		At:                h.Dispatch.Clock.Now(),
	}
	g, err := h.Dispatch.VerifySync(r.PathValue("id"), m)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"grid_sync": g,
			"error":     err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handlers) connectGrid(w http.ResponseWriter, r *http.Request) {
	g, err := h.Dispatch.ConnectGrid(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Handlers) beginRetest(w http.ResponseWriter, r *http.Request) {
	g, err := h.Dispatch.BeginRetest(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
