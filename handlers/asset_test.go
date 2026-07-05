package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAssetStreamsFile(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/assets/photo.png": []byte("fake-png-bytes"),
	}}
	h := NewAssetHandler(downloader, "/DropsyncFiles/jw-mind")

	req := httptest.NewRequest(http.MethodGet, "/api/asset?path=/DropsyncFiles/jw-mind/assets/photo.png", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "fake-png-bytes" {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("expected image/png content type, got %q", ct)
	}
}

func TestHandleAssetRejectsPathOutsideVault(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/etc/passwd": []byte("nope"),
	}}
	h := NewAssetHandler(downloader, "/DropsyncFiles/jw-mind")

	req := httptest.NewRequest(http.MethodGet, "/api/asset?path=/etc/passwd", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleAssetMissingFileReturns404(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{}}
	h := NewAssetHandler(downloader, "/DropsyncFiles/jw-mind")

	req := httptest.NewRequest(http.MethodGet, "/api/asset?path=/DropsyncFiles/jw-mind/assets/missing.png", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
