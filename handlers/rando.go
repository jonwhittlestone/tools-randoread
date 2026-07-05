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

// NoteLister is the subset of *dropbox.Client that Rando/Clipped need to
// enumerate vault notes.
type NoteLister interface {
	ListFolder(path string, recursive bool) ([]dropbox.Entry, error)
}

// RandoHandler serves a random note from the vault. It's clickable at any
// time, but only picks a new note once per day: the same note keeps being
// served until the reset hour (4pm, see period.go), per Jon.
type RandoHandler struct {
	Downloader NoteDownloader
	Lister     NoteLister
	VaultRoot  string
	PinStore   *state.PinStore
	Now        func() time.Time
	PickIndex  func(n int) int // returns an index in [0,n) — math/rand.Intn in production

	// AuthToken is embedded in resolved image URLs — see assetImageResolver.
	AuthToken string
}

// NewRandoHandler builds a RandoHandler. now defaults to time.Now and
// pickIndex to math/rand.Intn if nil.
func NewRandoHandler(downloader NoteDownloader, lister NoteLister, vaultRoot string, pinStore *state.PinStore, now func() time.Time, pickIndex func(int) int) *RandoHandler {
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
		PinStore:   pinStore,
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

// isExcalidrawDrawing reports whether name is an Obsidian Excalidraw
// drawing. These are saved with a ".md" extension (so a plain suffix check
// wouldn't exclude them) but hold JSON, not readable markdown.
func isExcalidrawDrawing(name string) bool {
	return strings.Contains(name, ".excalidraw")
}

// isInTemplatesDir reports whether path is inside the vault's templates/
// folder — template scaffolding, not a real note to surface.
func isInTemplatesDir(path, vaultRoot string) bool {
	return strings.HasPrefix(path, vaultRoot+"/templates/")
}

func candidateNotes(entries []dropbox.Entry, vaultRoot string) []dropbox.Entry {
	var out []dropbox.Entry
	for _, e := range entries {
		if e.IsFolder ||
			!strings.HasSuffix(e.Name, ".md") ||
			isConflictedCopy(e.Name) ||
			isExcalidrawDrawing(e.Name) ||
			isInTemplatesDir(e.Path, vaultRoot) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (h *RandoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := h.Now()
	period := currentPeriodStart(now)

	if pin, err := h.PinStore.Load(); err == nil && pin.Path != "" && pin.PeriodStart.Equal(period) {
		if raw, err := h.Downloader.Download(pin.Path); err == nil {
			h.respond(w, pin.Path, raw)
			return
		}
		// Pinned note vanished (e.g. deleted) — fall through and pick a
		// fresh one instead of getting stuck erroring for the rest of the day.
	}

	entries, err := h.Lister.ListFolder(h.VaultRoot, true)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to list vault notes")
		return
	}

	candidates := candidateNotes(entries, h.VaultRoot)
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

	if err := h.PinStore.Save(state.Pin{Path: chosen.Path, PeriodStart: period}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to persist today's pick")
		return
	}

	h.respond(w, chosen.Path, raw)
}

func (h *RandoHandler) respond(w http.ResponseWriter, path string, raw []byte) {
	html := markdown.Render(raw, assetImageResolver(h.VaultRoot, h.AuthToken))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": note.FormatVaultTitle(path, h.VaultRoot),
		"html":  html,
		"path":  path,
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
