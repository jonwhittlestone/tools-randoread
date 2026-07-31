package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"

	"github.com/jonwhittlestone/tools-randoread/internal/watchitlater"
)

// WatchitlaterClient is the subset of *watchitlater.Client WatchingHandler
// needs — an interface so tests can fake it without an httptest.Server.
type WatchitlaterClient interface {
	Current() (*watchitlater.Record, error)
	StartNext() error
	NextStatus() (*watchitlater.NextStatus, error)
	SetEmoji(videoID, emoji string) error
	ProxyVideo(w http.ResponseWriter, r *http.Request) error
	ProxyThumbnail(w http.ResponseWriter, r *http.Request) error
}

// WatchingHandler serves the "Watching It Later 👀" section — see
// main-26-fetch-watch-later.md section 03.03. Everything (metadata,
// staging, emoji tagging, video bytes) is proxied to tools-watchitlater;
// this app never touches its SQLite database or mycloud credentials
// directly, mirroring how Dropbox/reMarkable/SMTP secrets are already
// brokered server-side and never handed to the browser.
type WatchingHandler struct {
	Client WatchitlaterClient
}

// NewWatchingHandler builds a WatchingHandler.
func NewWatchingHandler(client WatchitlaterClient) *WatchingHandler {
	return &WatchingHandler{Client: client}
}

// ServeHTTP serves GET /api/watching — mirrors ClippedHandler/RandoHandler's
// {title, html, path} response shape so the existing frontend makeFeature
// helper works unchanged. If nothing is staged yet (first-ever use), it
// kicks off tools-watchitlater's next-video job and renders a "fetching"
// state instead — the frontend then polls next/status the same way it does
// after a "Get Next Video" click.
func (h *WatchingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	record, err := h.Client.Current()
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to reach watchitlater")
		return
	}

	var body string
	if !record.Staged {
		if err := h.Client.StartNext(); err != nil {
			writeJSONError(w, http.StatusBadGateway, "failed to start fetching a video")
			return
		}
		body = fetchingHTML()
	} else {
		body = recordHTML(record)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"title": "Watching it Later 👀",
		"html":  body,
		"path":  "watching",
	})
}

// HandleEmoji serves POST /api/watching/emoji — proxies to
// tools-watchitlater's existing per-download emoji endpoint (feature
// 03.01), keyed by videoID since "Watching It Later" only ever tags the
// currently staged video, but the ID travels with the request rather than
// being assumed server-side.
func (h *WatchingHandler) HandleEmoji(w http.ResponseWriter, r *http.Request) {
	var body struct {
		VideoID string `json:"videoID"`
		Emoji   string `json:"emoji"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.VideoID == "" {
		writeJSONError(w, http.StatusBadRequest, "missing videoID")
		return
	}

	if err := h.Client.SetEmoji(body.VideoID, body.Emoji); err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to set emoji")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

// HandleNext serves POST /api/watching/next — proxies to
// tools-watchitlater's async next-video job (fire-and-forget; poll
// HandleNextStatus for progress).
func (h *WatchingHandler) HandleNext(w http.ResponseWriter, r *http.Request) {
	if err := h.Client.StartNext(); err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to start fetching the next video")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "started"}) //nolint:errcheck
}

// HandleNextStatus serves GET /api/watching/next/status, polled by the
// frontend while a next-video job runs.
func (h *WatchingHandler) HandleNextStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.Client.NextStatus()
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to check progress")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status) //nolint:errcheck
}

// HandleVideo serves GET /api/watching/video — streams the staged video's
// bytes straight through from tools-watchitlater.
func (h *WatchingHandler) HandleVideo(w http.ResponseWriter, r *http.Request) {
	if err := h.Client.ProxyVideo(w, r); err != nil {
		log.Printf("randoread: watching: proxy video error: %v", err)
	}
}

// HandleThumbnail serves GET /api/watching/thumbnail.
func (h *WatchingHandler) HandleThumbnail(w http.ResponseWriter, r *http.Request) {
	if err := h.Client.ProxyThumbnail(w, r); err != nil {
		log.Printf("randoread: watching: proxy thumbnail error: %v", err)
	}
}

func fetchingHTML() string {
	return `<div class="watching">` +
		`<p>Fetching your first video…</p>` +
		`<div class="watching-progress"><div class="watching-progress-bar"></div><span class="watching-progress-label"></span></div>` +
		`</div>`
}

// recordHTML renders the staged video: an embedded player (poster'd with
// its thumbnail), the metadata the prompt asked for (title, YouTube link,
// downloaded/uploaded-at, playlist rank — there's no true "date added to
// Watch Later" in the data, only ordering, so it isn't shown), an
// emoji-tag trigger, a progress-bar container (populated by app.js while a
// next-video fetch runs), and the "Get Next Video →" button — disabled
// until the video has been tagged.
func recordHTML(r *watchitlater.Record) string {
	nextAttrs := ""
	if r.Emoji == "" {
		nextAttrs = " disabled"
	}
	emojiLabel := "+ Tag as watched"
	if r.Emoji != "" {
		emojiLabel = html.EscapeString(r.Emoji)
	}

	return fmt.Sprintf(
		`<div class="watching">`+
			`<video controls preload="metadata" poster="%s"><source src="%s">🎬 <a href="%s">%s</a></video>`+
			`<dl class="watching-meta">`+
			`<dt>Title</dt><dd>%s</dd>`+
			`<dt>YouTube</dt><dd><a href="%s" target="_blank" rel="noopener">%s</a></dd>`+
			`<dt>Downloaded</dt><dd>%s</dd>`+
			`<dt>Uploaded</dt><dd>%s</dd>`+
			`<dt>Playlist rank</dt><dd>%d</dd>`+
			`</dl>`+
			`<button type="button" class="watching-emoji-btn" data-video-id="%s">%s</button>`+
			`<div class="watching-progress hidden"><div class="watching-progress-bar"></div><span class="watching-progress-label"></span></div>`+
			`<button type="button" class="watching-next-btn"%s data-video-id="%s">Get Next Video →</button>`+
			`</div>`,
		html.EscapeString(r.ThumbnailURL), html.EscapeString(r.VideoURL), html.EscapeString(r.VideoURL), html.EscapeString(r.Title),
		html.EscapeString(r.Title),
		html.EscapeString(r.YoutubeURL), html.EscapeString(r.YoutubeURL),
		html.EscapeString(r.DownloadedAt), html.EscapeString(r.UploadedAt), r.PlaylistRank,
		html.EscapeString(r.VideoID), emojiLabel,
		nextAttrs, html.EscapeString(r.VideoID),
	)
}
