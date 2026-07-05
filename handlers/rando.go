package handlers

import (
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// NoteLister is the subset of *dropbox.Client that Rando/Clipped need to
// enumerate vault notes.
type NoteLister interface {
	ListFolder(path string, recursive bool) ([]dropbox.Entry, error)
}

// RandoHandler serves a random note from the vault. Clickable at any time —
// no cooldown (removed per Jon: "I shouldn't have specified 'Disabled'").
type RandoHandler struct {
	Downloader NoteDownloader
	Lister     NoteLister
	VaultRoot  string
	Now        func() time.Time
	PickIndex  func(n int) int // returns an index in [0,n) — math/rand.Intn in production

	// AuthToken is embedded in resolved image URLs — see assetImageResolver.
	AuthToken string
}

// NewRandoHandler builds a RandoHandler. now defaults to time.Now and
// pickIndex to math/rand.Intn if nil.
func NewRandoHandler(downloader NoteDownloader, lister NoteLister, vaultRoot string, now func() time.Time, pickIndex func(int) int) *RandoHandler {
	if now == nil {
		now = time.Now
	}
	if pickIndex == nil {
		pickIndex = defaultPickIndex
	}
	return &RandoHandler{
		Downloader: downloader,
		Lister:     lister,
		VaultRoot:  vaultRoot,
		Now:        now,
		PickIndex:  pickIndex,
	}
}

// isConflictedCopy reports whether name looks like a Dropbox conflict file
// (see the vault's CLAUDE.md: "Conflict files ... marked with 'conflicted
// copy'"), which shouldn't ever be surfaced as the canonical note.
func isConflictedCopy(name string) bool {
	return strings.Contains(name, "conflicted copy")
}

func candidateNotes(entries []dropbox.Entry) []dropbox.Entry {
	var out []dropbox.Entry
	for _, e := range entries {
		if e.IsFolder || !strings.HasSuffix(e.Name, ".md") || isConflictedCopy(e.Name) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (h *RandoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Lister.ListFolder(h.VaultRoot, true)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to list vault notes")
		return
	}

	candidates := candidateNotes(entries)
	if len(candidates) == 0 {
		writeJSONError(w, http.StatusBadGateway, "no notes found in vault")
		return
	}

	chosen := candidates[h.PickIndex(len(candidates))]

	raw, err := h.Downloader.Download(chosen.Path)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch the chosen note")
		return
	}

	html := markdown.Render(raw, assetImageResolver(h.VaultRoot, h.AuthToken))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": note.FormatVaultTitle(chosen.Path, h.VaultRoot),
		"html":  html,
		"path":  chosen.Path,
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}

// defaultPickIndex uses math/rand's auto-seeded global source (Go 1.20+).
func defaultPickIndex(n int) int {
	return rand.Intn(n)
}
