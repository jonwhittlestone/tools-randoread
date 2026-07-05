package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleEmailSendsRenderedNote(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/books/2026/main.md": []byte("## Hi\n\n![[photo.png]]"),
	}}

	var gotSubject, gotHTML string
	sendFunc := func(subject, html string) error {
		gotSubject, gotHTML = subject, html
		return nil
	}

	h := NewEmailHandler(downloader, "/DropsyncFiles/jw-mind", "https://howapped.zapto.org/randoread/", "secret-token", sendFunc)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"path":  "/DropsyncFiles/jw-mind/books/2026/main.md",
		"title": "books / 2026 / main",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/email", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(gotSubject, "books / 2026 / main") {
		t.Errorf("expected subject to include the title, got %q", gotSubject)
	}
	if !strings.Contains(gotHTML, "<h2>Hi</h2>") {
		t.Errorf("expected rendered note content, got %q", gotHTML)
	}
	if !strings.Contains(gotHTML, "https://howapped.zapto.org/randoread/api/asset?path=") {
		t.Errorf("expected an absolute, token-bearing asset URL for the image, got %q", gotHTML)
	}
	if !strings.Contains(gotHTML, "token=secret-token") {
		t.Errorf("expected the asset URL to carry the auth token, got %q", gotHTML)
	}
}

func TestHandleEmailMissingPathReturns400(t *testing.T) {
	h := NewEmailHandler(&fakeDownloader{}, "/DropsyncFiles/jw-mind", "https://example.com/", "token", func(string, string) error { return nil })

	req := httptest.NewRequest(http.MethodPost, "/api/email", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleEmailDownloadFailureReturns502(t *testing.T) {
	h := NewEmailHandler(&fakeDownloader{files: map[string][]byte{}}, "/DropsyncFiles/jw-mind", "https://example.com/", "token", func(string, string) error { return nil })

	body, _ := json.Marshal(map[string]string{"path": "/DropsyncFiles/jw-mind/missing.md", "title": "missing"}) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPost, "/api/email", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestHandleEmailSendFailureReturns502(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{"/DropsyncFiles/jw-mind/a.md": []byte("hi")}}
	sendFunc := func(string, string) error { return errors.New("smtp exploded") }
	h := NewEmailHandler(downloader, "/DropsyncFiles/jw-mind", "https://example.com/", "token", sendFunc)

	body, _ := json.Marshal(map[string]string{"path": "/DropsyncFiles/jw-mind/a.md", "title": "a"}) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPost, "/api/email", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}
