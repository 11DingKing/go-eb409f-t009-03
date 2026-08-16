package service

import (
	"time"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

// isolateOrFreeze implements the concurrency boundary between an executing
// black start and a concurrent Level-1 anomaly report. When the reporting
// cabin participates in an executing black start and has not yet been started,
// the black start is paused, the cabin is electrically isolated (and the fault
// code recorded), and the black start is resumed so it skips the isolated cabin.
// Otherwise the cabin is simply frozen. This ordering—fault isolation before
// startup resumes—prevents energizing a faulted cabin.
func (d *Dispatch) isolateOrFreeze(cabinID string, anomaly *domain.Anomaly, now time.Time) {
	conflict := false
	d.bsMu.Lock()
	bs := d.activeBS
	if bs != nil && bs.State == domain.BlackStartExecuting &&
		bs.Participates(cabinID) && !bs.CabinStarted(cabinID) && !bs.CabinExcluded(cabinID) {
		_ = bs.Pause(cabinID, now)
		d.bsPaused = true
		d.bsPauseCabin = cabinID
		conflict = true
	}
	d.bsMu.Unlock()

	cabin, err := d.Store.Cabin(cabinID)
	if err != nil {
		return
	}
	if conflict {
		cabin.Isolate(now)
		_ = anomaly.Isolate(faultCodeForCabin(cabinID, anomaly.Level), now)
	} else {
		cabin.Freeze(now)
	}
	_ = d.Store.SaveCabin(cabin)

	if conflict {
		// Resume the black start so the execution loop proceeds past the now
		// isolated cabin (it will skip it). Fault isolation happened first.
		d.bsMu.Lock()
		_ = bs.Resume()
		d.bsPaused = false
		d.bsPauseCabin = ""
		d.bsCond.Broadcast()
		d.bsMu.Unlock()
	}
}

// faultCodeForCabin returns a stable fault code for a frozen cabin anomaly.
func faultCodeForCabin(cabinID string, level domain.AnomalyLevel) string {
	return "ANOM_L" + level.String() + "_" + cabinID
}

// waitForCabinReady blocks the black-start execution loop while an anomaly is
// being isolated on cabinID. It returns false if the stop signal fires first.
func (d *Dispatch) waitForCabinReady(stop <-chan struct{}, bs *domain.BlackStart, cabinID string) bool {
	d.bsMu.Lock()
	defer d.bsMu.Unlock()
	for d.bsPaused && d.bsPauseCabin == cabinID && d.activeBS == bs {
		select {
		case <-stop:
			return false
		default:
		}
		d.bsCond.Wait()
	}
	return true
}

// activeExecutingBlackStart returns the currently executing black start, if any.
func (d *Dispatch) activeExecutingBlackStart() *domain.BlackStart {
	d.bsMu.Lock()
	defer d.bsMu.Unlock()
	if d.activeBS != nil && (d.activeBS.State == domain.BlackStartExecuting ||
		d.activeBS.State == domain.BlackStartPaused) {
		return d.activeBS
	}
	return nil
}
