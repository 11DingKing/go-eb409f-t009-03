package domain

import "time"

// CabinStatus is the operational status of a grid-forming energy-storage
// battery cabin within the microgrid.
type CabinStatus string

const (
	// CabinStatusOperational means the cabin is healthy and available.
	CabinStatusOperational CabinStatus = "operational"
	// CabinStatusUnderInspection means a certified inspector is on the cabin.
	CabinStatusUnderInspection CabinStatus = "under_inspection"
	// CabinStatusFaultReported means an anomaly has been reported but not yet
	// escalated to a Level-1 freeze.
	CabinStatusFaultReported CabinStatus = "fault_reported"
	// CabinStatusFrozen means a Level-1 anomaly froze inspection and maintenance.
	CabinStatusFrozen CabinStatus = "frozen"
	// CabinStatusMaintenance means an open repair work order is in progress.
	CabinStatusMaintenance CabinStatus = "maintenance"
	// CabinStatusIsolated means a faulted cabin was electrically isolated.
	CabinStatusIsolated CabinStatus = "isolated"
	// CabinStatusOffline means the cabin is administratively taken offline.
	CabinStatusOffline CabinStatus = "offline"
)

// Cabin models a single battery storage cabin and its aggregates (PV strings,
// wind turbine feeder) that participate in the source-grid-load-storage system.
type Cabin struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	SiteID      string      `json:"site_id"`
	CapacityKWh int         `json:"capacity_kwh"`
	Status      CabinStatus `json:"status"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// NewCabin constructs an operational cabin.
func NewCabin(id, name, siteID string, capacityKWh int, now time.Time) *Cabin {
	return &Cabin{
		ID:          id,
		Name:        name,
		SiteID:      siteID,
		CapacityKWh: capacityKWh,
		Status:      CabinStatusOperational,
		UpdatedAt:   now,
	}
}

// BeginInspection transitions the cabin into the under-inspection state. It is
// rejected when the cabin is frozen or isolated.
func (c *Cabin) BeginInspection(now time.Time) error {
	if c.IsFrozen() {
		return ErrCabinFrozen
	}
	if c.Status == CabinStatusUnderInspection || c.Status == CabinStatusMaintenance {
		return ErrCabinBusy
	}
	c.Status = CabinStatusUnderInspection
	c.UpdatedAt = now
	return nil
}

// FinishInspection returns the cabin to operational after a completed inspection.
func (c *Cabin) FinishInspection(now time.Time) {
	c.Status = CabinStatusOperational
	c.UpdatedAt = now
}

// Freeze locks the cabin against inspection and maintenance after a Level-1
// anomaly. Already-frozen cabins remain frozen (idempotent).
func (c *Cabin) Freeze(now time.Time) {
	c.Status = CabinStatusFrozen
	c.UpdatedAt = now
}

// ReportFault marks a non-freezing anomaly so dispatchers can see the cabin is
// flagged before a work order is opened.
func (c *Cabin) ReportFault(now time.Time) error {
	if c.IsFrozen() {
		return ErrCabinFrozen
	}
	c.Status = CabinStatusFaultReported
	c.UpdatedAt = now
	return nil
}

// BeginMaintenance transitions a frozen/isolated cabin to maintenance once a
// work order has been opened.
func (c *Cabin) BeginMaintenance(now time.Time) error {
	if c.Status != CabinStatusFrozen && c.Status != CabinStatusIsolated && c.Status != CabinStatusFaultReported {
		return ErrInvalidState
	}
	c.Status = CabinStatusMaintenance
	c.UpdatedAt = now
	return nil
}

// Isolate electrically isolates a faulted cabin so the rest of the microgrid
// (and any black-start sequence) can proceed safely.
func (c *Cabin) Isolate(now time.Time) {
	c.Status = CabinStatusIsolated
	c.UpdatedAt = now
}

// Restore brings a repaired cabin back to operational.
func (c *Cabin) Restore(now time.Time) {
	c.Status = CabinStatusOperational
	c.UpdatedAt = now
}

// IsFrozen reports whether the cabin is locked against inspection/maintenance.
func (c *Cabin) IsFrozen() bool {
	return c.Status == CabinStatusFrozen || c.Status == CabinStatusIsolated
}

// Available reports whether the cabin can participate in a black start.
func (c *Cabin) Available() bool {
	return c.Status == CabinStatusOperational || c.Status == CabinStatusFaultReported
}
