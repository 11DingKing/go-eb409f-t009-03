package domain

import (
	"fmt"
	"time"
)

// BlackStartState tracks the life cycle of a microgrid black-start operation.
type BlackStartState string

const (
	BlackStartRequested  BlackStartState = "requested"
	BlackStartDutyAuthed BlackStartState = "duty_authorized"
	BlackStartDualAuthed BlackStartState = "dual_authorized"
	BlackStartExecuting  BlackStartState = "executing"
	BlackStartPaused     BlackStartState = "paused"
	BlackStartCompleted  BlackStartState = "completed"
	BlackStartFailed     BlackStartState = "failed"
	BlackStartRolledBack BlackStartState = "rolled_back"
)

// AuthRole identifies an authorizing principal for a black start.
type AuthRole string

const (
	AuthRoleDutyOfficer     AuthRole = "duty_officer"
	AuthRoleMaintenanceLead AuthRole = "maintenance_lead"
)

// BlackStart is a system-wide restoration operation that brings participating
// cabins back online after a total loss of supply. It requires dual
// authorization and can be paused when a participating cabin reports an
// anomaly, so the fault is isolated before startup resumes.
type BlackStart struct {
	ID               string          `json:"id"`
	CabinIDs         []string        `json:"cabin_ids"`
	Requester        string          `json:"requester"`
	DutyAuthBy       string          `json:"duty_auth_by"`
	DutyAuthAt       time.Time       `json:"duty_auth_at"`
	MaintAuthBy      string          `json:"maint_auth_by"`
	MaintAuthAt      time.Time       `json:"maint_auth_at"`
	State            BlackStartState `json:"state"`
	RequestedAt      time.Time       `json:"requested_at"`
	StartedAt        time.Time       `json:"started_at"`
	CompletedAt      time.Time       `json:"completed_at"`
	StartedCabinIDs  []string        `json:"started_cabin_ids"`
	ExcludedCabinIDs []string        `json:"excluded_cabin_ids"`
	PausedForCabin   string          `json:"paused_for_cabin,omitempty"`
	FailReason       string          `json:"fail_reason,omitempty"`
}

// NewBlackStart creates a requested black start over the given cabins.
func NewBlackStart(id string, cabinIDs []string, requester string, now time.Time) *BlackStart {
	return &BlackStart{
		ID:               id,
		CabinIDs:         append([]string{}, cabinIDs...),
		Requester:        requester,
		State:            BlackStartRequested,
		RequestedAt:      now,
		StartedCabinIDs:  []string{},
		ExcludedCabinIDs: []string{},
	}
}

// Authorize records an authorization by role. Either role may authorize first,
// but Execute requires both. Re-authorizing the same role is idempotent.
func (b *BlackStart) Authorize(role AuthRole, principal string, now time.Time) error {
	switch b.State {
	case BlackStartRequested, BlackStartDutyAuthed, BlackStartDualAuthed:
	default:
		return fmt.Errorf("%w: cannot authorize from %s", ErrInvalidState, b.State)
	}
	switch role {
	case AuthRoleDutyOfficer:
		if b.DutyAuthBy == "" {
			b.DutyAuthBy = principal
			b.DutyAuthAt = now
		} else if b.DutyAuthBy != principal {
			return fmt.Errorf("%w: duty officer already authorized", ErrInvalidState)
		}
	case AuthRoleMaintenanceLead:
		if b.MaintAuthBy == "" {
			b.MaintAuthBy = principal
			b.MaintAuthAt = now
		} else if b.MaintAuthBy != principal {
			return fmt.Errorf("%w: maintenance lead already authorized", ErrInvalidState)
		}
	default:
		return fmt.Errorf("%w: unknown role %s", ErrInvalidState, role)
	}
	if b.DutyAuthBy != "" && b.MaintAuthBy != "" {
		b.State = BlackStartDualAuthed
	} else {
		b.State = BlackStartDutyAuthed
	}
	return nil
}

// FullyAuthorized reports whether both principals have authorized.
func (b *BlackStart) FullyAuthorized() bool {
	return b.DutyAuthBy != "" && b.MaintAuthBy != ""
}

// BeginExecution transitions to executing. Requires dual authorization.
func (b *BlackStart) BeginExecution(now time.Time) error {
	if !b.FullyAuthorized() {
		return ErrBlackStartNotAuthed
	}
	if b.State != BlackStartDualAuthed && b.State != BlackStartPaused {
		return fmt.Errorf("%w: cannot execute from %s", ErrInvalidState, b.State)
	}
	b.State = BlackStartExecuting
	if b.StartedAt.IsZero() {
		b.StartedAt = now
	}
	return nil
}

// MarkCabinStarted records that a participating cabin has been restored.
func (b *BlackStart) MarkCabinStarted(cabinID string) {
	for _, c := range b.StartedCabinIDs {
		if c == cabinID {
			return
		}
	}
	b.StartedCabinIDs = append(b.StartedCabinIDs, cabinID)
}

// MarkCabinExcluded records that a cabin was skipped because it was faulted or
// isolated, so it does not block completion.
func (b *BlackStart) MarkCabinExcluded(cabinID string) {
	for _, c := range b.ExcludedCabinIDs {
		if c == cabinID {
			return
		}
	}
	b.ExcludedCabinIDs = append(b.ExcludedCabinIDs, cabinID)
}

// CabinStarted reports whether the cabin was successfully restored.
func (b *BlackStart) CabinStarted(cabinID string) bool {
	for _, c := range b.StartedCabinIDs {
		if c == cabinID {
			return true
		}
	}
	return false
}

// CabinExcluded reports whether the cabin was excluded.
func (b *BlackStart) CabinExcluded(cabinID string) bool {
	for _, c := range b.ExcludedCabinIDs {
		if c == cabinID {
			return true
		}
	}
	return false
}

// Participates reports whether cabinID is part of this black start.
func (b *BlackStart) Participates(cabinID string) bool {
	for _, c := range b.CabinIDs {
		if c == cabinID {
			return true
		}
	}
	return false
}

// RemainingCabinIDs returns participating cabins not yet started or excluded.
func (b *BlackStart) RemainingCabinIDs() []string {
	done := make(map[string]bool, len(b.StartedCabinIDs)+len(b.ExcludedCabinIDs))
	for _, c := range b.StartedCabinIDs {
		done[c] = true
	}
	for _, c := range b.ExcludedCabinIDs {
		done[c] = true
	}
	var rem []string
	for _, c := range b.CabinIDs {
		if !done[c] {
			rem = append(rem, c)
		}
	}
	return rem
}

// Pause halts execution because a participating cabin reported an anomaly.
func (b *BlackStart) Pause(cabinID string, now time.Time) error {
	if b.State != BlackStartExecuting && b.State != BlackStartPaused {
		return fmt.Errorf("%w: cannot pause from %s", ErrInvalidState, b.State)
	}
	b.State = BlackStartPaused
	b.PausedForCabin = cabinID
	return nil
}

// Resume returns a paused black start to executing.
func (b *BlackStart) Resume() error {
	if b.State != BlackStartPaused {
		return fmt.Errorf("%w: cannot resume from %s", ErrInvalidState, b.State)
	}
	b.State = BlackStartExecuting
	b.PausedForCabin = ""
	return nil
}

// Complete finalizes a successful black start.
func (b *BlackStart) Complete(now time.Time) error {
	if b.State != BlackStartExecuting && b.State != BlackStartPaused {
		return fmt.Errorf("%w: cannot complete from %s", ErrInvalidState, b.State)
	}
	if len(b.RemainingCabinIDs()) > 0 {
		return fmt.Errorf("%w: cabins still pending", ErrInvalidState)
	}
	b.State = BlackStartCompleted
	b.CompletedAt = now
	b.PausedForCabin = ""
	return nil
}

// Fail marks the operation as failed with a reason.
func (b *BlackStart) Fail(reason string, now time.Time) {
	b.State = BlackStartFailed
	b.FailReason = reason
}

// Rollback aborts and reverts the operation.
func (b *BlackStart) Rollback(reason string, now time.Time) {
	b.State = BlackStartRolledBack
	b.FailReason = reason
}

// IsTerminal reports whether the operation reached a terminal state.
func (b *BlackStart) IsTerminal() bool {
	return b.State == BlackStartCompleted || b.State == BlackStartFailed || b.State == BlackStartRolledBack
}
