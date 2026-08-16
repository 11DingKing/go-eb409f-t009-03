package service

import (
	"fmt"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

// OpenWorkOrder creates a maintenance work order, optionally linked to an
// anomaly. If linked to a Level-1 anomaly the cabin moves to maintenance.
func (d *Dispatch) OpenWorkOrder(cabinID, anomalyID, title, desc, assignee string, insp domain.Inspector) (*domain.WorkOrder, error) {
	if _, err := d.Store.Cabin(cabinID); err != nil {
		return nil, fmt.Errorf("open work order: %w", err)
	}
	now := d.Clock.Now()
	wo := domain.NewWorkOrder(d.IDs.NewID("wo"), cabinID, anomalyID, title, desc, assignee, insp, now)
	if anomalyID != "" {
		if a, err := d.Store.Anomaly(anomalyID); err == nil && a.CabinID == cabinID && a.ShouldFreeze() {
			if cabin, err := d.Store.Cabin(cabinID); err == nil {
				_ = cabin.BeginMaintenance(now)
				_ = d.Store.SaveCabin(cabin)
			}
		}
	}
	return wo, d.Store.SaveWorkOrder(wo)
}

// StartWorkOrder moves an order to in-progress.
func (d *Dispatch) StartWorkOrder(id string) (*domain.WorkOrder, error) {
	wo, err := d.Store.WorkOrder(id)
	if err != nil {
		return nil, err
	}
	if err := wo.Start(d.Clock.Now()); err != nil {
		return nil, err
	}
	return wo, d.Store.SaveWorkOrder(wo)
}

// SubmitWorkOrderForReview requests the inspector's sign-off.
func (d *Dispatch) SubmitWorkOrderForReview(id string) (*domain.WorkOrder, error) {
	wo, err := d.Store.WorkOrder(id)
	if err != nil {
		return nil, err
	}
	if err := wo.SubmitForReview(d.Clock.Now()); err != nil {
		return nil, err
	}
	return wo, d.Store.SaveWorkOrder(wo)
}

// ReviewWorkOrder records the assigned inspector's signature. A work order can
// only be closed after this step (closed-loop control).
func (d *Dispatch) ReviewWorkOrder(id, inspectorID, note string) (*domain.WorkOrder, error) {
	wo, err := d.Store.WorkOrder(id)
	if err != nil {
		return nil, err
	}
	if err := wo.Review(inspectorID, note, d.Clock.Now()); err != nil {
		return nil, err
	}
	return wo, d.Store.SaveWorkOrder(wo)
}

// CloseWorkOrder finalizes an order. It is idempotent for already-closed orders
// and rejected unless the inspector has signed the review. On a successful close
// that was linked to a Level-1 anomaly, the anomaly is resolved and the cabin
// restored.
func (d *Dispatch) CloseWorkOrder(id string) (*domain.WorkOrder, error) {
	wo, err := d.Store.WorkOrder(id)
	if err != nil {
		return nil, err
	}
	prev := wo.State
	if err := wo.Close(d.Clock.Now()); err != nil {
		return nil, err
	}
	if err := d.Store.SaveWorkOrder(wo); err != nil {
		return nil, err
	}
	if prev != domain.WorkOrderClosed {
		if wo.AnomalyID != "" {
			if _, err := d.ResolveAnomaly(wo.AnomalyID); err == nil {
				// cabin restored by ResolveAnomaly
			}
		}
	}
	return wo, nil
}

// WorkOrder returns an order by ID.
func (d *Dispatch) WorkOrder(id string) (*domain.WorkOrder, error) { return d.Store.WorkOrder(id) }

// WorkOrders returns all work orders.
func (d *Dispatch) WorkOrders() []*domain.WorkOrder { return d.Store.WorkOrders() }
