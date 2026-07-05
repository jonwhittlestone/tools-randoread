package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeDownloader struct {
	files map[string][]byte
	err   error
}

func (f *fakeDownloader) Download(path string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.files[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return data, nil
}

func TestHandleDailyRendersTodaysNote(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/periodic/daily/2026-07-05-W27-Sun.md": []byte("## Hello\n\nworld"),
	}}
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)

	h := NewDailyHandler(downloader, "/DropsyncFiles/jw-mind", func() time.Time { return now })

	req := httptest.NewRequest(http.MethodGet, "/api/daily", nil)
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
	if body.Title != "2026-07-05-W27-Sun.md" {
		t.Errorf("expected title to be the bare filename, got %q", body.Title)
	}
	if body.HTML == "" || !strings.Contains(body.HTML, "<h2>Hello</h2>") {
		t.Errorf("expected rendered HTML, got %q", body.HTML)
	}
	if body.Path != "/DropsyncFiles/jw-mind/periodic/daily/2026-07-05-W27-Sun.md" {
		t.Errorf("expected the full dropbox path so the note can be emailed, got %q", body.Path)
	}
}

func TestHandleDailyMissingNoteReturnsError(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{}}
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)

	h := NewDailyHandler(downloader, "/DropsyncFiles/jw-mind", func() time.Time { return now })

	req := httptest.NewRequest(http.MethodGet, "/api/daily", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}
