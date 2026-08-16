package domain

import (
	"fmt"
	"time"
)

// WorkOrderState tracks the life cycle of a maintenance work order.
type WorkOrderState string

const (
	WorkOrderOpen           WorkOrderState = "open"
	WorkOrderInProgress     WorkOrderState = "in_progress"
	WorkOrderAwaitingReview WorkOrderState = "awaiting_review"
	WorkOrderClosed         WorkOrderState = "closed"
)

// WorkOrder is a maintenance ticket that must be reviewed and signed by the
// assigned inspector before it can be closed (closed-loop control).
type WorkOrder struct {
	ID          string         `json:"id"`
	CabinID     string         `json:"cabin_id"`
	AnomalyID   string         `json:"anomaly_id,omitempty"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Assignee    string         `json:"assignee"`
	Inspector   Inspector      `json:"inspector"`
	CreatedAt   time.Time      `json:"created_at"`
	State       WorkOrderState `json:"state"`
	ReviewNote  string         `json:"review_note"`
	SignedBy    string         `json:"signed_by"`
	SignedAt    time.Time      `json:"signed_at"`
	ClosedAt    time.Time      `json:"closed_at"`
}

// NewWorkOrder creates an open work order linked to an anomaly (if any).
func NewWorkOrder(id, cabinID, anomalyID, title, desc, assignee string, insp Inspector, now time.Time) *WorkOrder {
	return &WorkOrder{
		ID:          id,
		CabinID:     cabinID,
		AnomalyID:   anomalyID,
		Title:       title,
		Description: desc,
		Assignee:    assignee,
		Inspector:   insp,
		CreatedAt:   now,
		State:       WorkOrderOpen,
	}
}

// Start moves the order to in-progress.
func (w *WorkOrder) Start(now time.Time) error {
	if w.State != WorkOrderOpen {
		return fmt.Errorf("%w: cannot start from %s", ErrInvalidState, w.State)
	}
	w.State = WorkOrderInProgress
	return nil
}

// SubmitForReview requests the inspector's sign-off.
func (w *WorkOrder) SubmitForReview(now time.Time) error {
	if w.State != WorkOrderInProgress {
		return fmt.Errorf("%w: cannot submit for review from %s", ErrInvalidState, w.State)
	}
	w.State = WorkOrderAwaitingReview
	return nil
}

// Review records the inspector's signature. Only the assigned inspector may sign,
// and only while awaiting review.
func (w *WorkOrder) Review(inspectorID, note string, now time.Time) error {
	if w.State != WorkOrderAwaitingReview {
		return fmt.Errorf("%w: review requires awaiting_review state", ErrInvalidState)
	}
	if inspectorID != w.Inspector.ID {
		return fmt.Errorf("%w: only the assigned inspector may review", ErrInvalidState)
	}
	w.ReviewNote = note
	w.SignedBy = inspectorID
	w.SignedAt = now
	return nil
}

// Close finalizes the order. It is idempotent for already-closed orders and
// rejected otherwise unless the inspector has signed the review.
func (w *WorkOrder) Close(now time.Time) error {
	if w.State == WorkOrderClosed {
		return ErrDuplicate
	}
	if w.SignedBy == "" || w.SignedAt.IsZero() {
		return ErrWorkOrderNotReviewed
	}
	if w.State != WorkOrderAwaitingReview {
		return fmt.Errorf("%w: close from %s requires awaiting_review", ErrInvalidState, w.State)
	}
	w.State = WorkOrderClosed
	w.ClosedAt = now
	return nil
}

// IsClosed reports whether the order is closed.
func (w *WorkOrder) IsClosed() bool { return w.State == WorkOrderClosed }
