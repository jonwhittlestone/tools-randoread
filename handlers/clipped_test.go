package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
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
	h := NewClippedHandler(downloader, &fakeLister{entries: entries}, "/DropsyncFiles/jw-mind", func() time.Time { return now })
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

func TestHandleClippedIncludesDateClippedHeading(t *testing.T) {
	modified := time.Date(2026, 1, 10, 14, 5, 0, 0, time.UTC) // GMT in January, so also 14:05 in London
	entries := []dropbox.Entry{mdEntryModified("/DropsyncFiles/jw-mind/Clippings/a.md", modified)}
	h, _ := newTestClippedHandler(t, entries, modified.Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet, "/api/clipped", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		HTML string `json:"html"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck

	want := "<h3>Date Clipped: " + modified.In(randoLocation).Format("2006-01-02 15:04") + "</h3>"
	if !strings.Contains(body.HTML, want) {
		t.Fatalf("expected heading %q, got: %s", want, body.HTML)
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

func TestHandleClippedImageURLIncludesAuthToken(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/Clippings/a.md": []byte("![[photo.jpg]]"),
	}}
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/a.md", now),
		{Path: "/DropsyncFiles/jw-mind/assets/photo.jpg", Name: "photo.jpg"},
	}

	h := NewClippedHandler(downloader, &fakeLister{entries: entries}, "/DropsyncFiles/jw-mind", func() time.Time { return now })
	h.AuthToken = "secret-token"

	req := httptest.NewRequest(http.MethodGet, "/api/clipped", nil)
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

func TestHandleClippedHasNoCooldown(t *testing.T) {
	now := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{mdEntryModified("/DropsyncFiles/jw-mind/Clippings/a.md", now)}
	h, _ := newTestClippedHandler(t, entries, now)

	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/clipped", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}
}
