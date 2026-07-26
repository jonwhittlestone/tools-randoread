package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
)

func TestHandleClippingsListReturnsWithinWindow(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/recent.md", now.Add(-24*time.Hour)),
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/too-old.md", now.Add(-100*24*time.Hour)),
	}
	lister := &fakeLister{entries: entries}
	h := NewClippingsListHandler(lister, "/DropsyncFiles/jw-mind", func() time.Time { return now })

	req := httptest.NewRequest(http.MethodGet, "/api/clippings", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Clippings []struct {
			Path      string `json:"path"`
			Title     string `json:"title"`
			ClippedAt string `json:"clippedAt"`
		} `json:"clippings"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck

	if len(body.Clippings) != 1 {
		t.Fatalf("expected 1 clipping within the 3-month window, got %d: %+v", len(body.Clippings), body.Clippings)
	}
	if body.Clippings[0].Title != "recent" {
		t.Errorf("Title = %q, want %q", body.Clippings[0].Title, "recent")
	}
	if body.Clippings[0].Path != "/DropsyncFiles/jw-mind/Clippings/recent.md" {
		t.Errorf("Path = %q, want the full vault path", body.Clippings[0].Path)
	}
}

func TestHandleClippingsListSortedNewestFirst(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/older.md", now.Add(-48*time.Hour)),
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/newest.md", now.Add(-1*time.Hour)),
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/middle.md", now.Add(-24*time.Hour)),
	}
	lister := &fakeLister{entries: entries}
	h := NewClippingsListHandler(lister, "/DropsyncFiles/jw-mind", func() time.Time { return now })

	req := httptest.NewRequest(http.MethodGet, "/api/clippings", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		Clippings []struct {
			Title string `json:"title"`
		} `json:"clippings"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck

	want := []string{"newest", "middle", "older"}
	if len(body.Clippings) != len(want) {
		t.Fatalf("expected %d clippings, got %d", len(want), len(body.Clippings))
	}
	for i, w := range want {
		if body.Clippings[i].Title != w {
			t.Errorf("position %d: title = %q, want %q", i, body.Clippings[i].Title, w)
		}
	}
}

func TestHandleClippingsListExcludesConflictedCopies(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	entries := []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/article (conflicted copy 2026-01-09).md", now),
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/article.md", now.Add(-time.Hour)),
	}
	lister := &fakeLister{entries: entries}
	h := NewClippingsListHandler(lister, "/DropsyncFiles/jw-mind", func() time.Time { return now })

	req := httptest.NewRequest(http.MethodGet, "/api/clippings", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		Clippings []struct {
			Title string `json:"title"`
		} `json:"clippings"`
	}
	json.NewDecoder(rec.Body).Decode(&body) //nolint:errcheck

	if len(body.Clippings) != 1 || body.Clippings[0].Title != "article" {
		t.Fatalf("expected only the non-conflicted copy, got %+v", body.Clippings)
	}
}

func TestHandleClippingsListAlwaysListsFreshFromLister(t *testing.T) {
	// Per Jon: every click on the Clippings breadcrumb should refetch from
	// Dropbox — this handler must be wired with the uncached lister, not
	// the 24h vault-list cache Rando/Clipped share. That wiring itself is
	// asserted in main.go, but this at least pins that the handler always
	// calls through rather than caching internally.
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	lister := &fakeLister{entries: []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/a.md", now),
	}}
	h := NewClippingsListHandler(lister, "/DropsyncFiles/jw-mind", func() time.Time { return now })

	for i := range 3 {
		req := httptest.NewRequest(http.MethodGet, "/api/clippings", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected 200, got %d", i, rec.Code)
		}
	}

	if lister.calls != 3 {
		t.Errorf("expected the lister to be called once per request (3 total), got %d", lister.calls)
	}
}

func TestHandleClippingsListListsClippingsFolder(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	lister := &fakeLister{entries: []dropbox.Entry{
		mdEntryModified("/DropsyncFiles/jw-mind/Clippings/a.md", now),
	}}
	h := NewClippingsListHandler(lister, "/DropsyncFiles/jw-mind", func() time.Time { return now })

	req := httptest.NewRequest(http.MethodGet, "/api/clippings", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if lister.calledPath != "/DropsyncFiles/jw-mind/Clippings" {
		t.Errorf("expected the Clippings subfolder to be listed, got %q", lister.calledPath)
	}
}
