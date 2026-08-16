package domain

// Clone returns an independent deep copy of the cabin. Cabin has no slices, so a
// value copy suffices.
func (c Cabin) Clone() *Cabin { cp := c; return &cp }

// Clone returns an independent deep copy of the inspection, duplicating the
// photo evidence slice so callers cannot mutate the store's copy.
func (in Inspection) Clone() *Inspection {
	cp := in
	if in.Photos != nil {
		cp.Photos = append([]string(nil), in.Photos...)
	}
	return &cp
}

// Clone returns an independent deep copy of the anomaly.
func (a Anomaly) Clone() *Anomaly { cp := a; return &cp }

// Clone returns an independent deep copy of the work order.
func (w WorkOrder) Clone() *WorkOrder { cp := w; return &cp }

// Clone returns an independent deep copy of the black start, duplicating its
// cabin membership and progress slices.
func (b BlackStart) Clone() *BlackStart {
	cp := b
	cp.CabinIDs = append([]string(nil), b.CabinIDs...)
	cp.StartedCabinIDs = append([]string(nil), b.StartedCabinIDs...)
	cp.ExcludedCabinIDs = append([]string(nil), b.ExcludedCabinIDs...)
	return &cp
}

// Clone returns an independent deep copy of the grid sync.
func (g GridSync) Clone() *GridSync { cp := g; return &cp }
