package handlers

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/remarkable"
)

// fakeSender captures what SendEpub was called with, without touching a
// real tablet — mirrors EmailHandler's SendFunc test pattern. It reads the
// epub file's bytes immediately (the handler removes it via a deferred
// cleanup right after Send returns, so the path itself is gone by the time
// a test could inspect it afterwards).
type fakeSender struct {
	epubBytes []byte
	title     string
	err       error
}

func (f *fakeSender) send(epubPath, title string) (*remarkable.UploadResult, error) {
	data, readErr := os.ReadFile(epubPath)
	if readErr == nil {
		f.epubBytes = data
	}
	f.title = title
	if f.err != nil {
		return nil, f.err
	}
	return &remarkable.UploadResult{UUID: "fake-uuid", VisibleName: title}, nil
}

func TestHandleClippingsSendDeliversEpubWithPrefixedTitle(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/Clippings/a.md": []byte("# Hello\n\nSome clipped text."),
	}}
	sender := &fakeSender{}
	h := NewClippingsSendHandler(downloader, &fakeLister{}, "/DropsyncFiles/jw-mind", sender.send)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"path":  "/DropsyncFiles/jw-mind/Clippings/a.md",
		"title": "The AI jobs apocalypse",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/clippings/send-to-remarkable", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	wantTitle := "✂️ RANDOREAD CLIP: The AI jobs apocalypse"
	if sender.title != wantTitle {
		t.Errorf("title sent to the tablet = %q, want %q", sender.title, wantTitle)
	}

	if len(sender.epubBytes) == 0 {
		t.Fatal("expected SendEpub to be called with a readable, non-empty epub file")
	}
	r, err := zip.NewReader(bytes.NewReader(sender.epubBytes), int64(len(sender.epubBytes)))
	if err != nil {
		t.Fatalf("delivered file is not a valid epub/zip: %v", err)
	}
	found := false
	for _, f := range r.File {
		if f.Name == "mimetype" {
			found = true
		}
	}
	if !found {
		t.Error("delivered epub is missing the mimetype entry")
	}
}

func TestHandleClippingsSendMissingPathReturns400(t *testing.T) {
	sender := &fakeSender{}
	h := NewClippingsSendHandler(&fakeDownloader{files: map[string][]byte{}}, &fakeLister{}, "/DropsyncFiles/jw-mind", sender.send)

	body, _ := json.Marshal(map[string]string{"title": "no path here"}) //nolint:errcheck
	req := httptest.NewRequest(http.MethodPost, "/api/clippings/send-to-remarkable", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleClippingsSendDownloadFailureReturns502(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{}, err: errors.New("dropbox down")}
	sender := &fakeSender{}
	h := NewClippingsSendHandler(downloader, &fakeLister{}, "/DropsyncFiles/jw-mind", sender.send)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"path": "/DropsyncFiles/jw-mind/Clippings/a.md", "title": "t",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/clippings/send-to-remarkable", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleClippingsSendDeliveryFailureReturns502(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/Clippings/a.md": []byte("body"),
	}}
	sender := &fakeSender{err: errors.New("ssh dial failed")}
	h := NewClippingsSendHandler(downloader, &fakeLister{}, "/DropsyncFiles/jw-mind", sender.send)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"path": "/DropsyncFiles/jw-mind/Clippings/a.md", "title": "t",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/clippings/send-to-remarkable", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "deliver") {
		t.Errorf("expected a delivery-failure error message, got: %s", rec.Body.String())
	}
}

func TestHandleClippingsSendEmbedsResolvedImage(t *testing.T) {
	downloader := &fakeDownloader{files: map[string][]byte{
		"/DropsyncFiles/jw-mind/Clippings/a.md":   []byte("![[photo.jpg]]"),
		"/DropsyncFiles/jw-mind/assets/photo.jpg": {0xFF, 0xD8, 0xFF, 0xE0},
	}}
	lister := &fakeLister{entries: []dropbox.Entry{
		{Path: "/DropsyncFiles/jw-mind/assets/photo.jpg", Name: "photo.jpg"},
	}}
	sender := &fakeSender{}

	h := NewClippingsSendHandler(downloader, lister, "/DropsyncFiles/jw-mind", sender.send)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"path": "/DropsyncFiles/jw-mind/Clippings/a.md", "title": "t",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/clippings/send-to-remarkable", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(sender.epubBytes) == 0 {
		t.Fatal("expected SendEpub to be called with a readable, non-empty epub file")
	}
	r, err := zip.NewReader(bytes.NewReader(sender.epubBytes), int64(len(sender.epubBytes)))
	if err != nil {
		t.Fatalf("delivered file is not a valid epub/zip: %v", err)
	}
	found := false
	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "EPUB/images/") {
			found = true
		}
	}
	if !found {
		t.Error("expected the referenced image to be embedded in the delivered epub")
	}
}
