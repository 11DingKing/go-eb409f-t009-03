// Package clock provides an injectable time abstraction so that time-sensitive
// domain logic (inspection windows, anomaly escalation deadlines, sync retest
// backoff) can be exercised deterministically in tests without sleeping.
package clock

import (
	"sync"
	"time"
)

// Clock returns the current instant.
type Clock interface {
	Now() time.Time
}

// System is the production clock backed by the wall clock.
type System struct{}

// Now reports the current wall-clock time.
func (System) Now() time.Time { return time.Now() }

// Fake is a controllable clock for tests. It is safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a fake clock anchored at the given instant.
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

// Now reports the fake clock's current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set sets the fake clock to an exact instant.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}
