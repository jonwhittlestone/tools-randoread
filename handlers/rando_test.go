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
)

type fakeLister struct {
	entries    []dropbox.Entry
	err        error
	calledPath string
}

func (f *fakeLister) ListFolder(path string, recursive bool) ([]dropbox.Entry, error) {
	f.calledPath = path
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

	h := NewRandoHandler(downloader, &fakeLister{entries: entries}, "/DropsyncFiles/jw-mind", func() time.Time { return now }, func(n int) int { return pickIndex })
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

func TestHandleRandoImageURLIncludesAuthToken(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/a.md": []byte("![[photo.jpg]]"),
	}}
	entries := []dropbox.Entry{mdEntry("/DropsyncFiles/jw-mind/a.md")}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	h := NewRandoHandler(downloader, &fakeLister{entries: entries}, "/DropsyncFiles/jw-mind", func() time.Time { return now }, func(int) int { return 0 })
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

func TestHandleRandoHasNoCooldown(t *testing.T) {
	entries := []dropbox.Entry{mdEntry("/DropsyncFiles/jw-mind/a.md")}
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	h, _ := newTestRandoHandler(t, entries, now, 0)

	// Rando should be clickable at any time — no gating, no state persisted.
	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/rando", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}
}
