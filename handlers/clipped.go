package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// ClippingsSubpath is where web clippings live within the vault — see the
// vault's CLAUDE.md ("Clippings/ - Web clippings and saved articles").
const ClippingsSubpath = "/Clippings"

// ClippedHandler serves the most recently modified note in the vault's
// Clippings/ folder. Clickable at any time — no cooldown (removed per
// Jon: "I shouldn't have specified 'Disabled'").
type ClippedHandler struct {
	Downloader NoteDownloader
	Lister     NoteLister
	VaultRoot  string
	Now        func() time.Time
}

// NewClippedHandler builds a ClippedHandler. now defaults to time.Now if nil.
func NewClippedHandler(downloader NoteDownloader, lister NoteLister, vaultRoot string, now func() time.Time) *ClippedHandler {
	if now == nil {
		now = time.Now
	}
	return &ClippedHandler{Downloader: downloader, Lister: lister, VaultRoot: vaultRoot, Now: now}
}

func (h *ClippedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Lister.ListFolder(h.VaultRoot+ClippingsSubpath, true)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to list clippings")
		return
	}

	candidates := candidateNotes(entries)
	if len(candidates) == 0 {
		writeJSONError(w, http.StatusBadGateway, "no clippings found")
		return
	}

	mostRecent := candidates[0]
	for _, c := range candidates[1:] {
		if c.ModifiedAt.After(mostRecent.ModifiedAt) {
			mostRecent = c
		}
	}

	raw, err := h.Downloader.Download(mostRecent.Path)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch the chosen clipping")
		return
	}

	html := markdown.Render(raw, assetImageResolver(h.VaultRoot))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": note.FormatVaultTitle(mostRecent.Path, h.VaultRoot),
		"html":  html,
		"path":  mostRecent.Path,
	})
}
