package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/clock"
	"github.com/ejinagrid/ejinagrid/internal/domain"
	"github.com/ejinagrid/ejinagrid/internal/service"
	"github.com/ejinagrid/ejinagrid/internal/store"
)

func newTestDispatch(t *testing.T) (*service.Dispatch, *clock.Fake, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	st, err := store.New(path)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	clk := clock.NewFake(time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC))
	d := service.New(st, clk, &service.DefaultIDs{}, domain.InspectionWindow{StartHour: 8, EndHour: 10})
	_ = st.SaveCabin(domain.NewCabin("c1", "C1", "site", 1000, clk.Now()))
	return d, clk, path
}

func TestWorker_EscalatesOverdueAnomaly(t *testing.T) {
	d, clk, _ := newTestDispatch(t)
	a, err := d.ReportAnomaly("c1", domain.Level1, "overtemp", "ops")
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	// Within the deadline: worker escalates nothing.
	w := New(d, d.Store, clk, time.Second)
	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("runonce: %v", err)
	}
	if rep.EscalatedAnomalies != 0 {
		t.Fatalf("escalated = %d, want 0 within deadline", rep.EscalatedAnomalies)
	}
	// Past the 15-minute deadline: worker escalates it.
	clk.Advance(16 * time.Minute)
	rep, _ = w.RunOnce(context.Background())
	if rep.EscalatedAnomalies != 1 {
		t.Fatalf("escalated = %d, want 1", rep.EscalatedAnomalies)
	}
	got, _ := d.Anomaly(a.ID)
	if got.State != domain.AnomalyEscalated || !got.Escalated {
		t.Fatalf("anomaly = %+v, want escalated", got)
	}
	// Idempotent: running again escalates nothing new.
	rep, _ = w.RunOnce(context.Background())
	if rep.EscalatedAnomalies != 0 {
		t.Fatalf("re-run escalated = %d, want 0", rep.EscalatedAnomalies)
	}
}

func TestWorker_PreparesFailedSyncForRetest(t *testing.T) {
	d, _, _ := newTestDispatch(t)
	g, _ := d.InitiateGridSync("", domain.DefaultSyncThresholds)
	_, _ = d.BeginSyncing(g.ID)
	bad := domain.SyncMeasurement{GridVoltageVolts: 400, LocalVoltageVolts: 410, PhaseDeltaDeg: 1}
	_, _ = d.VerifySync(g.ID, bad)
	if got, _ := d.GridSync(g.ID); got.State != domain.GridSyncFailed {
		t.Fatalf("want failed, got %s", got.State)
	}
	w := New(d, d.Store, clock.System{}, time.Second)
	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("runonce: %v", err)
	}
	if rep.RetestsPrepared != 1 {
		t.Fatalf("retests prepared = %d, want 1", rep.RetestsPrepared)
	}
	got, _ := d.GridSync(g.ID)
	if got.State != domain.GridSyncRetesting {
		t.Fatalf("state = %s, want retesting", got.State)
	}
}

func TestWorker_SnapshotPersisted(t *testing.T) {
	d, _, path := newTestDispatch(t)
	w := New(d, d.Store, clock.System{}, time.Second)
	rep, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("runonce: %v", err)
	}
	if !rep.SnapshotSaved {
		t.Fatalf("snapshot not saved")
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("snapshot file missing/empty: %v", err)
	}
	// Reload a fresh store from the snapshot and verify state survived.
	st2, err := store.New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := st2.Cabin("c1"); err != nil {
		t.Fatalf("reloaded store missing c1: %v", err)
	}
}
