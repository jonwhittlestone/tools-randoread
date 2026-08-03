package watchitlater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c := NewClient(srv.URL, "test-token")
	return c
}

func TestCurrent_Staged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/watching/current" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Auth-Token"); got != "test-token" {
			t.Errorf("X-Auth-Token = %q, want test-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"staged":       true,
			"videoID":      "vid1",
			"title":        "Some Title",
			"youtubeURL":   "https://www.youtube.com/watch?v=vid1",
			"downloadedAt": "2026-01-01T00:00:00Z",
			"uploadedAt":   "2026-01-01T00:05:00Z",
			"playlistRank": 3,
			"emoji":        "🎸",
			"videoURL":     "api/watching/video",
			"thumbnailURL": "api/watching/thumbnail",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	record, err := c.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if !record.Staged || record.VideoID != "vid1" || record.PlaylistRank != 3 || record.Emoji != "🎸" {
		t.Errorf("Current() = %+v", record)
	}
}

func TestCurrent_NotStaged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"staged": false}) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	record, err := c.Current()
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if record.Staged {
		t.Errorf("Staged = true, want false")
	}
}

func TestStartNext_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewEncoder(w).Encode(map[string]string{"status": "started"}) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.StartNext(); err != nil {
		t.Fatalf("StartNext() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/watching/next" {
		t.Errorf("request = %s %s, want POST /api/watching/next", gotMethod, gotPath)
	}
}

func TestStartNext_ConflictIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.StartNext(); err == nil {
		t.Error("StartNext() error = nil, want non-nil on 409")
	}
}

func TestNextStatus_ReportsProgress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/watching/next/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"running":          true,
			"done":             false,
			"noneLeft":         false,
			"bytesTransferred": 42,
			"totalBytes":       100,
			"percent":          42.0,
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	status, err := c.NextStatus()
	if err != nil {
		t.Fatalf("NextStatus() error = %v", err)
	}
	if !status.Running || status.Percent != 42.0 || status.TotalBytes != 100 {
		t.Errorf("NextStatus() = %+v", status)
	}
}

func TestSetEmoji_SendsExpectedRequest(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "emoji": "🎸"}) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.SetEmoji("vid1", "🎸"); err != nil {
		t.Fatalf("SetEmoji() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/downloads/vid1/emoji" {
		t.Errorf("request = %s %s, want POST /api/downloads/vid1/emoji", gotMethod, gotPath)
	}
	if gotBody != `{"emoji":"🎸"}` {
		t.Errorf("body = %q", gotBody)
	}
}

func TestSetEmoji_NotFoundIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.SetEmoji("never-tracked", "🎸"); err == nil {
		t.Error("SetEmoji() error = nil, want non-nil on 404")
	}
}

func TestProxyVideo_ForwardsRangeHeaderAndUpstreamResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/watching/video" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Range"); got != "bytes=100-199" {
			t.Errorf("Range header = %q, want bytes=100-199", got)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 100-199/1000")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("partial-video-bytes")) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/api/watching/video", nil)
	req.Header.Set("Range", "bytes=100-199")
	rec := httptest.NewRecorder()

	if err := c.ProxyVideo(rec, req); err != nil {
		t.Fatalf("ProxyVideo() error = %v", err)
	}
	if rec.Code != http.StatusPartialContent {
		t.Errorf("status = %d, want 206", rec.Code)
	}
	if rec.Body.String() != "partial-video-bytes" {
		t.Errorf("body = %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Range") != "bytes 100-199/1000" {
		t.Errorf("Content-Range = %q", rec.Header().Get("Content-Range"))
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges = %q", rec.Header().Get("Accept-Ranges"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
}

func TestProxyThumbnail_PassesThroughBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/watching/thumbnail" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("jpeg-bytes")) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/api/watching/thumbnail", nil)
	rec := httptest.NewRecorder()

	if err := c.ProxyThumbnail(rec, req); err != nil {
		t.Fatalf("ProxyThumbnail() error = %v", err)
	}
	if rec.Body.String() != "jpeg-bytes" {
		t.Errorf("body = %q", rec.Body.String())
	}
}
