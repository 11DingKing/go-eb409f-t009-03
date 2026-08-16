// Package store provides a self-contained, thread-safe persistence layer for the
// microgrid dispatch center. State is held in memory and snapshotted to a JSON
// file so that a restart recovers the last known grid state without depending on
// any external database or cloud service.
//
// Every accessor returns an independent clone of the stored entity and every
// save stores a clone, so callers never alias the store's internal state. This
// keeps concurrent reads (including JSON snapshots) race-free against in-flight
// mutations performed by services on their own working copies.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

// ErrNotFound is returned when an entity lookup misses.
var ErrNotFound = errors.New("store: not found")

// snapshotData is the JSON representation of the whole store.
type snapshotData struct {
	Cabins      map[string]*domain.Cabin      `json:"cabins"`
	Inspections map[string]*domain.Inspection `json:"inspections"`
	Anomalies   map[string]*domain.Anomaly    `json:"anomalies"`
	WorkOrders  map[string]*domain.WorkOrder  `json:"work_orders"`
	BlackStarts map[string]*domain.BlackStart `json:"black_starts"`
	GridSyncs   map[string]*domain.GridSync   `json:"grid_syncs"`
}

// Store is the single source of truth for dispatch state.
type Store struct {
	mu   sync.RWMutex
	data snapshotData
	path string
}

// New returns a store that snapshots to path. If path contains a previous
// snapshot it is loaded on construction.
func New(path string) (*Store, error) {
	s := &Store{
		path: path,
		data: snapshotData{
			Cabins:      map[string]*domain.Cabin{},
			Inspections: map[string]*domain.Inspection{},
			Anomalies:   map[string]*domain.Anomaly{},
			WorkOrders:  map[string]*domain.WorkOrder{},
			BlackStarts: map[string]*domain.BlackStart{},
			GridSyncs:   map[string]*domain.GridSync{},
		},
	}
	if err := s.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var snap snapshotData
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	if snap.Cabins == nil {
		snap.Cabins = map[string]*domain.Cabin{}
	}
	if snap.Inspections == nil {
		snap.Inspections = map[string]*domain.Inspection{}
	}
	if snap.Anomalies == nil {
		snap.Anomalies = map[string]*domain.Anomaly{}
	}
	if snap.WorkOrders == nil {
		snap.WorkOrders = map[string]*domain.WorkOrder{}
	}
	if snap.BlackStarts == nil {
		snap.BlackStarts = map[string]*domain.BlackStart{}
	}
	if snap.GridSyncs == nil {
		snap.GridSyncs = map[string]*domain.GridSync{}
	}
	s.data = snap
	return nil
}

// Snapshot writes the current state to disk atomically.
func (s *Store) Snapshot() error {
	s.mu.RLock()
	b, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ----- Cabins -----

func (s *Store) SaveCabin(c *domain.Cabin) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Cabins[c.ID] = c.Clone()
	return nil
}

func (s *Store) Cabin(id string) (*domain.Cabin, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data.Cabins[id]
	if !ok {
		return nil, ErrNotFound
	}
	return c.Clone(), nil
}

func (s *Store) Cabins() []*domain.Cabin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.Cabin, 0, len(s.data.Cabins))
	for _, c := range s.data.Cabins {
		out = append(out, c.Clone())
	}
	return out
}

// ----- Inspections -----

func (s *Store) SaveInspection(in *domain.Inspection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Inspections[in.ID] = in.Clone()
	return nil
}

func (s *Store) Inspection(id string) (*domain.Inspection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	in, ok := s.data.Inspections[id]
	if !ok {
		return nil, ErrNotFound
	}
	return in.Clone(), nil
}

func (s *Store) Inspections() []*domain.Inspection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.Inspection, 0, len(s.data.Inspections))
	for _, in := range s.data.Inspections {
		out = append(out, in.Clone())
	}
	return out
}

func (s *Store) OpenInspectionsForCabin(cabinID string) []*domain.Inspection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*domain.Inspection
	for _, in := range s.data.Inspections {
		if in.CabinID == cabinID && !in.IsClosed() {
			out = append(out, in.Clone())
		}
	}
	return out
}

// ----- Anomalies -----

func (s *Store) SaveAnomaly(a *domain.Anomaly) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Anomalies[a.ID] = a.Clone()
	return nil
}

func (s *Store) Anomaly(id string) (*domain.Anomaly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.data.Anomalies[id]
	if !ok {
		return nil, ErrNotFound
	}
	return a.Clone(), nil
}

func (s *Store) Anomalies() []*domain.Anomaly {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.Anomaly, 0, len(s.data.Anomalies))
	for _, a := range s.data.Anomalies {
		out = append(out, a.Clone())
	}
	return out
}

// OpenAnomaliesForCabin returns unresolved anomalies on a cabin.
func (s *Store) OpenAnomaliesForCabin(cabinID string) []*domain.Anomaly {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*domain.Anomaly
	for _, a := range s.data.Anomalies {
		if a.CabinID == cabinID && !a.IsTerminal() {
			out = append(out, a.Clone())
		}
	}
	return out
}

// ----- Work orders -----

func (s *Store) SaveWorkOrder(w *domain.WorkOrder) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.WorkOrders[w.ID] = w.Clone()
	return nil
}

func (s *Store) WorkOrder(id string) (*domain.WorkOrder, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.data.WorkOrders[id]
	if !ok {
		return nil, ErrNotFound
	}
	return w.Clone(), nil
}

func (s *Store) WorkOrders() []*domain.WorkOrder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.WorkOrder, 0, len(s.data.WorkOrders))
	for _, w := range s.data.WorkOrders {
		out = append(out, w.Clone())
	}
	return out
}

// ----- Black starts -----

func (s *Store) SaveBlackStart(b *domain.BlackStart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.BlackStarts[b.ID] = b.Clone()
	return nil
}

func (s *Store) BlackStart(id string) (*domain.BlackStart, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.data.BlackStarts[id]
	if !ok {
		return nil, ErrNotFound
	}
	return b.Clone(), nil
}

func (s *Store) BlackStarts() []*domain.BlackStart {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.BlackStart, 0, len(s.data.BlackStarts))
	for _, b := range s.data.BlackStarts {
		out = append(out, b.Clone())
	}
	return out
}

// ActiveBlackStart returns a non-terminal black start, if any.
func (s *Store) ActiveBlackStart() *domain.BlackStart {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, b := range s.data.BlackStarts {
		if !b.IsTerminal() {
			return b.Clone()
		}
	}
	return nil
}

// ----- Grid syncs -----

func (s *Store) SaveGridSync(g *domain.GridSync) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.GridSyncs[g.ID] = g.Clone()
	return nil
}

func (s *Store) GridSync(id string) (*domain.GridSync, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g, ok := s.data.GridSyncs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return g.Clone(), nil
}

func (s *Store) GridSyncs() []*domain.GridSync {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*domain.GridSync, 0, len(s.data.GridSyncs))
	for _, g := range s.data.GridSyncs {
		out = append(out, g.Clone())
	}
	return out
}

// ActiveGridSync returns a non-terminal grid sync, if any.
func (s *Store) ActiveGridSync() *domain.GridSync {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.data.GridSyncs {
		if !g.IsTerminal() {
			return g.Clone()
		}
	}
	return nil
}

// Now returns the current time.
func Now() time.Time { return time.Now() }
