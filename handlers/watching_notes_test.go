package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/watchitlater"
)

const testTemplate = `> online link
## vault references
- [directory1-from-vault-relative / directory 2 / note-title](template/...)


---

...
`

// fakeNotesDropbox is a tiny in-memory stand-in for *dropbox.Client, used as
// both WatchingNotesHandler.Dropbox and .VaultLister (ListFolder's signature
// is identical either way).
type fakeNotesDropbox struct {
	files       map[string][]byte
	listErr     error
	downloadErr error
	uploadErr   error
	uploadCalls []struct {
		path string
		data []byte
	}
}

func newFakeNotesDropbox(vaultRoot string) *fakeNotesDropbox {
	f := &fakeNotesDropbox{files: map[string][]byte{}}
	f.files[vaultRoot+"/templates/video-notes.md"] = []byte(testTemplate)
	return f
}

func (f *fakeNotesDropbox) Download(path string) ([]byte, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	data, ok := f.files[path]
	if !ok {
		return nil, errors.New("not found: " + path)
	}
	return data, nil
}

func (f *fakeNotesDropbox) Upload(path string, data []byte) error {
	if f.uploadErr != nil {
		return f.uploadErr
	}
	f.files[path] = data
	f.uploadCalls = append(f.uploadCalls, struct {
		path string
		data []byte
	}{path, data})
	return nil
}

func (f *fakeNotesDropbox) ListFolder(path string, recursive bool) ([]dropbox.Entry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	var entries []dropbox.Entry
	for p := range f.files {
		if !strings.HasPrefix(p, path+"/") {
			continue
		}
		rest := strings.TrimPrefix(p, path+"/")
		if !recursive && strings.Contains(rest, "/") {
			continue
		}
		name := p[strings.LastIndex(p, "/")+1:]
		entries = append(entries, dropbox.Entry{Path: p, Name: name, Size: int64(len(f.files[p])) + 1})
	}
	return entries, nil
}

const testVaultRoot = "/DropsyncFiles/jw-mind"

func stagedRecord() *watchitlater.Record {
	return &watchitlater.Record{
		Staged:       true,
		VideoID:      "vid1",
		Title:        "Malcolm Gladwell: Talking to Strangers",
		YoutubeURL:   "https://www.youtube.com/watch?v=Hgr1Wv8mwh8",
		PlaylistRank: 80,
		StagedAt:     "2026-08-01T16:00:00Z",
	}
}

func newTestNotesHandler(client WatchitlaterClient, dbx *fakeNotesDropbox) *WatchingNotesHandler {
	h := NewWatchingNotesHandler(client, dbx, dbx, testVaultRoot)
	h.Now = func() time.Time { return time.Date(2026, 8, 1, 16, 0, 0, 0, time.UTC) }
	return h
}

func decodeNoteResponse(t *testing.T, rec *httptest.ResponseRecorder) noteResponse {
	t.Helper()
	var resp noteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, rec.Body.String())
	}
	return resp
}

func TestHandleGetNote_ReturnsDraftWhenNoneSaved(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	h := newTestNotesHandler(f, dbx)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/note", nil)
	rec := httptest.NewRecorder()
	h.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeNoteResponse(t, rec)
	if resp.Exists {
		t.Errorf("expected exists=false for a fresh draft")
	}
	if !strings.Contains(resp.Raw, "https://www.youtube.com/watch?v=Hgr1Wv8mwh8") {
		t.Errorf("expected draft to contain the YouTube URL, got %q", resp.Raw)
	}
	if len(dbx.uploadCalls) != 0 {
		t.Errorf("GET must never write to Dropbox, got uploads: %+v", dbx.uploadCalls)
	}
}

func TestHandleGetNote_ReturnsExistingNote(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	existingPath := testVaultRoot + "/Clippings/randoread-watching-it-later/2026-07-15-80-malcolm-gladwell-talking-to-strangers.md"
	dbx.files[existingPath] = []byte("> https://www.youtube.com/watch?v=Hgr1Wv8mwh8\n## vault references\n- nothing\n\n\n---\n\nmy real notes\n")
	h := newTestNotesHandler(f, dbx)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/note", nil)
	rec := httptest.NewRecorder()
	h.HandleGet(rec, req)

	resp := decodeNoteResponse(t, rec)
	if !resp.Exists {
		t.Errorf("expected exists=true for a previously-saved note")
	}
	if resp.Path != existingPath {
		t.Errorf("expected path %q (original date prefix preserved), got %q", existingPath, resp.Path)
	}
	if !strings.Contains(resp.Raw, "my real notes") {
		t.Errorf("expected saved content, got %q", resp.Raw)
	}
}

func TestHandleSaveNote_FirstSaveMintsFilename(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	h := newTestNotesHandler(f, dbx)

	body := `{"content":"> https://www.youtube.com/watch?v=Hgr1Wv8mwh8\n## vault references\n- nothing\n\n\n---\n\nnew content"}`
	req := httptest.NewRequest(http.MethodPost, "/api/watching/note", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleSave(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeNoteResponse(t, rec)
	wantPath := testVaultRoot + "/Clippings/randoread-watching-it-later/2026-08-01-80-malcolm-gladwell-talking-to-strangers.md"
	if resp.Path != wantPath {
		t.Errorf("expected minted path %q, got %q", wantPath, resp.Path)
	}
	if !resp.Exists {
		t.Errorf("expected exists=true after a successful save")
	}
	if _, ok := dbx.files[wantPath]; !ok {
		t.Errorf("expected file to be written to Dropbox at %q", wantPath)
	}
}

func TestHandleSaveNote_SecondSavePreservesOriginalFilename(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	existingPath := testVaultRoot + "/Clippings/randoread-watching-it-later/2026-07-15-80-malcolm-gladwell-talking-to-strangers.md"
	dbx.files[existingPath] = []byte("original content")
	h := newTestNotesHandler(f, dbx)

	body := `{"content":"edited content"}`
	req := httptest.NewRequest(http.MethodPost, "/api/watching/note", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleSave(rec, req)

	resp := decodeNoteResponse(t, rec)
	if resp.Path != existingPath {
		t.Errorf("expected save to overwrite the original path %q, got %q", existingPath, resp.Path)
	}
	if string(dbx.files[existingPath]) != "edited content" {
		t.Errorf("expected file content to be updated, got %q", dbx.files[existingPath])
	}
	if len(dbx.files) != 2 { // template + the one note, no second file minted
		t.Errorf("expected no second file to be created, files: %+v", keysOf(dbx.files))
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestHandleSearchNotes_ScoresBySubsequence(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	dbx.files[testVaultRoot+"/music/piano/jazz-standards-and-progressions.md"] = []byte("x")
	dbx.files[testVaultRoot+"/books/2026/atomic-habits.md"] = []byte("x")
	h := newTestNotesHandler(f, dbx)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/note/search?q=jazz", nil)
	rec := httptest.NewRecorder()
	h.HandleSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Results []noteSearchResult `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Results) != 1 {
		t.Fatalf("expected 1 match for %q, got %+v", "jazz", body.Results)
	}
	if !strings.Contains(body.Results[0].Title, "jazz-standards") {
		t.Errorf("unexpected match: %+v", body.Results[0])
	}
}

func TestHandleAddRelated_AppendsAndSaves(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	relatedPath := testVaultRoot + "/music/piano/jazz-standards-and-progressions.md"
	dbx.files[relatedPath] = []byte("x")
	h := newTestNotesHandler(f, dbx)

	body := `{"path":"` + relatedPath + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/watching/note/related", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleAddRelated(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeNoteResponse(t, rec)
	if len(resp.References) != 1 {
		t.Fatalf("expected 1 reference, got %+v", resp.References)
	}
	if resp.References[0].Path != relatedPath {
		t.Errorf("unexpected reference path: %+v", resp.References[0])
	}
	if !resp.Exists {
		t.Errorf("expected the note to have been saved (exists=true) after adding a reference")
	}
}

func TestHandleRelatedPreview_RendersReadOnly(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	relatedPath := testVaultRoot + "/music/piano/jazz-standards-and-progressions.md"
	dbx.files[relatedPath] = []byte("# Jazz Standards\n\nSome content.")
	h := newTestNotesHandler(f, dbx)

	req := httptest.NewRequest(http.MethodGet, "/api/watching/note/related-preview?path="+relatedPath, nil)
	rec := httptest.NewRecorder()
	h.HandleRelatedPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["html"], "Jazz Standards") {
		t.Errorf("expected rendered preview html, got %q", body["html"])
	}
	// raw/path let the frontend drop a related note into the same Edit flow
	// as the video's own note (main-randoread.md 05.02.02, "edit if
	// necessary") — see HandleSaveRelated.
	if body["raw"] != "# Jazz Standards\n\nSome content." {
		t.Errorf("expected raw markdown for editing, got %q", body["raw"])
	}
	if body["path"] != relatedPath {
		t.Errorf("expected path %q, got %q", relatedPath, body["path"])
	}
}

func TestHandleRelatedPreview_RejectsPathOutsideVault(t *testing.T) {
	f := &fakeWatchitlaterClient{current: stagedRecord()}
	dbx := newFakeNotesDropbox(testVaultRoot)
	h := newTestNotesHandler(f, dbx)

	for _, badPath := range []string{
		"/etc/passwd",
		testVaultRoot + "/../secret.md",
		testVaultRoot + "/notes.txt",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/watching/note/related-preview?path="+badPath, nil)
		rec := httptest.NewRecorder()
		h.HandleRelatedPreview(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q: expected 400, got %d", badPath, rec.Code)
		}
	}
}

func TestHandleSaveRelated_SavesAndRendersAtGivenPath(t *testing.T) {
	// Unlike HandleSave, this doesn't touch the currently staged video's own
	// note at all — no fakeWatchitlaterClient staged record is even needed.
	dbx := newFakeNotesDropbox(testVaultRoot)
	relatedPath := testVaultRoot + "/music/piano/jazz-standards-and-progressions.md"
	dbx.files[relatedPath] = []byte("# Jazz Standards\n\nOld content.")
	h := newTestNotesHandler(nil, dbx)

	body := `{"path":"` + relatedPath + `","content":"# Jazz Standards\n\nEdited content."}`
	req := httptest.NewRequest(http.MethodPost, "/api/watching/note/related-save", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleSaveRelated(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(dbx.files[relatedPath]); got != "# Jazz Standards\n\nEdited content." {
		t.Errorf("expected the related note to be overwritten in place, got %q", got)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["path"] != relatedPath {
		t.Errorf("expected path %q, got %q", relatedPath, resp["path"])
	}
	if resp["raw"] != "# Jazz Standards\n\nEdited content." {
		t.Errorf("expected raw to echo the saved content, got %q", resp["raw"])
	}
	if !strings.Contains(resp["html"], "Edited content") {
		t.Errorf("expected re-rendered html reflecting the edit, got %q", resp["html"])
	}
}

func TestHandleSaveRelated_RejectsPathOutsideVault(t *testing.T) {
	dbx := newFakeNotesDropbox(testVaultRoot)
	h := newTestNotesHandler(nil, dbx)

	for _, badPath := range []string{
		"/etc/passwd",
		testVaultRoot + "/../secret.md",
		testVaultRoot + "/notes.txt",
	} {
		body := `{"path":"` + badPath + `","content":"pwned"}`
		req := httptest.NewRequest(http.MethodPost, "/api/watching/note/related-save", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.HandleSaveRelated(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %q: expected 400, got %d", badPath, rec.Code)
		}
		if _, wrote := dbx.files[badPath]; wrote {
			t.Errorf("path %q: must not have been written to Dropbox", badPath)
		}
	}
}

func TestHandleSaveRelated_RejectsInvalidJSON(t *testing.T) {
	dbx := newFakeNotesDropbox(testVaultRoot)
	h := newTestNotesHandler(nil, dbx)

	req := httptest.NewRequest(http.MethodPost, "/api/watching/note/related-save", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.HandleSaveRelated(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
