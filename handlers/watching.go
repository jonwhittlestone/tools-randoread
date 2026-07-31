package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strings"

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

	// AuthToken is embedded as a query param in the video/poster URLs — see
	// recordHTML. Set externally after construction, mirroring
	// ClippedHandler/RandoHandler's AuthToken field.
	AuthToken string
}

// NewWatchingHandler builds a WatchingHandler.
func NewWatchingHandler(client WatchitlaterClient) *WatchingHandler {
	return &WatchingHandler{Client: client}
}

// ServeHTTP serves GET /api/watching — mirrors ClippedHandler/RandoHandler's
// {title, html, path} response shape so the existing frontend makeFeature
// helper works unchanged.
//
// "Nothing currently staged" is ambiguous on its own — tools-watchitlater
// clears the previous local files before it starts fetching a replacement,
// so Current() reports staged:false for the whole duration of an in-flight
// next-video job too, not just on first-ever use. Reloading the view during
// that window must not call StartNext again (tools-watchitlater would 409 a
// concurrent start, which would otherwise surface here as a spurious 502),
// so NextStatus is checked first to tell "already fetching" and "nothing
// left to categorize" apart from "truly never started."
func (h *WatchingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	record, err := h.Client.Current()
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to reach watchitlater")
		return
	}

	var body string
	switch {
	case record.Staged:
		body = recordHTML(record, h.AuthToken)
	default:
		status, err := h.Client.NextStatus()
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "failed to check watchitlater status")
			return
		}
		switch {
		case status.NoneLeft:
			body = caughtUpHTML()
		case status.Running:
			body = fetchingHTML()
		default:
			if err := h.Client.StartNext(); err != nil {
				writeJSONError(w, http.StatusBadGateway, "failed to start fetching a video")
				return
			}
			body = fetchingHTML()
		}
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

// fetchingHTML covers both first-ever use and reloading the view while a
// next-video job is already in flight — either way there's nothing staged
// to show yet, so the frontend just polls next/status until it's done.
func fetchingHTML() string {
	return `<div class="watching">` +
		`<p>Fetching a video…</p>` +
		`<div class="watching-progress"><div class="watching-progress-bar"></div><span class="watching-progress-label"></span></div>` +
		`</div>`
}

// caughtUpHTML is shown once NextUncategorized has nothing left to offer.
func caughtUpHTML() string {
	return `<div class="watching"><p>You’re all caught up 🎉</p></div>`
}

// recordHTML renders the staged video: an embedded player (poster'd with
// its thumbnail), the metadata the prompt asked for (title, YouTube link,
// downloaded/uploaded-at, playlist rank — there's no true "date added to
// Watch Later" in the data, only ordering, so it isn't shown), an
// emoji-tag trigger, a progress-bar container (populated by app.js while a
// next-video fetch runs), and the "Get Next Video →" button — disabled
// until the video has been tagged.
func recordHTML(r *watchitlater.Record, authToken string) string {
	// Uncategorized takes precedence over the daily-limit label: the
	// instant a Next call succeeds, DailyLimitReached flips true, but the
	// freshly staged video is always uncategorized — the clock label is
	// specifically for "tagged, but out of quota today," not that.
	nextAttrs := ""
	nextLabel := "Get Next Video →"
	switch {
	case r.Emoji == "":
		nextAttrs = " disabled"
	case r.DailyLimitReached:
		nextAttrs = " disabled"
		nextLabel = "Get Next Video ⏰"
	}
	emojiLabel := "+ Tag as watched"
	if r.Emoji != "" {
		emojiLabel = html.EscapeString(r.Emoji)
	}

	// <video src>/poster can't send the X-Auth-Token header, so the token
	// travels as a query param instead — same pattern vaultFileResolver
	// already uses for /api/asset URLs (RequireToken accepts either).
	videoURL := withToken(r.VideoURL, authToken)
	thumbnailURL := withToken(r.ThumbnailURL, authToken)

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
			`<button type="button" class="watching-emoji-btn" data-video-id="%s" data-emoji="%s">%s</button>`+
			`<div class="watching-progress hidden"><div class="watching-progress-bar"></div><span class="watching-progress-label"></span></div>`+
			`<button type="button" class="watching-next-btn"%s data-video-id="%s">%s</button>`+
			`</div>`,
		html.EscapeString(thumbnailURL), html.EscapeString(videoURL), html.EscapeString(videoURL), html.EscapeString(r.Title),
		html.EscapeString(r.Title),
		html.EscapeString(r.YoutubeURL), html.EscapeString(r.YoutubeURL),
		html.EscapeString(r.DownloadedAt), html.EscapeString(r.UploadedAt), r.PlaylistRank,
		html.EscapeString(r.VideoID), html.EscapeString(r.Emoji), emojiLabel,
		nextAttrs, html.EscapeString(r.VideoID), nextLabel,
	)
}

// withToken appends ?token=authToken to rawURL (which may already have its
// own query string, though the watching URLs currently don't).
func withToken(rawURL, authToken string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "token=" + url.QueryEscape(authToken)
}
