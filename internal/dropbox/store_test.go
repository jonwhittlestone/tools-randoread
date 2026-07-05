package dropbox

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewStore(path)

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if got.AccessToken != "" || got.RefreshToken != "" {
		t.Fatalf("expected zero-value tokens for missing file, got %+v", got)
	}

	want := Tokens{AccessToken: "access-1", RefreshToken: "refresh-1"}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if got != want {
		t.Fatalf("Load after save = %+v, want %+v", got, want)
	}
}

func TestStoreDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store := NewStore(path)

	if err := store.Save(Tokens{AccessToken: "a"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if got.AccessToken != "" {
		t.Fatalf("expected empty tokens after delete, got %+v", got)
	}
}
