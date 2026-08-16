package store

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ejinagrid/ejinagrid/internal/domain"
)

func TestStore_CabinSaveGetAndSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Now()
	c := domain.NewCabin("c1", "Cabin 1", "site-1", 1000, now)
	if err := s.SaveCabin(c); err != nil {
		t.Fatalf("SaveCabin: %v", err)
	}
	got, err := s.Cabin("c1")
	if err != nil {
		t.Fatalf("Cabin: %v", err)
	}
	if got.Name != "Cabin 1" || got.Status != domain.CabinStatusOperational {
		t.Fatalf("unexpected cabin %+v", got)
	}
	if _, err := s.Cabin("missing"); !IsNotFound(err) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Snapshot(); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if _, err := osStat(path); err != nil {
		t.Fatalf("snapshot file not created: %v", err)
	}
}

func TestStore_SnapshotReloadsState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s1, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	insp := domain.Inspector{ID: "i1", Name: "Alice", CertNo: "C-100"}
	insp.CertExpiry = time.Now().Add(365 * 24 * time.Hour)
	in, err := domain.NewInspection("insp-1", "c1", insp, domain.InspectionWindow{StartHour: 8, EndHour: 10}, time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewInspection: %v", err)
	}
	a := domain.NewAnomaly("a1", "c1", domain.Level1, "overtemp", "ops", time.Now())
	bs := domain.NewBlackStart("bs1", []string{"c1"}, "ops", time.Now())
	g := domain.NewGridSync("g1", "bs1", domain.DefaultSyncThresholds, time.Now())
	_ = s1.SaveInspection(in)
	_ = s1.SaveAnomaly(a)
	_ = s1.SaveBlackStart(bs)
	_ = s1.SaveGridSync(g)
	if err := s1.Snapshot(); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	s2, err := New(path)
	if err != nil {
		t.Fatalf("reload New: %v", err)
	}
	if got, err := s2.Inspection("insp-1"); err != nil || got.Inspector.CertNo != "C-100" {
		t.Fatalf("reloaded inspection mismatch: %v %v", got, err)
	}
	if got, err := s2.Anomaly("a1"); err != nil || got.Level != domain.Level1 {
		t.Fatalf("reloaded anomaly mismatch: %v %v", got, err)
	}
	if got, err := s2.BlackStart("bs1"); err != nil || got.State != domain.BlackStartRequested {
		t.Fatalf("reloaded blackstart mismatch: %v %v", got, err)
	}
	if got, err := s2.GridSync("g1"); err != nil || got.State != domain.GridSyncOffGrid {
		t.Fatalf("reloaded gridsync mismatch: %v %v", got, err)
	}
}

func TestStore_RelationQueries(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Now()
	_ = s.SaveCabin(domain.NewCabin("c1", "C1", "s", 1, now))
	open := domain.NewAnomaly("a-open", "c1", domain.Level1, "x", "r", now)
	resolved := domain.NewAnomaly("a-done", "c1", domain.Level2, "y", "r", now)
	_ = resolved.Resolve(now)
	_ = s.SaveAnomaly(open)
	_ = s.SaveAnomaly(resolved)
	got := s.OpenAnomaliesForCabin("c1")
	if len(got) != 1 || got[0].ID != "a-open" {
		t.Fatalf("open anomalies = %+v, want only a-open", got)
	}

	insp := domain.Inspector{ID: "i1", CertNo: "C", CertExpiry: now.Add(time.Hour)}
	in, err := domain.NewInspection("insp-1", "c1", insp, domain.InspectionWindow{StartHour: 8, EndHour: 10}, time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewInspection: %v", err)
	}
	_ = s.SaveInspection(in)
	if l := len(s.OpenInspectionsForCabin("c1")); l != 1 {
		t.Fatalf("open inspections = %d, want 1", l)
	}
	_ = in.Start(now)
	_ = in.UploadPhoto("p1")
	_ = in.Review("i1", "ok", now)
	_ = in.Complete(now)
	_ = s.SaveInspection(in)
	if l := len(s.OpenInspectionsForCabin("c1")); l != 0 {
		t.Fatalf("open inspections after close = %d, want 0", l)
	}
}

func TestStore_ConcurrentSaves(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c := domain.NewCabin("c"+itoa(uint64(n)), "C", "s", 1, time.Now())
			_ = s.SaveCabin(c)
			_, _ = s.Cabin("c" + itoa(uint64(n)))
		}(i)
	}
	wg.Wait()
	if len(s.Cabins()) != 50 {
		t.Fatalf("cabins = %d, want 50", len(s.Cabins()))
	}
}

// IsNotFound reports whether err is the store's not-found error.
func IsNotFound(err error) bool { return err == ErrNotFound }

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
