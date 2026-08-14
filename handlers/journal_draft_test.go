package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/journaldraft"
)

type fakeJournalDraftClient struct {
	available bool
	result    *journaldraft.Result
	err       error
	gotNote   string
	gotText   string
	gotNowISO string
}

func (f *fakeJournalDraftClient) Available() bool { return f.available }

func (f *fakeJournalDraftClient) Draft(ctx context.Context, dailyNoteRaw, userText, nowISO string) (*journaldraft.Result, error) {
	f.gotNote, f.gotText, f.gotNowISO = dailyNoteRaw, userText, nowISO
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func fixedNow() time.Time {
	return time.Date(2026, 8, 14, 11, 12, 0, 0, time.UTC)
}

func TestJournalDraftHandler_HandleStatus(t *testing.T) {
	for _, available := range []bool{true, false} {
		client := &fakeJournalDraftClient{available: available}
		h := NewJournalDraftHandler(&fakeDownloader{}, client, "/vault", fixedNow)

		w := httptest.NewRecorder()
		h.HandleStatus(w, httptest.NewRequest(http.MethodGet, "/api/journal/status", nil))

		var body map[string]bool
		json.NewDecoder(w.Body).Decode(&body) //nolint:errcheck
		if body["available"] != available {
			t.Errorf("available = %v, want %v", body["available"], available)
		}
	}
}

func TestJournalDraftHandler_HandleDraft_Success(t *testing.T) {
	notePath := "/vault/periodic/daily/2026-08-14-W33-Fri.md"
	downloader := &fakeDownloader{files: map[string][]byte{
		notePath: []byte("## 📌 etc.\n\n- "),
	}}
	client := &fakeJournalDraftClient{result: &journaldraft.Result{
		Heading:           "## 📌 etc.",
		InsertionMarkdown: "- `11:12`: noted",
		Reply:             "Noted it under etc.",
	}}
	h := NewJournalDraftHandler(downloader, client, "/vault", fixedNow)

	body, _ := json.Marshal(map[string]string{"userText": "Great sprint retro today."}) //nolint:errcheck
	w := httptest.NewRecorder()
	h.HandleDraft(w, httptest.NewRequest(http.MethodPost, "/api/journal/draft", bytes.NewReader(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var result journaldraft.Result
	json.NewDecoder(w.Body).Decode(&result) //nolint:errcheck
	if result.Heading != "## 📌 etc." || result.Reply != "Noted it under etc." {
		t.Errorf("result = %+v", result)
	}
	if client.gotText != "Great sprint retro today." {
		t.Errorf("gotText = %q", client.gotText)
	}
	if client.gotNote != "## 📌 etc.\n\n- " {
		t.Errorf("gotNote = %q", client.gotNote)
	}
	if client.gotNowISO != fixedNow().Format(time.RFC3339) {
		t.Errorf("gotNowISO = %q", client.gotNowISO)
	}
}

func TestJournalDraftHandler_HandleDraft_MissingUserText(t *testing.T) {
	h := NewJournalDraftHandler(&fakeDownloader{}, &fakeJournalDraftClient{}, "/vault", fixedNow)

	body, _ := json.Marshal(map[string]string{"userText": "  "}) //nolint:errcheck
	w := httptest.NewRecorder()
	h.HandleDraft(w, httptest.NewRequest(http.MethodPost, "/api/journal/draft", bytes.NewReader(body)))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestJournalDraftHandler_HandleDraft_DownloadFails(t *testing.T) {
	downloader := &fakeDownloader{err: errors.New("dropbox down")}
	h := NewJournalDraftHandler(downloader, &fakeJournalDraftClient{}, "/vault", fixedNow)

	body, _ := json.Marshal(map[string]string{"userText": "hello"}) //nolint:errcheck
	w := httptest.NewRecorder()
	h.HandleDraft(w, httptest.NewRequest(http.MethodPost, "/api/journal/draft", bytes.NewReader(body)))

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestJournalDraftHandler_HandleDraft_ClientFails(t *testing.T) {
	notePath := "/vault/periodic/daily/2026-08-14-W33-Fri.md"
	downloader := &fakeDownloader{files: map[string][]byte{notePath: []byte("note")}}
	client := &fakeJournalDraftClient{err: errors.New("agent error")}
	h := NewJournalDraftHandler(downloader, client, "/vault", fixedNow)

	body, _ := json.Marshal(map[string]string{"userText": "hello"}) //nolint:errcheck
	w := httptest.NewRecorder()
	h.HandleDraft(w, httptest.NewRequest(http.MethodPost, "/api/journal/draft", bytes.NewReader(body)))

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}
