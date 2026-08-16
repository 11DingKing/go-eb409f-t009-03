package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/clock"
	"github.com/ejinagrid/ejinagrid/internal/domain"
	"github.com/ejinagrid/ejinagrid/internal/store"
)

// newTestDispatch builds a Dispatch backed by a temp-file store and a fake
// clock anchored inside the daily inspection window, with three seeded cabins.
func newTestDispatch(t *testing.T) (*Dispatch, *clock.Fake) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.New(dir + "/state.json")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	clk := clock.NewFake(time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC))
	d := New(st, clk, &DefaultIDs{}, domain.InspectionWindow{StartHour: 8, EndHour: 10})
	for _, c := range []*domain.Cabin{
		domain.NewCabin("c1", "Cabin 1", "site-1", 2500, clk.Now()),
		domain.NewCabin("c2", "Cabin 2", "site-1", 2500, clk.Now()),
		domain.NewCabin("c3", "Cabin 3", "site-2", 2000, clk.Now()),
	} {
		if err := st.SaveCabin(c); err != nil {
			t.Fatalf("seed cabin: %v", err)
		}
	}
	return d, clk
}

func certifiedInspector() domain.Inspector {
	return domain.Inspector{
		ID:         "insp-1",
		Name:       "Alice",
		CertNo:     "CERT-2026-001",
		CertExpiry: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestInspection_FullHappyPath(t *testing.T) {
	d, _ := newTestDispatch(t)
	in, err := d.DispatchInspection("c1", certifiedInspector())
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if in.State != domain.InspectionDispatched {
		t.Fatalf("state = %s, want dispatched", in.State)
	}
	if _, err := d.StartInspection(in.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := d.UploadPhoto(in.ID, "photos/2026-08-16/c1-01.jpg"); err != nil {
		t.Fatalf("photo: %v", err)
	}
	if _, err := d.ReviewInspection(in.ID, "insp-1", "all readings nominal"); err != nil {
		t.Fatalf("review: %v", err)
	}
	got, err := d.CompleteInspection(in.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got.State != domain.InspectionCompleted {
		t.Fatalf("state = %s, want completed", got.State)
	}
	if len(got.Photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(got.Photos))
	}
	cabin, _ := d.Cabin("c1")
	if cabin.Status != domain.CabinStatusOperational {
		t.Fatalf("cabin status = %s, want operational", cabin.Status)
	}
}

func TestInspection_RejectUncertifiedInspector(t *testing.T) {
	d, _ := newTestDispatch(t)
	insp := certifiedInspector()
	insp.CertNo = "" // uncertified
	if _, err := d.DispatchInspection("c1", insp); !errors.Is(err, domain.ErrInspectorUncertified) {
		t.Fatalf("err = %v, want ErrInspectorUncertified", err)
	}
}

func TestInspection_RejectOutOfWindow(t *testing.T) {
	d, clk := newTestDispatch(t)
	clk.Set(time.Date(2026, 8, 16, 11, 0, 0, 0, time.UTC)) // past 10:00 window end
	if _, err := d.DispatchInspection("c1", certifiedInspector()); !errors.Is(err, domain.ErrOutOfInspectionWindow) {
		t.Fatalf("err = %v, want ErrOutOfInspectionWindow", err)
	}
}

func TestInspection_BlockedWhenCabinFrozen(t *testing.T) {
	d, _ := newTestDispatch(t)
	if _, err := d.ReportAnomaly("c1", domain.Level1, "battery overtemp", "ops"); err != nil {
		t.Fatalf("report anomaly: %v", err)
	}
	if _, err := d.DispatchInspection("c1", certifiedInspector()); !errors.Is(err, domain.ErrCabinFrozen) {
		t.Fatalf("err = %v, want ErrCabinFrozen", err)
	}
	cabin, _ := d.Cabin("c1")
	if !cabin.IsFrozen() {
		t.Fatalf("cabin should be frozen, got %s", cabin.Status)
	}
}

func TestAnomaly_Level1FreezesCabinAndInspections(t *testing.T) {
	d, _ := newTestDispatch(t)
	in, _ := d.DispatchInspection("c1", certifiedInspector())
	_, _ = d.StartInspection(in.ID)

	a, err := d.ReportAnomaly("c1", domain.Level1, "cell voltage imbalance", "ops")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if !a.ShouldFreeze() {
		t.Fatalf("level-1 should freeze")
	}
	cabin, _ := d.Cabin("c1")
	if cabin.Status != domain.CabinStatusFrozen {
		t.Fatalf("cabin = %s, want frozen", cabin.Status)
	}
	got, _ := d.Inspection(in.ID)
	if got.State != domain.InspectionFrozen {
		t.Fatalf("inspection = %s, want frozen", got.State)
	}
	// Completing the frozen inspection must not restore the cabin.
	_, _ = d.CompleteInspection(in.ID)
	cabin2, _ := d.Cabin("c1")
	if cabin2.Status == domain.CabinStatusOperational {
		t.Fatalf("cabin should remain frozen after frozen inspection complete")
	}
}

func TestAnomaly_AcknowledgeWithinDeadline(t *testing.T) {
	d, clk := newTestDispatch(t)
	a, err := d.ReportAnomaly("c1", domain.Level1, "smoke detected", "ops")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	// Advance just inside the 15-minute deadline.
	clk.Advance(14 * time.Minute)
	got, err := d.AcknowledgeAnomaly(a.ID, "duty-officer-1")
	if err != nil {
		t.Fatalf("ack: %v", err)
	}
	if got.State != domain.AnomalyAcknowledged {
		t.Fatalf("state = %s, want acknowledged", got.State)
	}
	if got.Escalated {
		t.Fatalf("should not be escalated within deadline")
	}
}

func TestAnomaly_AcknowledgeAfterDeadlineEscalates(t *testing.T) {
	d, clk := newTestDispatch(t)
	a, err := d.ReportAnomaly("c1", domain.Level1, "fire alarm", "ops")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	clk.Advance(16 * time.Minute) // past 15-minute deadline
	_, err = d.AcknowledgeAnomaly(a.ID, "duty-officer-1")
	if !errors.Is(err, domain.ErrAnomalyEscalated) {
		t.Fatalf("err = %v, want ErrAnomalyEscalated", err)
	}
	got, _ := d.Anomaly(a.ID)
	if !got.Escalated || got.State != domain.AnomalyEscalated {
		t.Fatalf("anomaly = %+v, want escalated", got)
	}
}

func TestWorkOrder_CloseRequiresReview(t *testing.T) {
	d, _ := newTestDispatch(t)
	wo, err := d.OpenWorkOrder("c1", "", "replace module", "cell module degraded", "tech-1", certifiedInspector())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, _ = d.StartWorkOrder(wo.ID)
	_, _ = d.SubmitWorkOrderForReview(wo.ID)
	// Close without review/signature must fail.
	if _, err := d.CloseWorkOrder(wo.ID); !errors.Is(err, domain.ErrWorkOrderNotReviewed) {
		t.Fatalf("err = %v, want ErrWorkOrderNotReviewed", err)
	}
	// Review by the wrong inspector must fail.
	if _, err := d.ReviewWorkOrder(wo.ID, "someone-else", "ok"); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("err = %v, want ErrInvalidState", err)
	}
	// Review by the assigned inspector, then close succeeds.
	if _, err := d.ReviewWorkOrder(wo.ID, "insp-1", "verified repaired"); err != nil {
		t.Fatalf("review: %v", err)
	}
	closed, err := d.CloseWorkOrder(wo.ID)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if closed.State != domain.WorkOrderClosed {
		t.Fatalf("state = %s, want closed", closed.State)
	}
}

func TestWorkOrder_IdempotentClose(t *testing.T) {
	d, _ := newTestDispatch(t)
	wo, _ := d.OpenWorkOrder("c1", "", "fix", "desc", "tech-1", certifiedInspector())
	_, _ = d.StartWorkOrder(wo.ID)
	_, _ = d.SubmitWorkOrderForReview(wo.ID)
	_, _ = d.ReviewWorkOrder(wo.ID, "insp-1", "ok")
	if _, err := d.CloseWorkOrder(wo.ID); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Closing an already-closed order is a no-op (ErrDuplicate).
	if _, err := d.CloseWorkOrder(wo.ID); !errors.Is(err, domain.ErrDuplicate) {
		t.Fatalf("second close err = %v, want ErrDuplicate", err)
	}
}

func TestBlackStart_DualAuthorizationRequired(t *testing.T) {
	d, _ := newTestDispatch(t)
	bs, err := d.RequestBlackStart([]string{"c1", "c2"}, "ops")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	// Execute with no authorization fails.
	if _, err := d.ExecuteBlackStart(context.Background(), bs.ID); !errors.Is(err, domain.ErrBlackStartNotAuthed) {
		t.Fatalf("err = %v, want ErrBlackStartNotAuthed", err)
	}
	// Only duty officer -> still not enough.
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleDutyOfficer, "duty-1")
	if _, err := d.ExecuteBlackStart(context.Background(), bs.ID); !errors.Is(err, domain.ErrBlackStartNotAuthed) {
		t.Fatalf("err = %v, want ErrBlackStartNotAuthed", err)
	}
	// Maintenance lead authorizes -> now dual-authorized.
	if _, err := d.AuthorizeBlackStart(bs.ID, domain.AuthRoleMaintenanceLead, "maint-1"); err != nil {
		t.Fatalf("maint auth: %v", err)
	}
	got, _ := d.BlackStart(bs.ID)
	if !got.FullyAuthorized() {
		t.Fatalf("expected fully authorized")
	}
}

func TestBlackStart_ExecuteCompletesAllCabins(t *testing.T) {
	d, _ := newTestDispatch(t)
	bs, _ := d.RequestBlackStart([]string{"c1", "c2", "c3"}, "ops")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleDutyOfficer, "duty-1")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleMaintenanceLead, "maint-1")
	got, err := d.ExecuteBlackStart(context.Background(), bs.ID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.State != domain.BlackStartCompleted {
		t.Fatalf("state = %s, want completed", got.State)
	}
	if len(got.StartedCabinIDs) != 3 || len(got.ExcludedCabinIDs) != 0 {
		t.Fatalf("started=%v excluded=%v, want all 3 started", got.StartedCabinIDs, got.ExcludedCabinIDs)
	}
}

func TestBlackStart_ExecuteSkipsPreIsolatedCabin(t *testing.T) {
	d, _ := newTestDispatch(t)
	// Isolate c2 before the black start runs (e.g. via a Level-1 anomaly).
	_, _ = d.ReportAnomaly("c2", domain.Level1, "fault", "ops")
	bs, _ := d.RequestBlackStart([]string{"c1", "c2", "c3"}, "ops")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleDutyOfficer, "duty-1")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleMaintenanceLead, "maint-1")
	got, err := d.ExecuteBlackStart(context.Background(), bs.ID)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got.State != domain.BlackStartCompleted {
		t.Fatalf("state = %s, want completed", got.State)
	}
	if got.CabinStarted("c2") {
		t.Fatalf("isolated cabin c2 must not be started")
	}
	if !got.CabinExcluded("c2") {
		t.Fatalf("c2 must be excluded")
	}
	if !got.CabinStarted("c1") || !got.CabinStarted("c3") {
		t.Fatalf("healthy cabins must be started: %v", got.StartedCabinIDs)
	}
}

func TestGridSync_VerifyPassThenConnect(t *testing.T) {
	d, _ := newTestDispatch(t)
	bs, _ := d.RequestBlackStart([]string{"c1"}, "ops")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleDutyOfficer, "duty-1")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleMaintenanceLead, "maint-1")
	_, _ = d.ExecuteBlackStart(context.Background(), bs.ID)

	g, err := d.InitiateGridSync(bs.ID, domain.DefaultSyncThresholds)
	if err != nil {
		t.Fatalf("initiate: %v", err)
	}
	if _, err := d.BeginSyncing(g.ID); err != nil {
		t.Fatalf("syncing: %v", err)
	}
	m := domain.SyncMeasurement{
		GridVoltageVolts: 400, LocalVoltageVolts: 401,
		GridFrequencyHz: 50.0, LocalFrequencyHz: 50.02,
		PhaseDeltaDeg: 2.0,
	}
	if _, err := d.VerifySync(g.ID, m); err != nil {
		t.Fatalf("verify: %v", err)
	}
	got, err := d.ConnectGrid(g.ID)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got.State != domain.GridSyncConnected {
		t.Fatalf("state = %s, want grid_connected", got.State)
	}
	if got.BackupTieLine {
		t.Fatalf("backup tie-line should be inactive after connect")
	}
}

func TestGridSync_VerifyFailRecordsFaultAndFallback(t *testing.T) {
	d, _ := newTestDispatch(t)
	g, _ := d.InitiateGridSync("", domain.DefaultSyncThresholds)
	_, _ = d.BeginSyncing(g.ID)
	// Frequency delta far outside the band.
	m := domain.SyncMeasurement{
		GridVoltageVolts: 400, LocalVoltageVolts: 400,
		GridFrequencyHz: 50.0, LocalFrequencyHz: 49.5,
		PhaseDeltaDeg: 1.0,
	}
	if _, err := d.VerifySync(g.ID, m); err == nil {
		t.Fatalf("expected verify failure")
	}
	got, _ := d.GridSync(g.ID)
	if got.State != domain.GridSyncFailed {
		t.Fatalf("state = %s, want sync_failed", got.State)
	}
	if got.FaultCode == "" {
		t.Fatalf("fault code must be recorded")
	}
	if !got.UpperGridNotified {
		t.Fatalf("upper grid must be notified on failure")
	}
	if !got.BackupTieLine {
		t.Fatalf("must fall back to backup tie-line")
	}
}

func TestGridSync_RetryAfterRetest(t *testing.T) {
	d, _ := newTestDispatch(t)
	g, _ := d.InitiateGridSync("", domain.DefaultSyncThresholds)
	_, _ = d.BeginSyncing(g.ID)
	bad := domain.SyncMeasurement{GridVoltageVolts: 400, LocalVoltageVolts: 410, PhaseDeltaDeg: 1}
	_, _ = d.VerifySync(g.ID, bad)
	if got, _ := d.GridSync(g.ID); got.State != domain.GridSyncFailed {
		t.Fatalf("want failed, got %s", got.State)
	}
	// Cannot re-sync while fault active.
	if _, err := d.BeginSyncing(g.ID); !errors.Is(err, domain.ErrSyncFaultActive) {
		t.Fatalf("err = %v, want ErrSyncFaultActive", err)
	}
	// Begin retest, then verify with a good measurement.
	if _, err := d.BeginRetest(g.ID); err != nil {
		t.Fatalf("retest: %v", err)
	}
	good := domain.SyncMeasurement{GridVoltageVolts: 400, LocalVoltageVolts: 400.5, GridFrequencyHz: 50, LocalFrequencyHz: 50.01, PhaseDeltaDeg: 1}
	if _, err := d.VerifySync(g.ID, good); err != nil {
		t.Fatalf("verify after retest: %v", err)
	}
	got, err := d.ConnectGrid(g.ID)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if got.State != domain.GridSyncConnected {
		t.Fatalf("state = %s, want grid_connected", got.State)
	}
	if got.FaultCode != "" {
		t.Fatalf("fault code should be cleared after successful retest")
	}
}

// TestCoordinator_ConflictIsolatesFaultedCabin exercises the concurrency
// boundary: a Level-1 anomaly on a participating cabin of an EXECUTING black
// start is isolated (not merely frozen) and the black start resumes.
func TestCoordinator_ConflictIsolatesFaultedCabin(t *testing.T) {
	d, clk := newTestDispatch(t)
	bs, _ := d.RequestBlackStart([]string{"c1", "c2", "c3"}, "ops")
	// AuthorizeBlackStart returns the authorized clone; reassign so the local
	// reference reflects the stored, fully-authorized value.
	bs, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleDutyOfficer, "duty-1")
	bs, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleMaintenanceLead, "maint-1")

	// Put the black start into executing and register it as active WITHOUT
	// running the loop, so we can observe the coordinator in isolation.
	if err := bs.BeginExecution(clk.Now()); err != nil {
		t.Fatalf("begin execution: %v", err)
	}
	d.bsMu.Lock()
	d.activeBS = bs
	d.bsMu.Unlock()
	_ = d.Store.SaveBlackStart(bs)
	t.Cleanup(func() {
		d.bsMu.Lock()
		d.activeBS = nil
		d.bsMu.Unlock()
	})

	a, err := d.ReportAnomaly("c2", domain.Level1, "cabin fire risk", "ops")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	cabin, _ := d.Cabin("c2")
	if cabin.Status != domain.CabinStatusIsolated {
		t.Fatalf("cabin = %s, want isolated (fault isolated before startup resumes)", cabin.Status)
	}
	if a.State != domain.AnomalyIsolated || a.FaultCode == "" {
		t.Fatalf("anomaly = %+v, want isolated with fault code", a)
	}
	// isolateOrFreeze pauses then resumes the active black start; it is not
	// re-saved (the execution loop would do that), so inspect the active object.
	if bs.State != domain.BlackStartExecuting {
		t.Fatalf("black start = %s, want executing (resumed after isolation)", bs.State)
	}
}

// TestBlackStart_ConcurrentAnomalyDuringExecution is the end-to-end concurrency
// test: a Level-1 anomaly is reported on the last cabin while the black start
// executes. The faulted cabin must be isolated and excluded, and the black start
// must complete without energizing it.
func TestBlackStart_ConcurrentAnomalyDuringExecution(t *testing.T) {
	d, _ := newTestDispatch(t)
	// A per-cabin restoration hold gives the concurrent report a deterministic
	// window to land before the loop reaches the last cabin.
	d.StepDelay = 20 * time.Millisecond

	bs, _ := d.RequestBlackStart([]string{"c1", "c2", "c3"}, "ops")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleDutyOfficer, "duty-1")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleMaintenanceLead, "maint-1")

	var (
		execRes *domain.BlackStart
		execErr error
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		execRes, execErr = d.ExecuteBlackStart(context.Background(), bs.ID)
	}()

	// Wait until the black start is executing, then report the anomaly on c3.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !d.BlackStartExecuting() {
		time.Sleep(2 * time.Millisecond)
	}
	if !d.BlackStartExecuting() {
		t.Fatal("black start never reached executing state")
	}
	if _, err := d.ReportAnomaly("c3", domain.Level1, "cabin smoke alarm", "ops"); err != nil {
		t.Fatalf("report: %v", err)
	}

	wg.Wait()
	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if execRes.State != domain.BlackStartCompleted {
		t.Fatalf("state = %s, want completed", execRes.State)
	}
	if execRes.CabinStarted("c3") {
		t.Fatalf("faulted cabin c3 must NOT be started")
	}
	if !execRes.CabinExcluded("c3") {
		t.Fatalf("faulted cabin c3 must be excluded")
	}
	cabin, _ := d.Cabin("c3")
	if cabin.Status != domain.CabinStatusIsolated {
		t.Fatalf("c3 = %s, want isolated", cabin.Status)
	}
	if !execRes.CabinStarted("c1") || !execRes.CabinStarted("c2") {
		t.Fatalf("healthy cabins must be started: %v", execRes.StartedCabinIDs)
	}
}

func TestBlackStart_CancelledExecution(t *testing.T) {
	d, _ := newTestDispatch(t)
	d.StepDelay = 200 * time.Millisecond // long hold so we can cancel mid-execution
	bs, _ := d.RequestBlackStart([]string{"c1", "c2"}, "ops")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleDutyOfficer, "duty-1")
	_, _ = d.AuthorizeBlackStart(bs.ID, domain.AuthRoleMaintenanceLead, "maint-1")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	got, err := d.ExecuteBlackStart(ctx, bs.ID)
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if got.State != domain.BlackStartFailed {
		t.Fatalf("state = %s, want failed", got.State)
	}
}
