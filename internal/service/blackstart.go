package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

// RequestBlackStart creates a requested black-start operation over the given
// cabins. Only one non-terminal black start may exist at a time.
func (d *Dispatch) RequestBlackStart(cabinIDs []string, requester string) (*domain.BlackStart, error) {
	if active := d.Store.ActiveBlackStart(); active != nil {
		return nil, fmt.Errorf("black start %s is already active (%s)", active.ID, active.State)
	}
	for _, id := range cabinIDs {
		if _, err := d.Store.Cabin(id); err != nil {
			return nil, fmt.Errorf("request black start: %w", err)
		}
	}
	bs := domain.NewBlackStart(d.IDs.NewID("bs"), cabinIDs, requester, d.Clock.Now())
	return bs, d.Store.SaveBlackStart(bs)
}

// AuthorizeBlackStart records a dual-authorization principal. Both the duty
// officer and the maintenance lead must authorize before execution.
func (d *Dispatch) AuthorizeBlackStart(id string, role domain.AuthRole, principal string) (*domain.BlackStart, error) {
	bs, err := d.Store.BlackStart(id)
	if err != nil {
		return nil, err
	}
	if err := bs.Authorize(role, principal, d.Clock.Now()); err != nil {
		return nil, err
	}
	return bs, d.Store.SaveBlackStart(bs)
}

// ExecuteBlackStart runs the restoration sequence to completion. It blocks
// until every participating cabin is either started or excluded (because it was
// isolated by a concurrent anomaly). The context can cancel a long pause.
//
// All access to the shared BlackStart value is serialized under bsMu so that a
// concurrent anomaly report cannot race with the execution loop.
func (d *Dispatch) ExecuteBlackStart(ctx context.Context, id string) (*domain.BlackStart, error) {
	bs, err := d.Store.BlackStart(id)
	if err != nil {
		return nil, err
	}

	// Register as the active executing black start under bsMu.
	d.bsMu.Lock()
	if err := bs.BeginExecution(d.Clock.Now()); err != nil {
		d.bsMu.Unlock()
		return nil, err
	}
	if d.activeBS != nil && d.activeBS.ID != bs.ID && !d.activeBS.IsTerminal() {
		d.bsMu.Unlock()
		return nil, fmt.Errorf("another black start is executing")
	}
	d.activeBS = bs
	d.bsMu.Unlock()
	defer func() {
		d.bsMu.Lock()
		if d.activeBS == bs {
			d.activeBS = nil
		}
		d.bsMu.Unlock()
	}()

	_ = d.Store.SaveBlackStart(bs)

	// Cancellation watcher: abort the execution loop on ctx cancel. Closing
	// stop unblocks both the per-cabin step-delay select and waitForCabinReady;
	// the broadcast wakes any loop currently parked on bsCond.Wait().
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			close(stop)
			d.bsMu.Lock()
			d.bsPaused = false
			d.bsCond.Broadcast()
			d.bsMu.Unlock()
		case <-done:
		}
	}()
	defer close(done)

	for {
		d.bsMu.Lock()
		remaining := bs.RemainingCabinIDs()
		d.bsMu.Unlock()
		if len(remaining) == 0 {
			break
		}
		cabinID := remaining[0]

		// Pause if an anomaly is isolating this cabin right now.
		if !d.waitForCabinReady(stop, bs, cabinID) {
			d.bsMu.Lock()
			bs.Fail("cancelled", d.Clock.Now())
			d.bsMu.Unlock()
			_ = d.Store.SaveBlackStart(bs)
			return bs, ctx.Err()
		}
		// Model the per-cabin restoration hold (energizing + verification).
		if d.StepDelay > 0 {
			select {
			case <-time.After(d.StepDelay):
			case <-stop:
				d.bsMu.Lock()
				bs.Fail("cancelled", d.Clock.Now())
				d.bsMu.Unlock()
				_ = d.Store.SaveBlackStart(bs)
				return bs, ctx.Err()
			}
		}

		cabin, err := d.Store.Cabin(cabinID)
		// Decide start vs. exclude under bsMu so the decision is consistent
		// with any concurrent anomaly-driven isolation.
		d.bsMu.Lock()
		if err != nil {
			bs.MarkCabinExcluded(cabinID)
		} else if cabin.IsFrozen() {
			// Faulted/isolated cabin: exclude to prevent misoperation.
			bs.MarkCabinExcluded(cabinID)
		} else {
			bs.MarkCabinStarted(cabinID)
		}
		d.bsMu.Unlock()
		_ = d.Store.SaveBlackStart(bs)
	}

	d.bsMu.Lock()
	if err := bs.Complete(d.Clock.Now()); err != nil {
		d.bsMu.Unlock()
		return nil, err
	}
	d.bsMu.Unlock()
	return bs, d.Store.SaveBlackStart(bs)
}

// RollbackBlackStart aborts an operation and reverts it.
func (d *Dispatch) RollbackBlackStart(id, reason string) (*domain.BlackStart, error) {
	bs, err := d.Store.BlackStart(id)
	if err != nil {
		return nil, err
	}
	bs.Rollback(reason, d.Clock.Now())
	return bs, d.Store.SaveBlackStart(bs)
}

// BlackStart returns a black start by ID.
func (d *Dispatch) BlackStart(id string) (*domain.BlackStart, error) { return d.Store.BlackStart(id) }

// BlackStarts returns all black starts.
func (d *Dispatch) BlackStarts() []*domain.BlackStart { return d.Store.BlackStarts() }

// BlackStartExecuting reports whether a black start is currently executing.
func (d *Dispatch) BlackStartExecuting() bool {
	d.bsMu.Lock()
	defer d.bsMu.Unlock()
	return d.activeBS != nil && d.activeBS.State == domain.BlackStartExecuting
}
