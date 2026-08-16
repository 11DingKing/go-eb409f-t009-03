package service

import (
	"fmt"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

// InitiateGridSync creates an off-grid synchronization operation bound to a
// completed black start (or a standalone restoration).
func (d *Dispatch) InitiateGridSync(blackStartID string, th domain.SyncThresholds) (*domain.GridSync, error) {
	if blackStartID != "" {
		bs, err := d.Store.BlackStart(blackStartID)
		if err != nil {
			return nil, fmt.Errorf("initiate grid sync: %w", err)
		}
		if bs.State != domain.BlackStartCompleted {
			return nil, fmt.Errorf("grid sync requires a completed black start, got %s", bs.State)
		}
	}
	g := domain.NewGridSync(d.IDs.NewID("sync"), blackStartID, th, d.Clock.Now())
	return g, d.Store.SaveGridSync(g)
}

// BeginSyncing starts the auto-synchronizer check for a grid-sync operation.
func (d *Dispatch) BeginSyncing(id string) (*domain.GridSync, error) {
	g, err := d.Store.GridSync(id)
	if err != nil {
		return nil, err
	}
	if err := g.BeginSyncing(d.Clock.Now()); err != nil {
		return nil, err
	}
	return g, d.Store.SaveGridSync(g)
}

// VerifySync records the synchronizer measurement. On success the operation is
// verified; on failure a fault code is recorded, the upper grid is notified and
// the system falls back to off-grid independent operation with the backup
// tie-line.
func (d *Dispatch) VerifySync(id string, m domain.SyncMeasurement) (*domain.GridSync, error) {
	g, err := d.Store.GridSync(id)
	if err != nil {
		return nil, err
	}
	if err := g.Verify(m, d.Clock.Now()); err != nil {
		_ = d.Store.SaveGridSync(g)
		return g, err
	}
	return g, d.Store.SaveGridSync(g)
}

// ConnectGrid closes the breaker to the grid after successful verification.
func (d *Dispatch) ConnectGrid(id string) (*domain.GridSync, error) {
	g, err := d.Store.GridSync(id)
	if err != nil {
		return nil, err
	}
	if err := g.Connect(d.Clock.Now()); err != nil {
		return nil, err
	}
	return g, d.Store.SaveGridSync(g)
}

// BeginRetest moves a failed grid sync into retesting after the condition is
// corrected and a fresh measurement is ready.
func (d *Dispatch) BeginRetest(id string) (*domain.GridSync, error) {
	g, err := d.Store.GridSync(id)
	if err != nil {
		return nil, err
	}
	if err := g.BeginRetest(d.Clock.Now()); err != nil {
		return nil, err
	}
	return g, d.Store.SaveGridSync(g)
}

// SyncFailuresNeedingRetest returns failed grid-sync operations that still need
// a retest. Used by the background worker to drive the retry loop.
func (d *Dispatch) SyncFailuresNeedingRetest() []*domain.GridSync {
	var out []*domain.GridSync
	for _, g := range d.Store.GridSyncs() {
		if g.State == domain.GridSyncFailed {
			out = append(out, g)
		}
	}
	return out
}

// GridSync returns a grid sync by ID.
func (d *Dispatch) GridSync(id string) (*domain.GridSync, error) { return d.Store.GridSync(id) }

// GridSyncs returns all grid syncs.
func (d *Dispatch) GridSyncs() []*domain.GridSync { return d.Store.GridSyncs() }
