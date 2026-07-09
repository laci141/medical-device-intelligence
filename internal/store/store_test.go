package store

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAndCount(t *testing.T) {
	s := openTemp(t)
	if err := s.UpsertRegulatoryAction("FDA", "Z-1-2024", "US", "dev1", "recall", "Ongoing", "20240101", "", map[string]any{"x": 1}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Upsert with the same key must not duplicate.
	if err := s.UpsertRegulatoryAction("FDA", "Z-1-2024", "US", "dev1", "recall", "Terminated", "20240101", "", nil); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	n, err := s.CountRegulatoryActions("FDA")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count=%d want 1 (upsert must not duplicate)", n)
	}
}

// TestEmptyIDRejected is the guardrail-10 regression: a blank id must error, not
// silently store zero rows.
func TestEmptyIDRejected(t *testing.T) {
	s := openTemp(t)
	if err := s.UpsertRegulatoryAction("FDA", "", "US", "d", "recall", "", "", "", nil); err != ErrEmptyID {
		t.Fatalf("empty source_id: got %v want ErrEmptyID", err)
	}
	if err := s.UpsertRegulatoryAction("", "Z-1", "US", "d", "recall", "", "", "", nil); err != ErrEmptyID {
		t.Fatalf("empty agency: got %v want ErrEmptyID", err)
	}
	n, _ := s.CountRegulatoryActions("")
	if n != 0 {
		t.Fatalf("no rows should have been stored, got %d", n)
	}
}
