package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/jonwhittlestone/tools-randoread/internal/epub"
	"github.com/jonwhittlestone/tools-randoread/internal/remarkable"
)

// remarkableClipTitlePrefix differentiates clippings sent from randoread in
// the tablet's My Files list, per Jon's 05.01.01 spec.
const remarkableClipTitlePrefix = "✂️ RANDOREAD CLIP: "

// RemarkableSender delivers an already-built EPUB file (at epubPath) to the
// tablet under title — bound to internal/remarkable.SendEpub with
// connection config baked in, mirroring EmailHandler's SendFunc pattern so
// tests don't need a real tablet.
type RemarkableSender func(epubPath, title string) (*remarkable.UploadResult, error)

// ClippingsSendHandler converts a clipping's markdown to EPUB (embedding
// any referenced images so it's readable fully offline — see
// internal/epub) and delivers it to the reMarkable tablet.
type ClippingsSendHandler struct {
	Downloader NoteDownloader
	Lister     NoteLister
	VaultRoot  string
	Send       RemarkableSender
}

// NewClippingsSendHandler builds a ClippingsSendHandler.
func NewClippingsSendHandler(downloader NoteDownloader, lister NoteLister, vaultRoot string, send RemarkableSender) *ClippingsSendHandler {
	return &ClippingsSendHandler{Downloader: downloader, Lister: lister, VaultRoot: vaultRoot, Send: send}
}

type clippingsSendRequest struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

func (h *ClippingsSendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req clippingsSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" || req.Title == "" {
		writeJSONError(w, http.StatusBadRequest, "missing clipping path or title")
		return
	}

	raw, err := h.Downloader.Download(req.Path)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch the clipping")
		return
	}

	epubBytes, err := epub.Build(req.Title, raw, epub.SizeL, h.fetchImage())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to convert the clipping to epub")
		return
	}

	epubPath, cleanup, err := writeTempEpub(epubBytes)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to prepare the epub for delivery")
		return
	}
	defer cleanup()

	if _, err := h.Send(epubPath, remarkableClipTitlePrefix+req.Title); err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("failed to deliver epub to reMarkable: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"}) //nolint:errcheck
}

// fetchImage adapts the vault's path resolver + downloader into
// epub.ImageFetcher, so referenced images end up embedded in the epub
// itself rather than left as randoread's auth-gated /api/asset URLs (which
// the tablet, reading fully offline, has no way to fetch).
func (h *ClippingsSendHandler) fetchImage() epub.ImageFetcher {
	resolvePath := vaultPathResolver(h.Lister, h.VaultRoot)

	return func(ref string) ([]byte, string, bool) {
		path, ok := resolvePath(ref)
		if !ok {
			return nil, "", false
		}
		data, err := h.Downloader.Download(path)
		if err != nil {
			return nil, "", false
		}
		return data, contentTypeFor(path), true
	}
}

// writeTempEpub writes data to a temp file and returns its path plus a
// cleanup func to remove it — internal/remarkable.SendEpub takes a
// filesystem path (it also backs the CLI, which naturally has one already),
// so the web handler needs to materialize the in-memory epub bytes first.
func writeTempEpub(data []byte) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "randoread-clip-*.epub")
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name()) //nolint:errcheck
		return "", nil, err
	}

	return f.Name(), func() { os.Remove(f.Name()) }, nil //nolint:errcheck
}
