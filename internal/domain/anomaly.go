package domain

import (
	"fmt"
	"time"
)

// AnomalyLevel is the severity tier of a reported equipment anomaly.
type AnomalyLevel int

const (
	// Level1 is critical: the cabin must be frozen and the duty officer notified
	// within 15 minutes.
	Level1 AnomalyLevel = 1
	// Level2 is major: requires a work order but does not freeze the cabin.
	Level2 AnomalyLevel = 2
	// Level3 is minor: logged for trend analysis.
	Level3 AnomalyLevel = 3
)

// AckDeadline is the maximum time the duty officer has to acknowledge a Level-1
// anomaly before it is auto-escalated.
const AckDeadline = 15 * time.Minute

// AnomalyState tracks the life cycle of an anomaly report.
type AnomalyState string

const (
	AnomalyReported     AnomalyState = "reported"
	AnomalyAcknowledged AnomalyState = "acknowledged"
	AnomalyIsolated     AnomalyState = "isolated"
	AnomalyResolved     AnomalyState = "resolved"
	AnomalyEscalated    AnomalyState = "escalated"
)

// Anomaly is a reported equipment abnormality on a cabin.
type Anomaly struct {
	ID             string       `json:"id"`
	CabinID        string       `json:"cabin_id"`
	Level          AnomalyLevel `json:"level"`
	Description    string       `json:"description"`
	Reporter       string       `json:"reporter"`
	ReportedAt     time.Time    `json:"reported_at"`
	AcknowledgedBy string       `json:"acknowledged_by"`
	AcknowledgedAt time.Time    `json:"acknowledged_at"`
	State          AnomalyState `json:"state"`
	Escalated      bool         `json:"escalated"`
	FaultCode      string       `json:"fault_code"`
	ResolvedAt     time.Time    `json:"resolved_at"`
}

// NewAnomaly creates a reported anomaly.
func NewAnomaly(id, cabinID string, level AnomalyLevel, desc, reporter string, now time.Time) *Anomaly {
	return &Anomaly{
		ID:          id,
		CabinID:     cabinID,
		Level:       level,
		Description: desc,
		Reporter:    reporter,
		ReportedAt:  now,
		State:       AnomalyReported,
	}
}

// ShouldFreeze reports whether the anomaly level forces a cabin freeze.
func (a *Anomaly) ShouldFreeze() bool { return a.Level == Level1 }

// AckDeadlineAt returns the wall-clock deadline by which the duty officer must
// acknowledge a Level-1 anomaly.
func (a *Anomaly) AckDeadlineAt() time.Time { return a.ReportedAt.Add(AckDeadline) }

// Acknowledge records duty-officer acknowledgement. It fails if the deadline
// has already passed (the anomaly is then escalated instead).
func (a *Anomaly) Acknowledge(by string, now time.Time) error {
	if a.State != AnomalyReported {
		return fmt.Errorf("%w: cannot acknowledge from %s", ErrInvalidState, a.State)
	}
	if a.Level == Level1 && now.After(a.AckDeadlineAt()) {
		a.Escalated = true
		a.State = AnomalyEscalated
		return ErrAnomalyEscalated
	}
	a.AcknowledgedBy = by
	a.AcknowledgedAt = now
	a.State = AnomalyAcknowledged
	return nil
}

// Escalate marks the anomaly as escalated past its acknowledgement deadline.
func (a *Anomaly) Escalate() error {
	if a.State != AnomalyReported {
		return fmt.Errorf("%w: cannot escalate from %s", ErrInvalidState, a.State)
	}
	a.Escalated = true
	a.State = AnomalyEscalated
	return nil
}

// Isolate records that the faulted cabin has been electrically isolated.
func (a *Anomaly) Isolate(faultCode string, now time.Time) error {
	if a.State == AnomalyResolved {
		return fmt.Errorf("%w: anomaly already resolved", ErrInvalidState)
	}
	a.FaultCode = faultCode
	a.State = AnomalyIsolated
	return nil
}

// Resolve closes the anomaly after the cabin is repaired and verified.
func (a *Anomaly) Resolve(now time.Time) error {
	if a.State == AnomalyResolved {
		return ErrDuplicate
	}
	a.State = AnomalyResolved
	a.ResolvedAt = now
	return nil
}

// IsTerminal reports whether the anomaly reached a terminal state.
func (a *Anomaly) IsTerminal() bool {
	return a.State == AnomalyResolved
}

// String returns the level as a stable label.
func (l AnomalyLevel) String() string {
	switch l {
	case Level1:
		return "1"
	case Level2:
		return "2"
	case Level3:
		return "3"
	default:
		return "?"
	}
}
