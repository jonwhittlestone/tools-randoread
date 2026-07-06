package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/state"
)

type fakeLister struct {
	entries    []dropbox.Entry
	err        error
	calledPath string
	calls      int
}

func (f *fakeLister) ListFolder(path string, recursive bool) ([]dropbox.Entry, error) {
	f.calledPath = path
	f.calls++
	return f.entries, f.err
}

func newTestRandoHandler(t *testing.T, entries []dropbox.Entry, now time.Time, pickIndex int) (*RandoHandler, *fakeDownloader) {
	t.Helper()
	downloader := &fakeDownloader{files: map[string][]byte{}}
	for _, e := range entries {
		if !e.IsFolder {
			downloader.files[e.Path] = []byte("## " + e.Name)
		}
	}

	pinStore := state.NewPinStore(filepath.Join(t.TempDir(), "pin.json"))
	h := NewRandoHandler(downloader, &fakeLister{entries: entries}, "/DropsyncFiles/jw-mind", pinStore, func() time.Time { return now }, func(n int) int { return pickIndex })
	return h, downloader
}

func mdEntry(path string) dropbox.Entry {
	return dropbox.Entry{Path: path, Name: filepath.Base(path), IsFolder: false}
}

func TestHandleRandoPicksAMarkdownFile(t *testing.T) {
	entries := []dropbox.Entry{
		{Path: "/DropsyncFiles/jw-mind/books", Name: "books", IsFolder: true},
		mdEntry("/DropsyncFiles/jw-mind/books/2026/happier-child-with-pda/main.md"),
		{Path: "/DropsyncFiles/jw-mind/assets/photo.png", Name: "photo.png", IsFolder: false},
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h, _ := newTestRandoHandler(t, entries, now, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Title string `json:"title"`
		HTML  string `json:"html"`
		Path  string `json:"path"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Title != "books / 2026 / happier-child-with-pda / main" {
		t.Errorf("unexpected title: %q", body.Title)
	}
	if body.Path != "/DropsyncFiles/jw-mind/books/2026/happier-child-with-pda/main.md" {
		t.Errorf("expected the chosen note's path so it can be emailed, got %q", body.Path)
	}
}

func TestHandleRandoExcludesConflictedCopies(t *testing.T) {
	entries := []dropbox.Entry{
		mdEntry("/DropsyncFiles/jw-mind/notes/idea (conflicted copy 2026-01-01).md"),
		mdEntry("/DropsyncFiles/jw-mind/notes/idea.md"),
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// pickIndex=0 would choose the conflicted copy if it weren't filtered out.
	h, _ := newTestRandoHandler(t, entries, now, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		Title string `json:"title"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body.Title != "notes / idea" {
		t.Errorf("expected the non-conflicted copy to be chosen, got title %q", body.Title)
	}
}

func TestHandleRandoExcludesExcalidrawFiles(t *testing.T) {
	entries := []dropbox.Entry{
		mdEntry("/DropsyncFiles/jw-mind/Excalidraw/Drawing 2026-01-01.excalidraw.md"),
		mdEntry("/DropsyncFiles/jw-mind/notes/idea.md"),
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// pickIndex=0 would choose the excalidraw file if it weren't filtered out.
	h, _ := newTestRandoHandler(t, entries, now, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		Title string `json:"title"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body.Title != "notes / idea" {
		t.Errorf("expected the excalidraw file to be excluded, got title %q", body.Title)
	}
}

func TestHandleRandoExcludesTemplatesDirectory(t *testing.T) {
	entries := []dropbox.Entry{
		mdEntry("/DropsyncFiles/jw-mind/templates/Daily.md"),
		mdEntry("/DropsyncFiles/jw-mind/notes/idea.md"),
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// pickIndex=0 would choose the template if it weren't filtered out.
	h, _ := newTestRandoHandler(t, entries, now, 0)

	req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		Title string `json:"title"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body.Title != "notes / idea" {
		t.Errorf("expected the templates/ file to be excluded, got title %q", body.Title)
	}
}

func TestHandleRandoImageURLIncludesAuthToken(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/a.md": []byte("![[photo.jpg]]"),
	}}
	entries := []dropbox.Entry{
		mdEntry("/DropsyncFiles/jw-mind/a.md"),
		{Path: "/DropsyncFiles/jw-mind/assets/photo.jpg", Name: "photo.jpg"},
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pinStore := state.NewPinStore(filepath.Join(t.TempDir(), "pin.json"))
	h := NewRandoHandler(downloader, &fakeLister{entries: entries}, "/DropsyncFiles/jw-mind", pinStore, func() time.Time { return now }, func(int) int { return 0 })
	h.AuthToken = "secret-token"

	req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		HTML string `json:"html"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if !strings.Contains(body.HTML, "token=secret-token") {
		t.Fatalf("expected the image URL to include the auth token, got: %s", body.HTML)
	}
}

func TestHandleRandoAlwaysReturns200(t *testing.T) {
	entries := []dropbox.Entry{mdEntry("/DropsyncFiles/jw-mind/a.md")}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, randoLocation)
	h, _ := newTestRandoHandler(t, entries, now, 0)

	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}
}

func TestHandleRandoReturnsSamePageWithinPeriod(t *testing.T) {
	entries := []dropbox.Entry{
		mdEntry("/DropsyncFiles/jw-mind/a.md"),
		mdEntry("/DropsyncFiles/jw-mind/b.md"),
	}
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, randoLocation)

	// pickIndex would choose a different candidate each time if actually
	// consulted on the second call — proves the second call short-circuits.
	calls := 0
	pickIndex := func(n int) int {
		idx := calls % n
		calls++
		return idx
	}

	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/a.md": []byte("## a"),
		"/DropsyncFiles/jw-mind/b.md": []byte("## b"),
	}}
	lister := &fakeLister{entries: entries}
	pinStore := state.NewPinStore(filepath.Join(t.TempDir(), "pin.json"))
	h := NewRandoHandler(downloader, lister, "/DropsyncFiles/jw-mind", pinStore, func() time.Time { return now }, pickIndex)

	var firstPath, secondPath string
	for i, dst := range []*string{&firstPath, &secondPath} {
		req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
		var body struct {
			Path string `json:"path"`
		}
		json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
		*dst = body.Path
	}

	if firstPath != secondPath {
		t.Fatalf("expected the same page within a period, got %q then %q", firstPath, secondPath)
	}
	if lister.calls != 1 {
		t.Fatalf("expected the vault to be listed only once (pinned second call shouldn't re-pick), got %d calls", lister.calls)
	}
}

func TestHandleRandoPicksNewPageAfterReset(t *testing.T) {
	entries := []dropbox.Entry{
		mdEntry("/DropsyncFiles/jw-mind/a.md"),
		mdEntry("/DropsyncFiles/jw-mind/b.md"),
	}
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/a.md": []byte("## a"),
		"/DropsyncFiles/jw-mind/b.md": []byte("## b"),
	}}
	lister := &fakeLister{entries: entries}
	pinStore := state.NewPinStore(filepath.Join(t.TempDir(), "pin.json"))

	now := time.Date(2026, 1, 2, 12, 0, 0, 0, randoLocation)
	calls := 0
	pickIndex := func(n int) int {
		idx := calls % n
		calls++
		return idx
	}
	h := NewRandoHandler(downloader, lister, "/DropsyncFiles/jw-mind", pinStore, func() time.Time { return now }, pickIndex)

	req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var first struct {
		Path string `json:"path"`
	}
	json.NewDecoder(rec.Body).Decode(&first) //nolint:errcheck

	// Cross the 4pm reset boundary into the next period.
	now = now.Add(25 * time.Hour)

	req = httptest.NewRequest(http.MethodGet, "/api/rando", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var second struct {
		Path string `json:"path"`
	}
	json.NewDecoder(rec.Body).Decode(&second) //nolint:errcheck

	if first.Path == second.Path {
		t.Fatalf("expected a new pick after the reset boundary, got %q both times", first.Path)
	}
	if lister.calls != 2 {
		t.Fatalf("expected the vault to be re-listed after reset, got %d calls", lister.calls)
	}
}
