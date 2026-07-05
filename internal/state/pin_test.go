package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPinStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pin.json")
	store := NewPinStore(path)

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got.Path != "" || !got.PeriodStart.IsZero() {
		t.Fatalf("expected zero-value pin for missing file, got %+v", got)
	}

	want := Pin{Path: "/vault/note.md", PeriodStart: time.Date(2026, 7, 5, 16, 0, 0, 0, time.UTC)}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if got.Path != want.Path || !got.PeriodStart.Equal(want.PeriodStart) {
		t.Fatalf("Load after save = %+v, want %+v", got, want)
	}
}
