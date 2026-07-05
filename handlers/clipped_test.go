package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/state"
)

func mdEntryModified(path string, modified time.Time) dropbox.Entry {
	e := mdEntry(path)
	e.ModifiedAt = modified
	return e
}

func newTestClippedHandler(t *testing.T, entries []dropbox.Entry, now time.Time) (*ClippedHandler, *fakeDownloader) {
	t.Helper()
	downloader := &fakeDownloader{files: map[string][]byte{}}
	for _, e := range entries {
		if !e.IsFolder {
			downloader.files[e.Path] = []byte("## " + e.Name)
		}
	}
	store := state.NewCooldownStore(filepath.Join(t.TempDir(), "clipped.json"))
	h := NewClippedHandler(downloader, &fakeLister{entries: entries}, "/DropsyncFiles/jw-mind", store, func() time.Time { return now })
	return h, downloader
}

func TestHandleClippedPicksMostRecent(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/older.md", now.Add(-72*time.Hour)),
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/newest.md", now.Add(-1*time.Hour)),
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/middle.md", now.Add(-24*time.Hour)),
	}
	h, _ := newTestClippedHandler(t, entries, now)

	req := httptest.NewRequest(http.MethodGet, "/api/clipped", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Title string `json:"title"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body.Title != "Clippings / newest" {
		t.Errorf("expected the most recently modified clipping, got title %q", body.Title)
	}
}

func TestHandleClippedOnlyListsClippingsFolder(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{mdEntryModified("/DropsyncFiles/jw-mind/Clippings/a.md", now)}
	h, _ := newTestClippedHandler(t, entries, now)

	req := httptest.NewRequest(http.MethodGet, "/api/clipped", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	lister := h.Lister.(*fakeLister)
	if lister.calledPath != "/DropsyncFiles/jw-mind/Clippings" {
		t.Errorf("expected ClippedHandler to list the Clippings subfolder, got %q", lister.calledPath)
	}
}

func TestHandleClippedOnCooldownReturns429(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{mdEntryModified("/DropsyncFiles/jw-mind/Clippings/a.md", now)}
	h, _ := newTestClippedHandler(t, entries, now)

	if err := h.Store.Save(state.Cooldown{Path: entries[0].Path, LastFetchedAt: now.Add(-1 * time.Hour)}); err != nil {
		t.Fatalf("seed cooldown: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/clipped", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestHandleClippedExcludesConflictedCopies(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/article (conflicted copy 2026-01-09).md", now),
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/article.md", now.Add(-time.Hour)),
	}
	h, _ := newTestClippedHandler(t, entries, now)

	req := httptest.NewRequest(http.MethodGet, "/api/clipped", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		Title string `json:"title"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck
	if body.Title != "Clippings / article" {
		t.Errorf("expected the non-conflicted copy despite being older, got title %q", body.Title)
	}
}
