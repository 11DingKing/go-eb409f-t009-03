package domain

import (
	"errors"
	"testing"
	"time"
)

func TestSyncMeasurement_WithinThresholds(t *testing.T) {
	th := SyncThresholds{MaxVoltageDeltaVolts: 5, MaxFrequencyDeltaHz: 0.1, MaxPhaseDeltaDeg: 5}
	pass := SyncMeasurement{GridVoltageVolts: 400, LocalVoltageVolts: 404.9, GridFrequencyHz: 50, LocalFrequencyHz: 50.05, PhaseDeltaDeg: 4.9}
	if err := pass.Within(th); err != nil {
		t.Fatalf("expected within, got %v", err)
	}
	over := SyncMeasurement{GridVoltageVolts: 400, LocalVoltageVolts: 400, GridFrequencyHz: 50, LocalFrequencyHz: 50.2, PhaseDeltaDeg: 1}
	if err := over.Within(th); err == nil {
		t.Fatalf("expected frequency failure")
	}
	g := NewGridSync("g", "", th, time.Now())
	_ = g.BeginSyncing(time.Now())
	if err := g.Verify(SyncMeasurement{PhaseDeltaDeg: 9, GridFrequencyHz: 50, LocalFrequencyHz: 50, GridVoltageVolts: 400, LocalVoltageVolts: 400}, time.Now()); err == nil {
		t.Fatalf("expected phase failure")
	}
	if g.FaultCode != "SYNC_PHASE_OUT_OF_BAND" {
		t.Fatalf("fault code = %q, want SYNC_PHASE_OUT_OF_BAND", g.FaultCode)
	}
	if !g.UpperGridNotified || !g.BackupTieLine {
		t.Fatalf("failure did not notify/fallback")
	}
}

func TestCabin_StateTransitions(t *testing.T) {
	now := time.Now()
	c := NewCabin("c", "C", "s", 1, now)
	if err := c.ReportFault(now); err != nil {
		t.Fatalf("report fault: %v", err)
	}
	if c.Status != CabinStatusFaultReported {
		t.Fatalf("status = %s", c.Status)
	}
	if err := c.BeginMaintenance(now); err != nil {
		t.Fatalf("begin maintenance from fault_reported: %v", err)
	}
	c.Restore(now)
	if c.Status != CabinStatusOperational {
		t.Fatalf("restore failed")
	}
	c.Freeze(now)
	if err := c.BeginInspection(now); !errors.Is(err, ErrCabinFrozen) {
		t.Fatalf("err = %v, want ErrCabinFrozen", err)
	}
}

func TestBlackStart_AuthorizeIdempotencyAndErrors(t *testing.T) {
	now := time.Now()
	b := NewBlackStart("b", []string{"c1"}, "ops", now)
	if err := b.Authorize(AuthRoleDutyOfficer, "d1", now); err != nil {
		t.Fatalf("first duty auth: %v", err)
	}
	if err := b.Authorize(AuthRoleDutyOfficer, "d1", now); err != nil {
		t.Fatalf("repeat duty auth: %v", err)
	}
	if err := b.Authorize(AuthRoleDutyOfficer, "d2", now); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("err = %v, want ErrInvalidState", err)
	}
	if b.FullyAuthorized() {
		t.Fatalf("should not be fully authorized yet")
	}
	if err := b.Authorize(AuthRoleMaintenanceLead, "m1", now); err != nil {
		t.Fatalf("maint auth: %v", err)
	}
	if !b.FullyAuthorized() {
		t.Fatalf("should be fully authorized")
	}
	if b.State != BlackStartDualAuthed {
		t.Fatalf("state = %s, want dual_authorized", b.State)
	}
	if err := b.Authorize(AuthRole("intern"), "x", now); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("err = %v, want ErrInvalidState", err)
	}
}

func TestAnomaly_AckDeadlineBoundary(t *testing.T) {
	reported := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	a := NewAnomaly("a", "c", Level1, "x", "r", reported)
	if err := a.Acknowledge("duty", reported.Add(AckDeadline)); err != nil {
		t.Fatalf("ack at deadline: %v", err)
	}
}
