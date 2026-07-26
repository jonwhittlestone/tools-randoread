package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// clippingsListWindow is "the last 3 months" per Jon's 05.01.01 spec — a
// fixed 90-day lookback rather than calendar-month arithmetic, which is
// close enough for a personal reading list and avoids edge cases like
// varying month lengths.
const clippingsListWindow = 90 * 24 * time.Hour

// ClippingsListHandler lists every clipping modified within the last 3
// months, newest first — the data table behind the Clippings breadcrumb
// link. Unlike Rando/Clipped it deliberately does not use the shared 24h
// vault-list cache: Jon wants every breadcrumb click to refetch fresh from
// Dropbox (see main.go's wiring — this handler must be given the raw
// Dropbox client, not vaultListCache).
type ClippingsListHandler struct {
	Lister    NoteLister
	VaultRoot string
	Now       func() time.Time
}

// NewClippingsListHandler builds a ClippingsListHandler. now defaults to
// time.Now if nil.
func NewClippingsListHandler(lister NoteLister, vaultRoot string, now func() time.Time) *ClippingsListHandler {
	if now == nil {
		now = time.Now
	}
	return &ClippingsListHandler{Lister: lister, VaultRoot: vaultRoot, Now: now}
}

type clippingSummary struct {
	Path      string `json:"path"`
	Title     string `json:"title"`
	ClippedAt string `json:"clippedAt"`
}

func (h *ClippingsListHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	entries, err := h.Lister.ListFolder(h.VaultRoot+ClippingsSubpath, true)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to list clippings")
		return
	}

	cutoff := h.Now().Add(-clippingsListWindow)
	candidates := candidateNotes(entries, h.VaultRoot)

	clippings := make([]clippingSummary, 0, len(candidates))
	for _, c := range candidates {
		if c.ModifiedAt.Before(cutoff) {
			continue
		}
		// Formatted relative to the Clippings/ subfolder itself (not the
		// true vault root) so Title is just the article name, matching the
		// table mockup's plain "Title" column — the breadcrumb above it
		// already supplies the "Clippings /" context.
		title := note.FormatVaultTitle(c.Path, h.VaultRoot+ClippingsSubpath)
		clippings = append(clippings, clippingSummary{
			Path:      c.Path,
			Title:     title,
			ClippedAt: c.ModifiedAt.In(randoLocation).Format(dateClippedFormat),
		})
	}

	sort.Slice(clippings, func(i, j int) bool {
		return clippings[i].ClippedAt > clippings[j].ClippedAt
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"clippings": clippings}) //nolint:errcheck
}
