package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/clock"
	"github.com/ejinagrid/ejinagrid/internal/domain"
	"github.com/ejinagrid/ejinagrid/internal/service"
	"github.com/ejinagrid/ejinagrid/internal/store"
)

// newTestServer wires a dispatch with a fake clock inside the inspection window
// and three seeded cabins, returning the handlers and the fake clock.
func newTestServer(t *testing.T) (*Handlers, *clock.Fake) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(dir + "/state.json")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	clk := clock.NewFake(time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC))
	d := service.New(st, clk, &service.DefaultIDs{}, domain.InspectionWindow{StartHour: 8, EndHour: 10})
	for _, id := range []string{"c1", "c2", "c3"} {
		_ = st.SaveCabin(domain.NewCabin(id, "Cabin "+id, "site-1", 2500, clk.Now()))
	}
	return NewHandlers(d), clk
}

func do(t *testing.T, h http.Handler, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var out map[string]any
	if rr.Body.Len() > 0 {
		_ = json.Unmarshal(rr.Body.Bytes(), &out)
	}
	return rr.Code, out
}

func TestHTTP_Healthz(t *testing.T) {
	h, _ := newTestServer(t)
	code, body := do(t, New(h), "GET", "/healthz", nil)
	if code != http.StatusOK {
		t.Fatalf("code = %d, want 200", code)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v", body)
	}
}

func TestHTTP_InspectionFullFlow(t *testing.T) {
	h, _ := newTestServer(t)
	srv := New(h)

	code, body := do(t, srv, "POST", "/api/inspections", map[string]any{
		"cabin_id":       "c1",
		"inspector_id":   "insp-1",
		"inspector_name": "Alice",
		"cert_no":        "CERT-001",
		"cert_expiry":    "2027-01-01T00:00:00Z",
	})
	if code != http.StatusCreated {
		t.Fatalf("dispatch code = %d body=%v", code, body)
	}
	id := body["id"].(string)

	if code, _ := do(t, srv, "POST", "/api/inspections/"+id+"/start", nil); code != http.StatusOK {
		t.Fatalf("start code = %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/inspections/"+id+"/photo", map[string]any{"ref": "p/1.jpg"}); code != http.StatusOK {
		t.Fatalf("photo code = %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/inspections/"+id+"/review", map[string]any{"inspector_id": "insp-1", "note": "ok"}); code != http.StatusOK {
		t.Fatalf("review code = %d", code)
	}
	if code, b := do(t, srv, "POST", "/api/inspections/"+id+"/complete", nil); code != http.StatusOK {
		t.Fatalf("complete code = %d body=%v", code, b)
	}
	// Cabin restored to operational.
	_, cab := do(t, srv, "GET", "/api/cabins/c1", nil)
	if cab["status"] != "operational" {
		t.Fatalf("cabin status = %v, want operational", cab["status"])
	}
}

func TestHTTP_DispatchUncertifiedReturns422(t *testing.T) {
	h, _ := newTestServer(t)
	srv := New(h)
	code, body := do(t, srv, "POST", "/api/inspections", map[string]any{
		"cabin_id":     "c1",
		"inspector_id": "insp-1",
		"cert_no":      "", // uncertified
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d, want 422 (body=%v)", code, body)
	}
}

func TestHTTP_AnomalyLevel1FreezesCabin(t *testing.T) {
	h, _ := newTestServer(t)
	srv := New(h)
	code, body := do(t, srv, "POST", "/api/anomalies", map[string]any{
		"cabin_id":    "c2",
		"level":       1,
		"description": "overtemp",
		"reporter":    "ops",
	})
	if code != http.StatusCreated {
		t.Fatalf("report code = %d body=%v", code, body)
	}
	_, cab := do(t, srv, "GET", "/api/cabins/c2", nil)
	if cab["status"] != "frozen" {
		t.Fatalf("cabin status = %v, want frozen", cab["status"])
	}
}

func TestHTTP_WorkOrderCloseWithoutReviewReturns422(t *testing.T) {
	h, _ := newTestServer(t)
	srv := New(h)
	_, body := do(t, srv, "POST", "/api/workorders", map[string]any{
		"cabin_id":       "c1",
		"title":          "fix",
		"description":    "desc",
		"assignee":       "tech-1",
		"inspector_id":   "insp-1",
		"inspector_name": "Alice",
		"cert_no":        "CERT-001",
	})
	id := body["id"].(string)
	do(t, srv, "POST", "/api/workorders/"+id+"/start", nil)
	do(t, srv, "POST", "/api/workorders/"+id+"/review", map[string]any{"inspector_id": "insp-1", "note": "ok"})
	// Note: we did NOT submit for review, so close should still be rejected.
	code, _ := do(t, srv, "POST", "/api/workorders/"+id+"/close", nil)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("close code = %d, want 422 (needs submit_for_review first)", code)
	}
}

func TestHTTP_BlackStartDualAuthAndExecute(t *testing.T) {
	h, _ := newTestServer(t)
	srv := New(h)
	_, body := do(t, srv, "POST", "/api/blackstarts", map[string]any{
		"cabin_ids": []string{"c1", "c2"}, "requester": "ops",
	})
	id := body["id"].(string)
	// Execute before dual auth -> 422.
	if code, _ := do(t, srv, "POST", "/api/blackstarts/"+id+"/execute", nil); code != http.StatusUnprocessableEntity {
		t.Fatalf("execute without auth code = %d, want 422", code)
	}
	do(t, srv, "POST", "/api/blackstarts/"+id+"/authorize", map[string]any{"role": "duty_officer", "principal": "d1"})
	do(t, srv, "POST", "/api/blackstarts/"+id+"/authorize", map[string]any{"role": "maintenance_lead", "principal": "m1"})
	code, b := do(t, srv, "POST", "/api/blackstarts/"+id+"/execute", nil)
	if code != http.StatusOK {
		t.Fatalf("execute code = %d body=%v", code, b)
	}
	if b["state"] != "completed" {
		t.Fatalf("state = %v, want completed", b["state"])
	}
}

func TestHTTP_GridSyncVerifyFailFallback(t *testing.T) {
	h, _ := newTestServer(t)
	srv := New(h)
	// A standalone grid sync (no black start) to test the failure path.
	_, body := do(t, srv, "POST", "/api/gridsyncs", map[string]any{})
	id := body["id"].(string)
	do(t, srv, "POST", "/api/gridsyncs/"+id+"/sync", nil)
	code, b := do(t, srv, "POST", "/api/gridsyncs/"+id+"/verify", map[string]any{
		"grid_voltage_volts": 400, "local_voltage_volts": 400,
		"grid_frequency_hz": 50.0, "local_frequency_hz": 49.5, // out of band
		"phase_delta_deg": 1.0,
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("verify code = %d, want 422", code)
	}
	gs := b["grid_sync"].(map[string]any)
	if gs["state"] != "sync_failed" {
		t.Fatalf("state = %v, want sync_failed", gs["state"])
	}
	if gs["fault_code"] == "" || gs["upper_grid_notified"] != true || gs["backup_tie_line"] != true {
		t.Fatalf("failure fields missing: %v", gs)
	}
}
