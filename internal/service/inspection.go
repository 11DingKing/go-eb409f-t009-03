package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

// DispatchInspection assigns a certified inspector to inspect a cabin within
// the fixed daily window. The cabin is marked under-inspection.
func (d *Dispatch) DispatchInspection(cabinID string, insp domain.Inspector) (*domain.Inspection, error) {
	cabin, err := d.Store.Cabin(cabinID)
	if err != nil {
		return nil, fmt.Errorf("dispatch inspection: %w", err)
	}
	now := d.Clock.Now()
	if err := cabin.BeginInspection(now); err != nil {
		return nil, err
	}
	in, err := domain.NewInspection(d.IDs.NewID("insp"), cabinID, insp, d.Window, now)
	if err != nil {
		// Roll back the cabin state if inspection creation failed.
		cabin.Restore(now)
		return nil, err
	}
	if err := d.Store.SaveCabin(cabin); err != nil {
		return nil, err
	}
	if err := d.Store.SaveInspection(in); err != nil {
		return nil, err
	}
	return in, nil
}

// StartInspection moves an inspection to in-progress.
func (d *Dispatch) StartInspection(id string) (*domain.Inspection, error) {
	in, err := d.Store.Inspection(id)
	if err != nil {
		return nil, err
	}
	if err := in.Start(d.Clock.Now()); err != nil {
		return nil, err
	}
	return in, d.Store.SaveInspection(in)
}

// UploadPhoto attaches a traceability photo reference to an inspection.
func (d *Dispatch) UploadPhoto(id, photoRef string) (*domain.Inspection, error) {
	in, err := d.Store.Inspection(id)
	if err != nil {
		return nil, err
	}
	if err := in.UploadPhoto(photoRef); err != nil {
		return nil, err
	}
	return in, d.Store.SaveInspection(in)
}

// ReviewInspection records the assigned inspector's checklist signature.
func (d *Dispatch) ReviewInspection(id, inspectorID, note string) (*domain.Inspection, error) {
	in, err := d.Store.Inspection(id)
	if err != nil {
		return nil, err
	}
	if err := in.Review(inspectorID, note, d.Clock.Now()); err != nil {
		return nil, err
	}
	return in, d.Store.SaveInspection(in)
}

// CompleteInspection closes a reviewed inspection and restores the cabin. The
// cabin is only restored when there is no active anomaly on it.
func (d *Dispatch) CompleteInspection(id string) (*domain.Inspection, error) {
	in, err := d.Store.Inspection(id)
	if err != nil {
		return nil, err
	}
	if err := in.Complete(d.Clock.Now()); err != nil {
		return nil, err
	}
	if open := d.Store.OpenAnomaliesForCabin(in.CabinID); len(open) == 0 {
		if cabin, err := d.Store.Cabin(in.CabinID); err == nil {
			cabin.FinishInspection(d.Clock.Now())
			_ = d.Store.SaveCabin(cabin)
		}
	}
	return in, d.Store.SaveInspection(in)
}

// FreezeInspectionsForCabin freezes every open inspection on a cabin when a
// Level-1 anomaly is reported (called by the anomaly flow).
func (d *Dispatch) FreezeInspectionsForCabin(cabinID string) {
	for _, in := range d.Store.OpenInspectionsForCabin(cabinID) {
		_ = in.Freeze(d.Clock.Now())
		_ = d.Store.SaveInspection(in)
	}
}

// Inspection returns an inspection by ID.
func (d *Dispatch) Inspection(id string) (*domain.Inspection, error) {
	return d.Store.Inspection(id)
}

// Inspections returns all inspections.
func (d *Dispatch) Inspections() []*domain.Inspection { return d.Store.Inspections() }

// SeedCabins populates the store with the given cabins (used for bootstrap).
func (d *Dispatch) SeedCabins(ctx context.Context, cabins ...*domain.Cabin) error {
	_ = ctx
	for _, c := range cabins {
		if err := d.Store.SaveCabin(c); err != nil {
			return err
		}
	}
	return nil
}

// Cabin returns a cabin by ID.
func (d *Dispatch) Cabin(id string) (*domain.Cabin, error) { return d.Store.Cabin(id) }

// Cabins returns all cabins.
func (d *Dispatch) Cabins() []*domain.Cabin { return d.Store.Cabins() }

// errIs reports whether err wraps target.
func errIs(err, target error) bool { return errors.Is(err, target) }
