// Package watchitlater is a thin HTTP client for tools-watchitlater's
// "Watching It Later" API. tools-randoread's backend calls this
// server-to-server, using its own auth token — the browser only ever
// talks to tools-randoread, mirroring how Dropbox/reMarkable/SMTP
// credentials are already brokered server-side and never handed to the
// frontend (see internal/dropbox, internal/remarkable).
package watchitlater

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client talks to a single tools-watchitlater instance.
type Client struct {
	BaseURL    string // e.g. "https://howapped.zapto.org/watchitlater/"
	AuthToken  string
	HTTPClient *http.Client // overridable for tests; defaults to http.DefaultClient
}

// NewClient builds a Client for baseURL, authenticating with authToken.
func NewClient(baseURL, authToken string) *Client {
	return &Client{BaseURL: baseURL, AuthToken: authToken}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// Record mirrors tools-watchitlater's GET /api/watching/current response.
type Record struct {
	Staged       bool   `json:"staged"`
	VideoID      string `json:"videoID"`
	Title        string `json:"title"`
	YoutubeURL   string `json:"youtubeURL"`
	DownloadedAt string `json:"downloadedAt"`
	UploadedAt   string `json:"uploadedAt"`
	PlaylistRank int    `json:"playlistRank"`
	Emoji        string `json:"emoji"`
	VideoURL     string `json:"videoURL"`
	ThumbnailURL string `json:"thumbnailURL"`
	// DailyLimitReached is true once "Get Next Video" has already been
	// used today (Europe/London, 4pm reset) — see tools-watchitlater's
	// handlers/period.go for the period boundary this mirrors.
	DailyLimitReached bool `json:"dailyLimitReached"`
	// StagedAt (RFC3339) is when this video was staged — lets a caller
	// tell "staged just now" apart from "has been sitting here since a
	// previous day," which DailyLimitReached alone can't (it flips false
	// the moment the period rolls over, whether or not this video is
	// actually stale).
	StagedAt string `json:"stagedAt"`
	// CategorizedAt (RFC3339) is when this video was tagged with an emoji —
	// empty if it hasn't been tagged. Distinct from StagedAt: re-staging an
	// old, already-tagged video from history ("Load watch later videos")
	// makes StagedAt recent while CategorizedAt stays whenever it was
	// originally tagged, which is exactly what videoIsStaleAndTagged needs
	// to tell "genuinely tagged today" apart from "an old video someone
	// just replayed from history."
	CategorizedAt string `json:"categorizedAt"`
	// NextFreshVideoAt (RFC3339) is when the daily limit next rolls over —
	// see tools-watchitlater's handlers/period.go.
	NextFreshVideoAt string `json:"nextFreshVideoAt"`
}

// NextStatus mirrors tools-watchitlater's GET /api/watching/next/status response.
type NextStatus struct {
	Running          bool    `json:"running"`
	Done             bool    `json:"done"`
	NoneLeft         bool    `json:"noneLeft"`
	BytesTransferred int64   `json:"bytesTransferred"`
	TotalBytes       int64   `json:"totalBytes"`
	Percent          float64 `json:"percent"`
	Error            string  `json:"error"`
}

// Current fetches the currently staged video's metadata (Staged is false if
// nothing has been staged yet).
func (c *Client) Current() (*Record, error) {
	var r Record
	if err := c.getJSON("api/watching/current", &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// StartNext kicks off tools-watchitlater's background next-video job.
// Returns an error on a non-200 response (e.g. 409 if one's already running).
func (c *Client) StartNext() error {
	return c.doStatusOK(http.MethodPost, "api/watching/next", nil)
}

// NextStatus polls the currently running (or just-finished) next-video job.
func (c *Client) NextStatus() (*NextStatus, error) {
	var s NextStatus
	if err := c.getJSON("api/watching/next/status", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// HistoryRecord mirrors one entry of tools-watchitlater's GET
// /api/watching/history response — a previously categorized video.
type HistoryRecord struct {
	VideoID       string `json:"videoID"`
	Title         string `json:"title"`
	YoutubeURL    string `json:"youtubeURL"`
	DownloadedAt  string `json:"downloadedAt"`
	UploadedAt    string `json:"uploadedAt"`
	Emoji         string `json:"emoji"`
	CategorizedAt string `json:"categorizedAt"`
}

// History fetches every previously categorized video, most recently
// categorized first — "Load watch later videos" (main-randoread.md
// 05.02.03).
func (c *Client) History() ([]HistoryRecord, error) {
	var resp struct {
		Videos []HistoryRecord `json:"videos"`
	}
	if err := c.getJSON("api/watching/history", &resp); err != nil {
		return nil, err
	}
	return resp.Videos, nil
}

// StageVideo re-downloads and stages videoID as the current video — like
// StartNext, but for one specific already-categorized video rather than
// walking uncategorized candidates. Poll NextStatus for progress, same as
// StartNext; returns an error on a non-200 response (e.g. 409 if a fetch is
// already running, 400 if videoID hasn't been categorized).
func (c *Client) StageVideo(videoID string) error {
	return c.doStatusOK(http.MethodPost, "api/watching/stage/"+videoID, nil)
}

// SetEmoji tags videoID with emoji (or clears it, if emoji is "") via
// tools-watchitlater's existing per-download emoji endpoint (feature 03.01
// — nothing watching-specific needed here).
func (c *Client) SetEmoji(videoID, emoji string) error {
	body, err := json.Marshal(map[string]string{"emoji": emoji})
	if err != nil {
		return fmt.Errorf("marshal emoji body: %w", err)
	}
	req, err := c.newRequest(http.MethodPost, "api/downloads/"+videoID+"/emoji", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("set emoji: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("set emoji: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ProxyVideo streams the currently staged video's bytes to w, forwarding
// r's Range header upstream and the upstream's status/response headers back
// downstream — so seeking still works end-to-end even though the browser
// never talks to tools-watchitlater directly.
func (c *Client) ProxyVideo(w http.ResponseWriter, r *http.Request) error {
	return c.proxyGet(w, r, "api/watching/video")
}

// ProxyThumbnail streams the currently staged thumbnail's bytes to w.
func (c *Client) ProxyThumbnail(w http.ResponseWriter, r *http.Request) error {
	return c.proxyGet(w, r, "api/watching/thumbnail")
}

func (c *Client) proxyGet(w http.ResponseWriter, r *http.Request, path string) error {
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("proxy %s: %w", path, err)
	}
	defer resp.Body.Close()

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

func (c *Client) doStatusOK(method, path string, body io.Reader) error {
	req, err := c.newRequest(method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: unexpected status %d", method, path, resp.StatusCode)
	}
	return nil
}

func (c *Client) getJSON(path string, out any) error {
	req, err := c.newRequest(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("get %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: unexpected status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := strings.TrimSuffix(c.BaseURL, "/") + "/" + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", path, err)
	}
	req.Header.Set("X-Auth-Token", c.AuthToken)
	return req, nil
}
