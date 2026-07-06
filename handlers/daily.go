package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// NoteDownloader is the subset of *dropbox.Client that note handlers need —
// declared here so tests can fake it without a real Dropbox connection.
type NoteDownloader interface {
	Download(path string) ([]byte, error)
}

// DailyHandler serves the vault's daily note.
type DailyHandler struct {
	Downloader NoteDownloader
	Lister     NoteLister // resolves embeds vault-wide — see vaultFileResolver
	VaultRoot  string
	Now        func() time.Time

	// AuthToken is embedded in resolved image URLs — see vaultFileResolver.
	AuthToken string
}

// NewDailyHandler builds a DailyHandler. now defaults to time.Now if nil.
func NewDailyHandler(downloader NoteDownloader, lister NoteLister, vaultRoot string, now func() time.Time) *DailyHandler {
	if now == nil {
		now = time.Now
	}
	return &DailyHandler{Downloader: downloader, Lister: lister, VaultRoot: vaultRoot, Now: now}
}

func (h *DailyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filename := note.DailyFilename(h.Now())
	path := h.VaultRoot + "/periodic/daily/" + filename

	raw, err := h.Downloader.Download(path)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to fetch today's daily note"}) //nolint:errcheck
		return
	}

	html := markdown.Render(raw, vaultFileResolver(h.Lister, h.VaultRoot, h.AuthToken))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": filename,
		"html":  html,
		"path":  path,
	})
}
