package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCooldownStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rando.json")
	store := NewCooldownStore(path)

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if !got.LastFetchedAt.IsZero() {
		t.Fatalf("expected zero-value cooldown for missing file, got %+v", got)
	}

	want := Cooldown{Path: "/vault/note.md", LastFetchedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if !got.LastFetchedAt.Equal(want.LastFetchedAt) || got.Path != want.Path {
		t.Fatalf("Load after save = %+v, want %+v", got, want)
	}
}
