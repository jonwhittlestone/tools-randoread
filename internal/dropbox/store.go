// Package dropbox is a minimal client for the Dropbox HTTP API: OAuth2+PKCE
// token exchange/refresh, and files/download + files/list_folder. It's a Go
// port of tools-browsernotes' server/dropbox_proxy.py, which already has a
// working self-service connect flow against the same vault.
package dropbox

import (
	"encoding/json"
	"os"
)

// Tokens is the persisted Dropbox OAuth token pair.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Store persists Tokens to a JSON file on disk (mounted volume in
// production), so no manual token files ever need to be placed by hand —
// the OAuth callback writes here, and every restart reads from here.
type Store struct {
	Path string
}

// NewStore builds a Store backed by the given file path.
func NewStore(path string) *Store {
	return &Store{Path: path}
}

// Load returns the persisted tokens, or a zero-value Tokens if none have
// been saved yet (not an error — it just means Dropbox isn't connected).
func (s *Store) Load() (Tokens, error) {
	data, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return Tokens{}, nil
	}
	if err != nil {
		return Tokens{}, err
	}

	var tokens Tokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return Tokens{}, err
	}
	return tokens, nil
}

// Save writes tokens to disk, overwriting any previous value.
func (s *Store) Save(tokens Tokens) error {
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Path, data, 0o600)
}

// Delete removes the persisted tokens file (used by "disconnect").
func (s *Store) Delete() error {
	err := os.Remove(s.Path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
