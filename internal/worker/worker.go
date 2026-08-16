// Package worker runs background dispatch tasks: escalation of overdue Level-1
// anomalies, retest preparation for failed grid synchronizations, and periodic
// persistence snapshots. All tasks are cancellable via context and individually
// runnable via RunOnce for deterministic testing.
package worker

import (
	"context"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/clock"
	"github.com/ejinagrid/ejinagrid/internal/service"
	"github.com/ejinagrid/ejinagrid/internal/store"
)

// Report summarizes what a single worker pass accomplished.
type Report struct {
	EscalatedAnomalies int    `json:"escalated_anomalies"`
	RetestsPrepared    int    `json:"retests_prepared"`
	SnapshotSaved      bool   `json:"snapshot_saved"`
	SnapshotErr        string `json:"snapshot_err,omitempty"`
}

// Worker drives the recurring dispatch background tasks.
type Worker struct {
	dispatch *service.Dispatch
	store    *store.Store
	clock    clock.Clock
	interval time.Duration
}

// New creates a worker with the given tick interval.
func New(d *service.Dispatch, s *store.Store, clk clock.Clock, interval time.Duration) *Worker {
	return &Worker{dispatch: d, store: s, clock: clk, interval: interval}
}

// Run loops until ctx is cancelled, performing RunOnce on every tick.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Persist final state on shutdown.
			_ = w.store.Snapshot()
			return
		case <-ticker.C:
			_, _ = w.RunOnce(ctx)
		}
	}
}

// RunOnce performs one pass of escalation, retest preparation and snapshotting.
func (w *Worker) RunOnce(ctx context.Context) (Report, error) {
	var rep Report

	// Escalate Level-1 anomalies past their 15-minute acknowledgement deadline.
	for _, a := range w.dispatch.AnomaliesNeedingEscalation() {
		if _, err := w.dispatch.EscalateAnomaly(a.ID); err == nil {
			rep.EscalatedAnomalies++
		}
	}

	// Prepare failed grid-sync operations for a retest so the retry can proceed.
	for _, g := range w.dispatch.SyncFailuresNeedingRetest() {
		if _, err := w.dispatch.BeginRetest(g.ID); err == nil {
			rep.RetestsPrepared++
		}
	}

	// Persist current state.
	if err := w.store.Snapshot(); err != nil {
		rep.SnapshotErr = err.Error()
		return rep, err
	}
	rep.SnapshotSaved = true
	return rep, nil
}
