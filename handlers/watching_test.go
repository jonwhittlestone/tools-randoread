package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/watchitlater"
)

// fakeWatchitlaterClient implements WatchitlaterClient for tests, recording
// calls and returning canned results/errors.
type fakeWatchitlaterClient struct {
	current       *watchitlater.Record
	currentErr    error
	startNextErr  error
	startNextCall bool
	reconcileErr  error
	reconcileCall bool
	status        *watchitlater.NextStatus
	statusErr     error
	setEmojiCalls []struct{ videoID, emoji string }
	setEmojiErr   error
	proxyVideoErr error
	proxyThumbErr error
	history       []watchitlater.HistoryRecord
	historyErr    error
	stageVideoErr error
	stagedVideoID string
}

func (f *fakeWatchitlaterClient) Current() (*watchitlater.Record, error) {
	return f.current, f.currentErr
}
func (f *fakeWatchitlaterClient) StartNext() error {
	f.startNextCall = true
	return f.startNextErr
}
func (f *fakeWatchitlaterClient) Reconcile() error {
	f.reconcileCall = true
	return f.reconcileErr
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
func (f *fakeWatchitlaterClient) History() ([]watchitlater.HistoryRecord, error) {
	return f.history, f.historyErr
}
func (f *fakeWatchitlaterClient) StageVideo(videoID string) error {
	f.stagedVideoID = videoID
	return f.stageVideoErr
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

func TestWatchingServeHTTP_IncludesCollapsedNotesContainer(t *testing.T) {
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], `class="watching-notes-toggle"`) {
		t.Errorf("html missing notes toggle: %s", body["html"])
	}
	if !strings.Contains(body["html"], `class="watching-notes-panel hidden"`) {
		t.Errorf("expected notes panel to start hidden/collapsed: %s", body["html"])
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

func TestWatchingServeHTTP_AutoAdvancesStaleTaggedVideo(t *testing.T) {
	// The staged video was tagged (watched, categorised) in a previous
	// daily-limit period — don't make the user click "Get Next Video" for
	// something they've already dealt with; just continue automatically.
	staleCategorizedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, randoLocation).Format(time.RFC3339)
	f := &fakeWatchitlaterClient{
		current: &watchitlater.Record{
			Staged: true, VideoID: "vid1", Title: "Old Video", Emoji: "✅",
			VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
			CategorizedAt: staleCategorizedAt,
		},
		status: &watchitlater.NextStatus{},
	}
	h := NewWatchingHandler(f)
	h.Now = func() time.Time { return time.Date(2026, 7, 5, 18, 0, 0, 0, randoLocation) }

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !f.reconcileCall {
		t.Error("expected Reconcile to be called for a stale, tagged video")
	}
	if f.startNextCall {
		t.Error("expected StartNext NOT to be called — reconciling back to a legitimately staged video must not cost the daily quota")
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if strings.Contains(body["html"], "Old Video") {
		t.Errorf("expected the fetching state, not the stale video's own page: %s", body["html"])
	}
}

func TestWatchingServeHTTP_DoesNotAutoAdvanceFreshTaggedVideo(t *testing.T) {
	// Same period as "now" — tagged today, quota may happen to still be
	// available (e.g. the very first video ever), but it was NOT tagged in
	// a previous period, so this must not auto-advance.
	freshCategorizedAt := time.Date(2026, 7, 5, 17, 0, 0, 0, randoLocation).Format(time.RFC3339)
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Fresh Video", Emoji: "✅",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
		CategorizedAt: freshCategorizedAt,
	}}
	h := NewWatchingHandler(f)
	h.Now = func() time.Time { return time.Date(2026, 7, 5, 18, 0, 0, 0, randoLocation) }

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if f.startNextCall {
		t.Error("expected no auto-advance for a video staged in the current period")
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], "Fresh Video") {
		t.Errorf("expected the video's own page, got: %s", body["html"])
	}
}

// TestWatchingServeHTTP_AutoAdvancesReplayedHistoryVideo guards against the
// actual production bug found while smoke-testing "Load watch later
// videos": re-staging an old, already-tagged video from the history table
// sets StagedAt to right now, but its tag (CategorizedAt) is from long
// ago. Before videoIsStaleAndTagged was switched to key off CategorizedAt
// instead of StagedAt, the fresh StagedAt made a months-old replay look
// like "tagged today" and get shown on the default view, instead of
// auto-advancing back to the genuine current/uncategorized video.
func TestWatchingServeHTTP_AutoAdvancesReplayedHistoryVideo(t *testing.T) {
	longAgoCategorizedAt := time.Date(2018, 11, 28, 12, 0, 0, 0, randoLocation).Format(time.RFC3339)
	justNowStagedAt := time.Date(2026, 7, 5, 17, 59, 0, 0, randoLocation).Format(time.RFC3339)
	f := &fakeWatchitlaterClient{
		current: &watchitlater.Record{
			Staged: true, VideoID: "vid1", Title: "Walking Bass Line Formula", Emoji: "🎸",
			VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
			StagedAt: justNowStagedAt, CategorizedAt: longAgoCategorizedAt,
		},
		status: &watchitlater.NextStatus{},
	}
	h := NewWatchingHandler(f)
	h.Now = func() time.Time { return time.Date(2026, 7, 5, 18, 0, 0, 0, randoLocation) }

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !f.reconcileCall {
		t.Error("expected Reconcile to be called to advance past the replayed history video")
	}
	if f.startNextCall {
		t.Error("expected StartNext NOT to be called — today's real video already consumed the daily quota, so getting back to it must not go through the gated path")
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if strings.Contains(body["html"], "Walking Bass Line Formula") {
		t.Errorf("expected the fetching state, not the replayed video's own page: %s", body["html"])
	}
}

// TestWatchingServeHTTP_SkipStaleCheckShowsReplayedVideoAsIs guards against
// the actual production bug found right after the fix above shipped: every
// history video is, by definition, already tagged, so the very next poll
// after stageHistoryVideo's own staging job finished immediately looked
// "stale and tagged" by this exact same check — silently replacing the
// video the user just deliberately picked from history with whatever else
// was actually next in the uncategorized queue. skipStaleCheck=1 is what
// app.js now passes for that one follow-up load only.
func TestWatchingServeHTTP_SkipStaleCheckShowsReplayedVideoAsIs(t *testing.T) {
	longAgoCategorizedAt := time.Date(2018, 11, 28, 12, 0, 0, 0, randoLocation).Format(time.RFC3339)
	justNowStagedAt := time.Date(2026, 7, 5, 17, 59, 0, 0, randoLocation).Format(time.RFC3339)
	f := &fakeWatchitlaterClient{
		current: &watchitlater.Record{
			Staged: true, VideoID: "vid1", Title: "Walking Bass Line Formula", Emoji: "🎸",
			VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
			StagedAt: justNowStagedAt, CategorizedAt: longAgoCategorizedAt,
		},
		status: &watchitlater.NextStatus{},
	}
	h := NewWatchingHandler(f)
	h.Now = func() time.Time { return time.Date(2026, 7, 5, 18, 0, 0, 0, randoLocation) }

	req := httptest.NewRequest(http.MethodGet, "/api/watching?skipStaleCheck=1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if f.reconcileCall || f.startNextCall {
		t.Error("expected no reconcile/advance when skipStaleCheck=1 — the user just deliberately picked this video")
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], "Walking Bass Line Formula") {
		t.Errorf("expected the replayed video's own page, got: %s", body["html"])
	}
}

func TestWatchingServeHTTP_DoesNotAutoAdvanceStaleUntaggedVideo(t *testing.T) {
	// Per explicit instruction: even if 24h has elapsed, an uncategorised
	// video must NOT auto-advance — the user has to tag it first.
	staleStagedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, randoLocation).Format(time.RFC3339)
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Untagged Old Video", Emoji: "",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
		StagedAt: staleStagedAt,
	}}
	h := NewWatchingHandler(f)
	h.Now = func() time.Time { return time.Date(2026, 7, 5, 18, 0, 0, 0, randoLocation) }

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if f.startNextCall {
		t.Error("expected no auto-advance for an uncategorised video, however stale")
	}
	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], "Untagged Old Video") {
		t.Errorf("expected the video's own page, got: %s", body["html"])
	}
	if !strings.Contains(body["html"], `class="watching-next-btn" disabled`) {
		t.Errorf("expected the plain disabled arrow state, got: %s", body["html"])
	}
}

func TestWatchingServeHTTP_StaleTaggedVideoRespectsAlreadyRunningJob(t *testing.T) {
	staleCategorizedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, randoLocation).Format(time.RFC3339)
	f := &fakeWatchitlaterClient{
		current: &watchitlater.Record{
			Staged: true, VideoID: "vid1", Title: "Old Video", Emoji: "✅",
			VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
			CategorizedAt: staleCategorizedAt,
		},
		status: &watchitlater.NextStatus{Running: true},
	}
	h := NewWatchingHandler(f)
	h.Now = func() time.Time { return time.Date(2026, 7, 5, 18, 0, 0, 0, randoLocation) }

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if f.startNextCall {
		t.Error("expected no duplicate StartNext while a job is already running")
	}
	if f.reconcileCall {
		t.Error("expected no duplicate Reconcile while a job is already running")
	}
}

func TestWatchingServeHTTP_DisablesNextWithClockLabelWhenDailyLimitReached(t *testing.T) {
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title", Emoji: "🎸",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
		DailyLimitReached: true,
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], `class="watching-next-btn" disabled`) {
		t.Errorf("Get Next Video should be disabled when the daily limit is reached: %s", body["html"])
	}
	if !strings.Contains(body["html"], "Get Next Video ⏰") {
		t.Errorf("expected the clock label when the daily limit is reached: %s", body["html"])
	}
}

func TestWatchingServeHTTP_IncludesNextFreshVideoLabelWhenDailyLimitReached(t *testing.T) {
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title", Emoji: "🎸",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
		DailyLimitReached: true,
		NextFreshVideoAt:  time.Date(2026, 5, 15, 13, 0, 0, 0, randoLocation).Format(time.RFC3339),
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	want := "Next fresh video ☕: Fri 13:00 15.05.26"
	if body["titleRight"] != want {
		t.Errorf("titleRight = %q, want %q", body["titleRight"], want)
	}
}

func TestWatchingServeHTTP_OmitsNextFreshVideoLabelWhenLimitAvailable(t *testing.T) {
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title", Emoji: "🎸",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
		DailyLimitReached: false,
		NextFreshVideoAt:  time.Date(2026, 5, 15, 13, 0, 0, 0, randoLocation).Format(time.RFC3339),
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body["titleRight"] != "" {
		t.Errorf("titleRight = %q, want empty (limit is available, no wait to show)", body["titleRight"])
	}
}

func TestWatchingServeHTTP_OmitsNextFreshVideoLabelWhenUncategorized(t *testing.T) {
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid1", Title: "Some Title", Emoji: "",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
		DailyLimitReached: true,
		NextFreshVideoAt:  time.Date(2026, 5, 15, 13, 0, 0, 0, randoLocation).Format(time.RFC3339),
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body["titleRight"] != "" {
		t.Errorf("titleRight = %q, want empty (not yet tagged, no wait to show)", body["titleRight"])
	}
}

func TestWatchingServeHTTP_UncategorizedTakesPrecedenceOverDailyLimitLabel(t *testing.T) {
	// The realistic post-advance state: DailyLimitReached flips true the
	// instant a Next call succeeds, but the freshly staged video is always
	// uncategorized. The button must still read the plain arrow (and stay
	// disabled for the "not tagged yet" reason), not the clock — the clock
	// is specifically for "you tagged it, but you're out of quota today."
	f := &fakeWatchitlaterClient{current: &watchitlater.Record{
		Staged: true, VideoID: "vid2", Title: "Fresh Video", Emoji: "",
		VideoURL: "api/watching/video", ThumbnailURL: "api/watching/thumbnail",
		DailyLimitReached: true,
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body["html"], `class="watching-next-btn" disabled`) {
		t.Errorf("expected disabled: %s", body["html"])
	}
	if !strings.Contains(body["html"], "Get Next Video →") {
		t.Errorf("expected the plain arrow label (uncategorized takes precedence), got: %s", body["html"])
	}
	if strings.Contains(body["html"], "⏰") {
		t.Errorf("did not expect the clock label while still uncategorized: %s", body["html"])
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

func TestHandleHistory_ReturnsVideosFromClient(t *testing.T) {
	f := &fakeWatchitlaterClient{history: []watchitlater.HistoryRecord{
		{VideoID: "vid1", Title: "Some Title", Emoji: "🎸", CategorizedAt: "2026-08-01T00:00:00Z"},
	}}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/history", nil)
	rec := httptest.NewRecorder()
	h.HandleHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Videos []watchitlater.HistoryRecord `json:"videos"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Videos) != 1 || body.Videos[0].VideoID != "vid1" {
		t.Fatalf("unexpected videos: %+v", body.Videos)
	}
}

func TestHandleHistory_UpstreamErrorReturns502(t *testing.T) {
	f := &fakeWatchitlaterClient{historyErr: errors.New("connection refused")}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/history", nil)
	rec := httptest.NewRecorder()
	h.HandleHistory(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestHandleStage_DelegatesVideoIDToClient(t *testing.T) {
	f := &fakeWatchitlaterClient{}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/stage/vid1", nil)
	req.SetPathValue("videoID", "vid1")
	rec := httptest.NewRecorder()
	h.HandleStage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if f.stagedVideoID != "vid1" {
		t.Errorf("stagedVideoID = %q, want vid1", f.stagedVideoID)
	}
}

func TestHandleStage_MissingVideoID(t *testing.T) {
	f := &fakeWatchitlaterClient{}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/stage/", nil)
	req.SetPathValue("videoID", "")
	rec := httptest.NewRecorder()
	h.HandleStage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleStage_UpstreamErrorReturns502(t *testing.T) {
	f := &fakeWatchitlaterClient{stageVideoErr: errors.New("409 conflict")}
	h := NewWatchingHandler(f)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/stage/vid1", nil)
	req.SetPathValue("videoID", "vid1")
	rec := httptest.NewRecorder()
	h.HandleStage(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}
