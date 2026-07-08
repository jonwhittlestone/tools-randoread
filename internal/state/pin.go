// Package state persists small pieces of server-side state — currently
// just Rando's "note of the day" pin — to a JSON file on disk (a mounted
// volume in production), so it survives restarts/redeploys.
package state

import (
	"encoding/json"
	"os"
	"time"
)

// Pin records which note Rando is currently serving and which reset period
// that pick belongs to, so repeated clicks within the same period return
// the same note instead of picking a new one every time.
type Pin struct {
	Path        string    `json:"path"`
	PeriodStart time.Time `json:"period_start"`

	// ModifiedAt is the pinned file's Dropbox modified time, carried along
	// so "Rando Clipped" can show its "Date Clipped:" heading on the fast
	// (already-pinned) path without re-listing the vault. Zero for plain
	// Rando, which doesn't use it.
	ModifiedAt time.Time `json:"modified_at,omitempty"`
}

// PinStore persists a Pin to a JSON file.
type PinStore struct {
	Path string
}

// NewPinStore builds a PinStore backed by the given file path.
func NewPinStore(path string) *PinStore {
	return &PinStore{Path: path}
}

// Load returns the persisted pin, or a zero-value Pin if none has been
// recorded yet (not an error — it just means nothing's been picked).
func (s *PinStore) Load() (Pin, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return Pin{}, nil
	}
	if err != nil {
		return Pin{}, err
	}

	var p Pin
	if err := json.Unmarshal(data, &p); err != nil {
		return Pin{}, err
	}
	return p, nil
}

// Save writes the pin to disk, overwriting any previous value.
func (s *PinStore) Save(p Pin) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o600)
}
