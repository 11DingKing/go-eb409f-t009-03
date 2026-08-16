package domain

import (
	"fmt"
	"time"
)

// GridSyncState tracks the off-grid/on-grid switching life cycle.
type GridSyncState string

const (
	GridSyncOffGrid   GridSyncState = "off_grid"
	GridSyncSyncing   GridSyncState = "syncing"
	GridSyncVerified  GridSyncState = "verified"
	GridSyncConnected GridSyncState = "grid_connected"
	GridSyncFailed    GridSyncState = "sync_failed"
	GridSyncRetesting GridSyncState = "retesting"
)

// SyncThresholds define the auto-synchronizer acceptance bands. Voltage,
// frequency and phase difference must all be within bounds before connecting.
type SyncThresholds struct {
	MaxVoltageDeltaVolts float64
	MaxFrequencyDeltaHz  float64
	MaxPhaseDeltaDeg     float64
}

// DefaultSyncThresholds are conservative band defaults.
var DefaultSyncThresholds = SyncThresholds{
	MaxVoltageDeltaVolts: 5.0,
	MaxFrequencyDeltaHz:  0.1,
	MaxPhaseDeltaDeg:     5.0,
}

// SyncMeasurement is a single reading from the auto-synchronizer.
type SyncMeasurement struct {
	GridVoltageVolts  float64   `json:"grid_voltage_volts"`
	LocalVoltageVolts float64   `json:"local_voltage_volts"`
	GridFrequencyHz   float64   `json:"grid_frequency_hz"`
	LocalFrequencyHz  float64   `json:"local_frequency_hz"`
	PhaseDeltaDeg     float64   `json:"phase_delta_deg"`
	At                time.Time `json:"at"`
}

// Within returns nil if m is within t, otherwise an error describing the first
// exceeded parameter.
func (m SyncMeasurement) Within(t SyncThresholds) error {
	if d := abs(m.GridVoltageVolts - m.LocalVoltageVolts); d > t.MaxVoltageDeltaVolts {
		return fmt.Errorf("voltage delta %.2fV exceeds %.2fV", d, t.MaxVoltageDeltaVolts)
	}
	if d := abs(m.GridFrequencyHz - m.LocalFrequencyHz); d > t.MaxFrequencyDeltaHz {
		return fmt.Errorf("frequency delta %.3fHz exceeds %.3fHz", d, t.MaxFrequencyDeltaHz)
	}
	if d := abs(m.PhaseDeltaDeg); d > t.MaxPhaseDeltaDeg {
		return fmt.Errorf("phase delta %.2f° exceeds %.2f°", d, t.MaxPhaseDeltaDeg)
	}
	return nil
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// GridSync is an off-grid/on-grid switching operation bound to a black start.
type GridSync struct {
	ID                string          `json:"id"`
	BlackStartID      string          `json:"black_start_id"`
	Thresholds        SyncThresholds  `json:"thresholds"`
	State             GridSyncState   `json:"state"`
	Measurement       SyncMeasurement `json:"measurement,omitempty"`
	FaultCode         string          `json:"fault_code,omitempty"`
	UpperGridNotified bool            `json:"upper_grid_notified"`
	BackupTieLine     bool            `json:"backup_tie_line"`
	Attempt           int             `json:"attempt"`
	CreatedAt         time.Time       `json:"created_at"`
	ConnectedAt       time.Time       `json:"connected_at,omitempty"`
}

// NewGridSync creates an off-grid sync operation.
func NewGridSync(id, blackStartID string, th SyncThresholds, now time.Time) *GridSync {
	return &GridSync{
		ID:           id,
		BlackStartID: blackStartID,
		Thresholds:   th,
		State:        GridSyncOffGrid,
		CreatedAt:    now,
	}
}

// BeginSyncing starts the synchronizer check.
func (g *GridSync) BeginSyncing(now time.Time) error {
	if g.State == GridSyncConnected {
		return ErrDuplicate
	}
	if g.State == GridSyncFailed {
		return fmt.Errorf("%w: retest required", ErrSyncFaultActive)
	}
	g.State = GridSyncSyncing
	return nil
}

// Verify records the synchronizer reading. On success the operation is
// verified and may connect; on failure it records a fault code, notifies the
// upper grid and falls back to off-grid with the backup tie-line.
func (g *GridSync) Verify(m SyncMeasurement, now time.Time) error {
	if g.State != GridSyncSyncing && g.State != GridSyncRetesting {
		return fmt.Errorf("%w: verify from %s", ErrInvalidState, g.State)
	}
	g.Measurement = m
	if err := m.Within(g.Thresholds); err != nil {
		g.fail(err, now)
		return err
	}
	g.State = GridSyncVerified
	g.FaultCode = ""
	return nil
}

// Connect closes the breaker to the grid. Requires prior verification.
func (g *GridSync) Connect(now time.Time) error {
	if g.State == GridSyncConnected {
		return ErrDuplicate
	}
	if g.State != GridSyncVerified {
		return ErrSyncNotVerified
	}
	g.State = GridSyncConnected
	g.ConnectedAt = now
	g.BackupTieLine = false
	return nil
}

// fail records a synchronizer failure: sets the fault code, notifies the upper
// grid and falls back to off-grid independent operation with the backup tie-line.
func (g *GridSync) fail(err error, now time.Time) {
	g.State = GridSyncFailed
	g.FaultCode = faultCodeFor(err)
	g.UpperGridNotified = true
	g.BackupTieLine = true
	g.Attempt++
}

// FaultCode returns the recorded fault code, if any.
func (g *GridSync) Fault() string { return g.FaultCode }

// BeginRetest moves a failed sync into retesting so a fresh measurement can be
// taken after the condition is corrected.
func (g *GridSync) BeginRetest(now time.Time) error {
	if g.State != GridSyncFailed {
		return fmt.Errorf("%w: retest only after failure", ErrInvalidState)
	}
	g.State = GridSyncRetesting
	return nil
}

// IsTerminal reports whether the operation reached a terminal state.
func (g *GridSync) IsTerminal() bool {
	return g.State == GridSyncConnected
}

// faultCodeFor maps a synchronizer failure to a stable, machine-readable code.
func faultCodeFor(err error) string {
	msg := err.Error()
	switch {
	case contains(msg, "voltage"):
		return "SYNC_VOLTAGE_OUT_OF_BAND"
	case contains(msg, "frequency"):
		return "SYNC_FREQ_OUT_OF_BAND"
	case contains(msg, "phase"):
		return "SYNC_PHASE_OUT_OF_BAND"
	default:
		return "SYNC_UNKNOWN"
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
