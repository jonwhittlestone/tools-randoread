// Package state persists small pieces of server-side state — currently just
// the Rando/Clipped 24h-cooldown tracker — to a JSON file on disk (a mounted
// volume in production), so it survives restarts/redeploys.
package state

import (
	"encoding/json"
	"os"
	"time"
)

// Cooldown records the last note fetched by a gated feature (Rando,
// Clipped) and when, so a fresh fetch can be refused within the window.
type Cooldown struct {
	Path          string    `json:"path"`
	LastFetchedAt time.Time `json:"last_fetched_at"`
}

// CooldownStore persists a Cooldown to a JSON file.
type CooldownStore struct {
	Path string
}

// NewCooldownStore builds a CooldownStore backed by the given file path.
func NewCooldownStore(path string) *CooldownStore {
	return &CooldownStore{Path: path}
}

// Load returns the persisted cooldown, or a zero-value Cooldown if none has
// been recorded yet (not an error — it just means nothing's been fetched).
func (s *CooldownStore) Load() (Cooldown, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return Cooldown{}, nil
	}
	if err != nil {
		return Cooldown{}, err
	}

	var c Cooldown
	if err := json.Unmarshal(data, &c); err != nil {
		return Cooldown{}, err
	}
	return c, nil
}

// Save writes the cooldown to disk, overwriting any previous value.
func (s *CooldownStore) Save(c Cooldown) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o600)
}
