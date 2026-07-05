package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
	"github.com/jonwhittlestone/tools-randoread/internal/state"
)

// clippingsSubpath is where web clippings live within the vault — see the
// vault's CLAUDE.md ("Clippings/ - Web clippings and saved articles").
const clippingsSubpath = "/Clippings"

// ClippedHandler serves the most recently modified note in the vault's
// Clippings/ folder, gated to once every 24h (same cooldown window as
// Rando, tracked separately).
type ClippedHandler struct {
	Downloader NoteDownloader
	Lister     NoteLister
	VaultRoot  string
	Store      *state.CooldownStore
	Now        func() time.Time
}

// NewClippedHandler builds a ClippedHandler. now defaults to time.Now if nil.
func NewClippedHandler(downloader NoteDownloader, lister NoteLister, vaultRoot string, store *state.CooldownStore, now func() time.Time) *ClippedHandler {
	if now == nil {
		now = time.Now
	}
	return &ClippedHandler{Downloader: downloader, Lister: lister, VaultRoot: vaultRoot, Store: store, Now: now}
}

func (h *ClippedHandler) remainingCooldown(c state.Cooldown) time.Duration {
	if c.LastFetchedAt.IsZero() {
		return 0
	}
	remaining := Cooldown - h.Now().Sub(c.LastFetchedAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (h *ClippedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cooldown, err := h.Store.Load()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read cooldown state")
		return
	}

	if remaining := h.remainingCooldown(cooldown); remaining > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"error":             "clipped is on cooldown",
			"retryAfterSeconds": int(remaining.Seconds()),
		})
		return
	}

	entries, err := h.Lister.ListFolder(h.VaultRoot+clippingsSubpath, true)
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

	if err := h.Store.Save(state.Cooldown{Path: mostRecent.Path, LastFetchedAt: h.Now()}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to persist cooldown state")
		return
	}

	html := markdown.Render(raw, assetImageResolver(h.VaultRoot))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": note.FormatVaultTitle(mostRecent.Path, h.VaultRoot),
		"html":  html,
	})
}

// HandleStatus serves GET /api/clipped/status.
func (h *ClippedHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
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
