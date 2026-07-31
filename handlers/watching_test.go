package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jonwhittlestone/tools-randoread/internal/watchitlater"
)

// fakeWatchitlaterClient implements WatchitlaterClient for tests, recording
// calls and returning canned results/errors.
type fakeWatchitlaterClient struct {
	current       *watchitlater.Record
	currentErr    error
	startNextErr  error
	startNextCall bool
	status        *watchitlater.NextStatus
	statusErr     error
	setEmojiCalls []struct{ videoID, emoji string }
	setEmojiErr   error
	proxyVideoErr error
	proxyThumbErr error
}

func (f *fakeWatchitlaterClient) Current() (*watchitlater.Record, error) {
	return f.current, f.currentErr
}
func (f *fakeWatchitlaterClient) StartNext() error {
	f.startNextCall = true
	return f.startNextErr
}
func (f *fakeWatchitlaterClient) NextStatus() (*watchitlater.NextStatus, error) {
	return f.status, f.statusErr
}
func (f *fakeWatchitlaterClient) SetEmoji(videoID, emoji string) error {
	f.setEmojiCalls = append(f.setEmojiCalls, struct{ videoID, emoji string }{videoID, emoji})
	return f.setEmojiErr
}
func (f *fakeWatchitlaterClient) ProxyVideo(w http.ResponseWriter, r *http.Request) error {
	if f.proxyVideoErr != nil {
		return f.proxyVideoErr
	}
	w.Write([]byte("video-bytes")) //nolint:errcheck
	return nil
}
func (f *fakeWatchitlaterClient) ProxyThumbnail(w http.ResponseWriter, r *http.Request) error {
	if f.proxyThumbErr != nil {
		return f.proxyThumbErr
	}
	w.Write([]byte("thumb-bytes")) //nolint:errcheck
	return nil
}

func TestWatchingServeHTTP_RendersStagedRecord(t *testing.T) {
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title",
		YoutubeURL:   "https://www.youtube.com/watch?v=vid1",
		DownloadedAt: "2026-01-01T00:00:00Z", UploadedAt: "2026-01-01T00:05:00Z",
		PlaylistRank: 3, Emoji: "", VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["html"], "Some Title") {
		t.Errorf("html missing title: %s", body["html"])
	}
	if !strings.Contains(body["html"], "api/watching/video") {
		t.Errorf("html missing video src: %s", body["html"])
	}
	if !strings.Contains(body["html"], `disabled`) {
		t.Errorf("html should disable Get Next Video button when uncategorized: %s", body["html"])
	}
	if f.startNextCall {
		t.Error("StartNext should not be called when something is already staged")
	}
}

func TestWatchingServeHTTP_EmbedsAuthTokenInVideoAndPosterURLs(t *testing.T) {
	// <video src>/poster can't send the X-Auth-Token header, so the token
	// must travel as a query param — same pattern as vaultFileResolver's
	// /api/asset URLs (RequireToken accepts either; see handlers/auth.go).
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
	}}
	h := NewWatchingHandler(f)
	h.AuthToken = "secret-token"

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], `src="api/watching/video?token=secret-token"`) {
		t.Errorf("video src missing token: %s", body["html"])
	}
	if !strings.Contains(body["html"], `poster="api/watching/thumbnail?token=secret-token"`) {
		t.Errorf("poster missing token: %s", body["html"])
	}
}

func TestWatchingServeHTTP_EnablesNextButtonWhenCategorized(t *testing.T) {
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title", Emoji: "🎸",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if strings.Contains(body["html"], `class="watching-next-btn" disabled`) {
		t.Errorf("Get Next Video button should be enabled once categorized: %s", body["html"])
	}
}

func TestWatchingServeHTTP_BootstrapsFirstVideoWhenNothingStaged(t *testing.T) {
	f := &fakeWatchitlaterClient{
		current: &watchitlater.Record{Staged: false},
		status:  &watchitlater.NextStatus{}, // nothing has ever run
	}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !f.startNextCall {
		t.Error("expected StartNext to be called to bootstrap the first video")
	}
}

func TestWatchingServeHTTP_DoesNotRestartAnAlreadyRunningJob(t *testing.T) {
	// While a next-video job is mid-download, Current() reports staged:false
	// (the previous local file was already cleared) — reloading the view in
	// that window must NOT call StartNext again, since tools-watchitlater
	// would 409 a concurrent start and this handler would surface that as a
	// spurious 502 instead of just showing the in-progress state.
	f := &fakeWatchitlaterClient{
		current: &watchitlater.Record{Staged: false},
		status:  &watchitlater.NextStatus{Running: true, BytesTransferred: 50, TotalBytes: 100, Percent: 50},
	}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if f.startNextCall {
		t.Error("StartNext should not be called while a job is already running")
	}
}

func TestWatchingServeHTTP_ShowsCaughtUpWhenNoneLeft(t *testing.T) {
	f := &fakeWatchitlaterClient{
		current: &watchitlater.Record{Staged: false},
		status:  &watchitlater.NextStatus{NoneLeft: true, Done: true},
	}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if f.startNextCall {
		t.Error("StartNext should not be called when nothing is left to categorize")
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], "caught up") {
		t.Errorf("expected a caught-up message, got: %s", body["html"])
	}
}

func TestWatchingServeHTTP_NextStatusErrorReturns502(t *testing.T) {
	f := &fakeWatchitlaterClient{
		current:   &watchitlater.Record{Staged: false},
		statusErr: errors.New("connection refused"),
	}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestWatchingServeHTTP_UpstreamErrorReturns502(t *testing.T) {
	f := &fakeWatchitlaterClient{currentErr: errors.New("connection refused")}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestHandleEmoji_ProxiesToClient(t *testing.T) {
	f := &fakeWatchitlaterClient{}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/emoji", strings.NewReader(`{"videoID":"vid1","emoji":"🎸"}`))
	rec := httptest.NewRecorder()
	h.HandleEmoji(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(f.setEmojiCalls) != 1 || f.setEmojiCalls[0].videoID != "vid1" || f.setEmojiCalls[0].emoji != "🎸" {
		t.Errorf("SetEmoji calls = %+v", f.setEmojiCalls)
	}
}

func TestHandleEmoji_MissingVideoID(t *testing.T) {
	f := &fakeWatchitlaterClient{}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/emoji", strings.NewReader(`{"emoji":"🎸"}`))
	rec := httptest.NewRecorder()
	h.HandleEmoji(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleEmoji_UpstreamErrorReturns502(t *testing.T) {
	f := &fakeWatchitlaterClient{setEmojiErr: errors.New("upstream down")}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/emoji", strings.NewReader(`{"videoID":"vid1","emoji":"🎸"}`))
	rec := httptest.NewRecorder()
	h.HandleEmoji(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestHandleNext_ProxiesToClient(t *testing.T) {
	f := &fakeWatchitlaterClient{}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/next", nil)
	rec := httptest.NewRecorder()
	h.HandleNext(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !f.startNextCall {
		t.Error("expected StartNext to be called")
	}
}

func TestHandleNext_UpstreamErrorReturns502(t *testing.T) {
	f := &fakeWatchitlaterClient{startNextErr: errors.New("already running")}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/next", nil)
	rec := httptest.NewRecorder()
	h.HandleNext(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestHandleNextStatus_ReturnsStatusJSON(t *testing.T) {
	f := &fakeWatchitlaterClient{status: &watchitlater.NextStatus{Running: true, Percent: 42}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/next/status", nil)
	rec := httptest.NewRecorder()
	h.HandleNextStatus(rec, req)

	var body watchitlater.NextStatus
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.Running || body.Percent != 42 {
		t.Errorf("body = %+v", body)
	}
}

func TestHandleVideo_DelegatesToClient(t *testing.T) {
	f := &fakeWatchitlaterClient{}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/video", nil)
	rec := httptest.NewRecorder()
	h.HandleVideo(rec, req)

	if rec.Body.String() != "video-bytes" {
		t.Errorf("body = %q, want video-bytes", rec.Body.String())
	}
}

func TestHandleThumbnail_DelegatesToClient(t *testing.T) {
	f := &fakeWatchitlaterClient{}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/thumbnail", nil)
	rec := httptest.NewRecorder()
	h.HandleThumbnail(rec, req)

	if rec.Body.String() != "thumb-bytes" {
		t.Errorf("body = %q, want thumb-bytes", rec.Body.String())
	}
}
