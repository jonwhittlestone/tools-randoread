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
	"github.com/jonwhittlestone/tools-randoread/internal/state"
)

// Cooldown is how long a gated feature (Rando, Clipped) must wait between
// fetches.
const Cooldown = 24 * time.Hour

// NoteLister is the subset of *dropbox.Client that Rando/Clipped need to
// enumerate vault notes.
type NoteLister interface {
	ListFolder(path string, recursive bool) ([]dropbox.Entry, error)
}

// RandoHandler serves a random note from the vault, gated to once every 24h.
type RandoHandler struct {
	Downloader NoteDownloader
	Lister     NoteLister
	VaultRoot  string
	Store      *state.CooldownStore
	Now        func() time.Time
	PickIndex  func(n int) int // returns an index in [0,n) — math/rand.Intn in production
}

// NewRandoHandler builds a RandoHandler. now defaults to time.Now and
// pickIndex to math/rand.Intn if nil.
func NewRandoHandler(downloader NoteDownloader, lister NoteLister, vaultRoot string, store *state.CooldownStore, now func() time.Time, pickIndex func(int) int) *RandoHandler {
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
		Store:      store,
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

func (h *RandoHandler) remainingCooldown(c state.Cooldown) time.Duration {
	if c.LastFetchedAt.IsZero() {
		return 0
	}
	elapsed := h.Now().Sub(c.LastFetchedAt)
	remaining := Cooldown - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (h *RandoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cooldown, err := h.Store.Load()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read cooldown state")
		return
	}

	if remaining := h.remainingCooldown(cooldown); remaining > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"error":             "rando is on cooldown",
			"retryAfterSeconds": int(remaining.Seconds()),
		})
		return
	}

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

	if err := h.Store.Save(state.Cooldown{Path: chosen.Path, LastFetchedAt: h.Now()}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to persist cooldown state")
		return
	}

	html := markdown.Render(raw, assetImageResolver(h.VaultRoot))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": note.FormatVaultTitle(chosen.Path, h.VaultRoot),
		"html":  html,
	})
}

// HandleStatus serves GET /api/rando/status — lets the frontend show a
// disabled button with a countdown without consuming a fetch attempt.
func (h *RandoHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	cooldown, err := h.Store.Load()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read cooldown state")
		return
	}

	remaining := h.remainingCooldown(cooldown)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"onCooldown":        remaining > 0,
		"retryAfterSeconds": int(remaining.Seconds()),
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
