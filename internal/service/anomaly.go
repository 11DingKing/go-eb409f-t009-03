package service

import (
	"fmt"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

// ReportAnomaly records an equipment anomaly. A Level-1 anomaly freezes the
// cabin and its open inspections, and—if the cabin participates in an executing
// black start—pauses that black start so the fault is isolated first.
func (d *Dispatch) ReportAnomaly(cabinID string, level domain.AnomalyLevel, desc, reporter string) (*domain.Anomaly, error) {
	if _, err := d.Store.Cabin(cabinID); err != nil {
		return nil, fmt.Errorf("report anomaly: %w", err)
	}
	now := d.Clock.Now()
	anomaly := domain.NewAnomaly(d.IDs.NewID("anom"), cabinID, level, desc, reporter, now)

	if anomaly.ShouldFreeze() {
		// Coordinate against any executing black start and freeze/isolate.
		d.isolateOrFreeze(cabinID, anomaly, now)
		// Freeze open inspections on the cabin.
		d.FreezeInspectionsForCabin(cabinID)
	} else {
		// Non-freezing levels still flag the cabin for the dispatcher.
		if cabin, err := d.Store.Cabin(cabinID); err == nil {
			_ = cabin.ReportFault(now)
			_ = d.Store.SaveCabin(cabin)
		}
	}

	if err := d.Store.SaveAnomaly(anomaly); err != nil {
		return nil, err
	}
	return anomaly, nil
}

// AcknowledgeAnomaly records duty-officer acknowledgement. For Level-1 anomalies
// this must happen within the 15-minute deadline or the anomaly is escalated.
func (d *Dispatch) AcknowledgeAnomaly(id, by string) (*domain.Anomaly, error) {
	a, err := d.Store.Anomaly(id)
	if err != nil {
		return nil, err
	}
	if err := a.Acknowledge(by, d.Clock.Now()); err != nil {
		_ = d.Store.SaveAnomaly(a)
		return nil, err
	}
	return a, d.Store.SaveAnomaly(a)
}

// IsolateAnomaly marks the fault as isolated with a recorded fault code and
// isolates the cabin electrically.
func (d *Dispatch) IsolateAnomaly(id, faultCode string) (*domain.Anomaly, error) {
	a, err := d.Store.Anomaly(id)
	if err != nil {
		return nil, err
	}
	if err := a.Isolate(faultCode, d.Clock.Now()); err != nil {
		return nil, err
	}
	if cabin, err := d.Store.Cabin(a.CabinID); err == nil {
		cabin.Isolate(d.Clock.Now())
		_ = d.Store.SaveCabin(cabin)
	}
	return a, d.Store.SaveAnomaly(a)
}

// ResolveAnomaly closes an anomaly after the cabin is repaired and verified,
// restoring the cabin to operational.
func (d *Dispatch) ResolveAnomaly(id string) (*domain.Anomaly, error) {
	a, err := d.Store.Anomaly(id)
	if err != nil {
		return nil, err
	}
	if err := a.Resolve(d.Clock.Now()); err != nil {
		return nil, err
	}
	if cabin, err := d.Store.Cabin(a.CabinID); err == nil {
		cabin.Restore(d.Clock.Now())
		_ = d.Store.SaveCabin(cabin)
	}
	return a, d.Store.SaveAnomaly(a)
}

// AnomaliesNeedingEscalation returns Level-1 anomalies still unacknowledged
// past their deadline. Used by the background worker.
func (d *Dispatch) AnomaliesNeedingEscalation() []*domain.Anomaly {
	now := d.Clock.Now()
	var out []*domain.Anomaly
	for _, a := range d.Store.Anomalies() {
		if a.Level == domain.Level1 && a.State == domain.AnomalyReported && now.After(a.AckDeadlineAt()) {
			out = append(out, a)
		}
	}
	return out
}

// EscalateAnomaly forces escalation of an overdue Level-1 anomaly and notifies
// the upper-level dispatcher (recorded as a flag).
func (d *Dispatch) EscalateAnomaly(id string) (*domain.Anomaly, error) {
	a, err := d.Store.Anomaly(id)
	if err != nil {
		return nil, err
	}
	if err := a.Escalate(); err != nil {
		return nil, err
	}
	return a, d.Store.SaveAnomaly(a)
}

// Anomaly returns an anomaly by ID.
func (d *Dispatch) Anomaly(id string) (*domain.Anomaly, error) { return d.Store.Anomaly(id) }

// Anomalies returns all anomalies.
func (d *Dispatch) Anomalies() []*domain.Anomaly { return d.Store.Anomalies() }
