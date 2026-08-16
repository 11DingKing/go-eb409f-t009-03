// Package service implements the application orchestration for the microgrid
// dispatch center: inspection dispatch, tiered anomaly handling, work-order
// closure, black-start execution and off-grid/on-grid switching. It enforces
// the concurrency boundaries, idempotency and failure-recovery rules that the
// domain entities alone cannot express.
package service

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/clock"
	"github.com/ejinagrid/ejinagrid/internal/domain"
	"github.com/ejinagrid/ejinagrid/internal/store"
)

// IDGenerator produces monotonically increasing, collision-resistant IDs.
type IDGenerator interface {
	NewID(prefix string) string
}

// DefaultIDs is a process-unique counter-based ID generator.
type DefaultIDs struct{ n atomic.Uint64 }

// NewID returns "<prefix>-<counter>".
func (g *DefaultIDs) NewID(prefix string) string {
	return prefix + "-" + itoa(g.n.Add(1))
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Dispatch is the application facade. It composes the store, clock and ID
// generator and exposes the five dispatch flows. The embedded coordination
// state (bsMu/bsCond) serializes black-start execution against anomaly reports.
type Dispatch struct {
	Store     *store.Store
	Clock     clock.Clock
	IDs       IDGenerator
	Window    domain.InspectionWindow
	StepDelay time.Duration

	// Black-start execution coordination state. Only one black start may run
	// at a time; the condition variable lets an anomaly report pause it.
	bsMu         sync.Mutex
	bsCond       *sync.Cond
	activeBS     *domain.BlackStart
	bsPaused     bool
	bsPauseCabin string
}

// New wires a Dispatch over the given dependencies.
func New(s *store.Store, clk clock.Clock, ids IDGenerator, window domain.InspectionWindow) *Dispatch {
	d := &Dispatch{
		Store:  s,
		Clock:  clk,
		IDs:    ids,
		Window: window,
	}
	d.bsCond = sync.NewCond(&d.bsMu)
	return d
}
