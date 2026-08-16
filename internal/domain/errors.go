// Package domain defines the core business entities and their state machines for
// the Ejina Banner microgrid dispatch center. Entities are plain Go structs with
// transition methods that enforce invariants; persistence and transport are
// handled by outer layers.
package domain

import "errors"

// Common domain errors. Comparing with errors.Is keeps behaviour explicit at
// every layer boundary.
var (
	ErrCabinFrozen           = errors.New("cabin is frozen, inspection and maintenance blocked")
	ErrCabinBusy             = errors.New("cabin is already engaged in another operation")
	ErrInvalidState          = errors.New("invalid state transition")
	ErrInspectorUncertified  = errors.New("inspector is not certified for battery-cabin inspection")
	ErrOutOfInspectionWindow = errors.New("outside the fixed daily inspection window")
	ErrAnomalyEscalated      = errors.New("anomaly passed the 15-minute acknowledgement deadline")
	ErrWorkOrderNotReviewed  = errors.New("work order must be reviewed and signed before closure")
	ErrInspectionNotSigned   = errors.New("inspection must be reviewed and signed by the inspector")
	ErrBlackStartNotAuthed   = errors.New("black start requires dual authorization by duty officer and maintenance lead")
	ErrBlackStartConflict    = errors.New("black start conflicts with an active anomaly on a participating cabin")
	ErrSyncNotVerified       = errors.New("grid connection requires successful synchronizer verification")
	ErrSyncFaultActive       = errors.New("sync fault active; retest before reconnecting")
	ErrDuplicate             = errors.New("duplicate operation ignored")
)
