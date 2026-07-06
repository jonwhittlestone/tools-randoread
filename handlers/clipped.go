package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// dateClippedFormat matches Jon's requested heading format (interpreted as
// hour:minute — "hh:ss" as literally written would omit minutes entirely).
const dateClippedFormat = "2006-01-02 15:04"

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

	// AuthToken is embedded in resolved image URLs — see assetImageResolver.
	AuthToken string
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

	candidates := candidateNotes(entries, h.VaultRoot)
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

	html := markdown.Render(raw, vaultFileResolver(h.Lister, h.VaultRoot, h.AuthToken))
	heading := "<h3>Date Clipped: " + mostRecent.ModifiedAt.In(randoLocation).Format(dateClippedFormat) + "</h3>\n"
	html = heading + html

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": note.FormatVaultTitle(mostRecent.Path, h.VaultRoot),
		"html":  html,
		"path":  mostRecent.Path,
	})
}
