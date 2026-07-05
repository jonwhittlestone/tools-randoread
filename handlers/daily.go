package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
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
	VaultRoot  string
	Now        func() time.Time

	// AuthToken is embedded in resolved image URLs — <img> tags can't send
	// the X-Auth-Token header, so this rides along as the query-param
	// fallback RequireToken accepts instead. See assetImageResolver.
	AuthToken string
}

// NewDailyHandler builds a DailyHandler. now defaults to time.Now if nil.
func NewDailyHandler(downloader NoteDownloader, vaultRoot string, now func() time.Time) *DailyHandler {
	if now == nil {
		now = time.Now
	}
	return &DailyHandler{Downloader: downloader, VaultRoot: vaultRoot, Now: now}
}

// assetImageResolver builds the /api/asset proxy URL for an Obsidian image
// embed, assuming it lives in the vault's assets/ folder (Obsidian's default
// attachment location — see the vault's CLAUDE.md). It doesn't verify the
// file exists; a missing asset just renders as a broken image, same as any
// other dead <img> link.
//
// authToken is embedded as a query param because the browser requests this
// URL directly via <img src>, which can't carry the X-Auth-Token header —
// RequireToken accepts either (see handlers/auth.go).
func assetImageResolver(vaultRoot, authToken string) markdown.ImageResolver {
	return func(filename string) (string, bool) {
		path := vaultRoot + "/assets/" + filename
		q := url.Values{"path": {path}, "token": {authToken}}
		return "api/asset?" + q.Encode(), true
	}
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

	html := markdown.Render(raw, assetImageResolver(h.VaultRoot, h.AuthToken))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": filename,
		"html":  html,
		"path":  path,
	})
}
