package domain

import (
	"fmt"
	"time"
)

// InspectionState tracks the life cycle of a daily cabin inspection.
type InspectionState string

const (
	// InspectionDispatched: duty officer has assigned an inspector.
	InspectionDispatched InspectionState = "dispatched"
	// InspectionInProgress: inspector is on site.
	InspectionInProgress InspectionState = "in_progress"
	// InspectionPhotoUploaded: traceability photos have been attached.
	InspectionPhotoUploaded InspectionState = "photo_uploaded"
	// InspectionReviewed: the inspector has signed off the checklist.
	InspectionReviewed InspectionState = "reviewed"
	// InspectionCompleted: the inspection is closed and the cabin restored.
	InspectionCompleted InspectionState = "completed"
	// InspectionFrozen: the cabin was frozen mid-inspection.
	InspectionFrozen InspectionState = "frozen"
)

// InspectionWindow defines the fixed daily window during which inspections must
// be carried out (e.g. 08:00–10:00 local time).
type InspectionWindow struct {
	StartHour int
	EndHour   int
}

// Contains reports whether t falls inside the window on its calendar day.
func (w InspectionWindow) Contains(t time.Time) bool {
	h := t.Hour()
	return h >= w.StartHour && h < w.EndHour
}

// Inspector is a certified person permitted to perform cabin inspections.
type Inspector struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CertNo     string    `json:"cert_no"`
	CertExpiry time.Time `json:"cert_expiry"`
	Revoked    bool      `json:"revoked"`
}

// Certified reports whether the inspector holds a valid, non-expired cert.
func (i Inspector) Certified(now time.Time) bool {
	if i.CertNo == "" || i.Revoked {
		return false
	}
	return i.CertExpiry.IsZero() || i.CertExpiry.After(now)
}

// Inspection is a daily cabin inspection record, photo-evidenced and signed.
type Inspection struct {
	ID           string           `json:"id"`
	CabinID      string           `json:"cabin_id"`
	Inspector    Inspector        `json:"inspector"`
	Window       InspectionWindow `json:"window"`
	DispatchedAt time.Time        `json:"dispatched_at"`
	State        InspectionState  `json:"state"`
	Photos       []string         `json:"photos"`
	ReviewerNote string           `json:"reviewer_note"`
	SignedBy     string           `json:"signed_by"`
	SignedAt     time.Time        `json:"signed_at"`
	CompletedAt  time.Time        `json:"completed_at"`
}

// NewInspection validates the inspector cert and the dispatch window, then
// creates a dispatched inspection.
func NewInspection(id, cabinID string, insp Inspector, window InspectionWindow, now time.Time) (*Inspection, error) {
	if !insp.Certified(now) {
		return nil, ErrInspectorUncertified
	}
	if !window.Contains(now) {
		return nil, ErrOutOfInspectionWindow
	}
	return &Inspection{
		ID:           id,
		CabinID:      cabinID,
		Inspector:    insp,
		Window:       window,
		DispatchedAt: now,
		State:        InspectionDispatched,
		Photos:       []string{},
	}, nil
}

// Start moves a dispatched inspection to in-progress.
func (in *Inspection) Start(now time.Time) error {
	if in.State != InspectionDispatched {
		return fmt.Errorf("%w: cannot start from %s", ErrInvalidState, in.State)
	}
	in.State = InspectionInProgress
	return nil
}

// UploadPhoto attaches a piece of traceability evidence (an object key/path).
func (in *Inspection) UploadPhoto(ref string) error {
	if in.State != InspectionInProgress && in.State != InspectionPhotoUploaded {
		return fmt.Errorf("%w: cannot upload photo from %s", ErrInvalidState, in.State)
	}
	if ref == "" {
		return fmt.Errorf("%w: empty photo reference", ErrInvalidState)
	}
	in.Photos = append(in.Photos, ref)
	in.State = InspectionPhotoUploaded
	return nil
}

// Review records the inspector's signature on the checklist.
func (in *Inspection) Review(inspectorID, note string, now time.Time) error {
	if in.State != InspectionPhotoUploaded {
		return fmt.Errorf("%w: review requires uploaded photos", ErrInvalidState)
	}
	if inspectorID != in.Inspector.ID {
		return fmt.Errorf("%w: only the assigned inspector may sign", ErrInvalidState)
	}
	in.ReviewerNote = note
	in.SignedBy = inspectorID
	in.SignedAt = now
	in.State = InspectionReviewed
	return nil
}

// Complete closes the inspection. It requires a prior review signature.
func (in *Inspection) Complete(now time.Time) error {
	if in.State != InspectionReviewed {
		return ErrInspectionNotSigned
	}
	in.State = InspectionCompleted
	in.CompletedAt = now
	return nil
}

// Freeze halts the inspection when its cabin is frozen by a Level-1 anomaly.
func (in *Inspection) Freeze(now time.Time) error {
	if in.State == InspectionCompleted || in.State == InspectionFrozen {
		return nil
	}
	in.State = InspectionFrozen
	return nil
}

// IsClosed reports whether the inspection reached a terminal state.
func (in *Inspection) IsClosed() bool {
	return in.State == InspectionCompleted || in.State == InspectionFrozen
}
