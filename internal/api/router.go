// Package api exposes the dispatch center over HTTP using the standard library
// ServeMux with method+path patterns. Handlers translate JSON requests into
// service calls and JSON responses, mapping domain errors to status codes.
package api

import (
	"net/http"
)

// New returns the HTTP handler for the dispatch center.
func New(h *Handlers) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /api/cabins", h.listCabins)
	mux.HandleFunc("GET /api/cabins/{id}", h.getCabin)
	mux.HandleFunc("POST /api/cabins", h.seedCabin)

	mux.HandleFunc("GET /api/inspections", h.listInspections)
	mux.HandleFunc("POST /api/inspections", h.dispatchInspection)
	mux.HandleFunc("GET /api/inspections/{id}", h.getInspection)
	mux.HandleFunc("POST /api/inspections/{id}/start", h.startInspection)
	mux.HandleFunc("POST /api/inspections/{id}/photo", h.uploadPhoto)
	mux.HandleFunc("POST /api/inspections/{id}/review", h.reviewInspection)
	mux.HandleFunc("POST /api/inspections/{id}/complete", h.completeInspection)

	mux.HandleFunc("GET /api/anomalies", h.listAnomalies)
	mux.HandleFunc("POST /api/anomalies", h.reportAnomaly)
	mux.HandleFunc("GET /api/anomalies/{id}", h.getAnomaly)
	mux.HandleFunc("POST /api/anomalies/{id}/ack", h.ackAnomaly)
	mux.HandleFunc("POST /api/anomalies/{id}/isolate", h.isolateAnomaly)
	mux.HandleFunc("POST /api/anomalies/{id}/resolve", h.resolveAnomaly)

	mux.HandleFunc("GET /api/workorders", h.listWorkOrders)
	mux.HandleFunc("POST /api/workorders", h.openWorkOrder)
	mux.HandleFunc("GET /api/workorders/{id}", h.getWorkOrder)
	mux.HandleFunc("POST /api/workorders/{id}/start", h.startWorkOrder)
	mux.HandleFunc("POST /api/workorders/{id}/review", h.reviewWorkOrder)
	mux.HandleFunc("POST /api/workorders/{id}/close", h.closeWorkOrder)

	mux.HandleFunc("GET /api/blackstarts", h.listBlackStarts)
	mux.HandleFunc("POST /api/blackstarts", h.requestBlackStart)
	mux.HandleFunc("GET /api/blackstarts/{id}", h.getBlackStart)
	mux.HandleFunc("POST /api/blackstarts/{id}/authorize", h.authorizeBlackStart)
	mux.HandleFunc("POST /api/blackstarts/{id}/execute", h.executeBlackStart)
	mux.HandleFunc("POST /api/blackstarts/{id}/rollback", h.rollbackBlackStart)

	mux.HandleFunc("GET /api/gridsyncs", h.listGridSyncs)
	mux.HandleFunc("POST /api/gridsyncs", h.initiateGridSync)
	mux.HandleFunc("GET /api/gridsyncs/{id}", h.getGridSync)
	mux.HandleFunc("POST /api/gridsyncs/{id}/sync", h.beginSyncing)
	mux.HandleFunc("POST /api/gridsyncs/{id}/verify", h.verifySync)
	mux.HandleFunc("POST /api/gridsyncs/{id}/connect", h.connectGrid)
	mux.HandleFunc("POST /api/gridsyncs/{id}/retest", h.beginRetest)

	return mux
}
