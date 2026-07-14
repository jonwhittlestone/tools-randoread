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

// extraContentTypes covers formats the vault embeds whose type isn't
// reliably resolvable via mime.TypeByExtension in the production container
// (debian:bookworm-slim has no mime-support package, so there's no
// /etc/mime.types for Go to consult) — confirmed via HAR capture: an
// embedded .mp4 was served as application/octet-stream, which browsers
// won't play inline.
var extraContentTypes = map[string]string{
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".m4v":  "video/x-m4v",
}

func contentTypeFor(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := extraContentTypes[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
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

	w.Header().Set("Content-Type", contentTypeFor(path))
	w.Write(data) //nolint:errcheck
}
