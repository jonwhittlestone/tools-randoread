package handlers

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// AssetHandler proxies a Dropbox file (typically an image referenced by an
// Obsidian embed) to the browser, so the client never needs Dropbox
// credentials of its own.
type AssetHandler struct {
	Downloader NoteDownloader
	VaultRoot  string
}

// NewAssetHandler builds an AssetHandler restricted to serving files under
// vaultRoot (never an arbitrary Dropbox path).
func NewAssetHandler(downloader NoteDownloader, vaultRoot string) *AssetHandler {
	return &AssetHandler{Downloader: downloader, VaultRoot: vaultRoot}
}

func (h *AssetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" || !strings.HasPrefix(path, h.VaultRoot+"/") {
		http.Error(w, "invalid asset path", http.StatusBadRequest)
		return
	}

	data, err := h.Downloader.Download(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Write(data) //nolint:errcheck
}
