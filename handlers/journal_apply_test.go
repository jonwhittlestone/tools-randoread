package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePNGSignature is just enough for http.DetectContentType to recognize
// it as image/png (the WHATWG sniffing algorithm only checks the leading
// magic bytes, not a structurally valid image) — no real image content
// needed for these tests.
var fakePNGSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x00}

// newApplyRequest builds a multipart/form-data request matching what
// journal.js actually sends — HandleApply no longer accepts JSON, see its
// doc comment on why a photo is uploaded here rather than at draft time.
// imageBytes == nil omits the "image" part entirely.
func newApplyRequest(t *testing.T, fields map[string]string, imageBytes []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatalf("WriteField(%q): %v", k, err)
		}
	}
	if imageBytes != nil {
		part, err := writer.CreateFormFile("image", "photo.png")
		if err != nil {
			t.Fatalf("CreateFormFile: %v", err)
		}
		if _, err := part.Write(imageBytes); err != nil {
			t.Fatalf("write image bytes: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/journal/apply", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestJournalApplyHandler_Success(t *testing.T) {
	vaultRoot := "/vault"
	notePath := vaultRoot + "/periodic/daily/2026-08-14-W33-Fri.md"

	dbx := newFakeNotesDropbox(vaultRoot)
	dbx.files[notePath] = []byte("## 📌 etc.\n\n- \n")

	h := NewJournalApplyHandler(dbx, &fakeLister{}, vaultRoot, fixedNow)

	req := newApplyRequest(t, map[string]string{
		"heading":           "## 📌 etc.",
		"insertionMarkdown": "- `11:12`: noted",
	}, nil)
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if !strings.Contains(resp["raw"], "- `11:12`: noted") {
		t.Errorf("raw does not contain the inserted line: %q", resp["raw"])
	}
	if resp["path"] != notePath {
		t.Errorf("path = %q, want %q", resp["path"], notePath)
	}

	if len(dbx.uploadCalls) != 1 {
		t.Fatalf("uploadCalls = %d, want 1", len(dbx.uploadCalls))
	}
	if dbx.uploadCalls[0].path != notePath {
		t.Errorf("uploaded path = %q, want %q", dbx.uploadCalls[0].path, notePath)
	}
	if !strings.Contains(string(dbx.uploadCalls[0].data), "- `11:12`: noted") {
		t.Errorf("uploaded content missing the inserted line: %q", dbx.uploadCalls[0].data)
	}
}

func TestJournalApplyHandler_WithImage_UploadsPhotoAndAppendsEmbed(t *testing.T) {
	vaultRoot := "/vault"
	notePath := vaultRoot + "/periodic/daily/2026-08-14-W33-Fri.md"

	dbx := newFakeNotesDropbox(vaultRoot)
	dbx.files[notePath] = []byte("## 📌 etc.\n\n- \n")

	h := NewJournalApplyHandler(dbx, &fakeLister{}, vaultRoot, fixedNow)

	req := newApplyRequest(t, map[string]string{
		"heading":           "## 📌 etc.",
		"insertionMarkdown": "- `11:12`: a photo from the walk",
	}, fakePNGSignature)
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	if len(dbx.uploadCalls) != 2 {
		t.Fatalf("uploadCalls = %d, want 2 (image + note)", len(dbx.uploadCalls))
	}

	imageUpload := dbx.uploadCalls[0]
	if !strings.HasPrefix(imageUpload.path, vaultRoot+"/assets/randoread/journal-") {
		t.Errorf("image path = %q, want it under %s/assets/randoread/journal-", imageUpload.path, vaultRoot)
	}
	if !strings.HasSuffix(imageUpload.path, ".png") {
		t.Errorf("image path = %q, want a .png extension sniffed from the PNG signature", imageUpload.path)
	}
	if !bytes.Equal(imageUpload.data, fakePNGSignature) {
		t.Errorf("uploaded image bytes don't match what was sent")
	}

	noteUpload := dbx.uploadCalls[1]
	if noteUpload.path != notePath {
		t.Errorf("note uploaded to %q, want %q", noteUpload.path, notePath)
	}
	embedRef := strings.TrimPrefix(imageUpload.path, vaultRoot+"/")
	wantEmbed := "- `11:12`: a photo from the walk\n\t![[" + embedRef + "]]"
	if !strings.Contains(string(noteUpload.data), wantEmbed) {
		t.Errorf("note does not contain the expected embed line, got:\n%s\nwant substring:\n%s", noteUpload.data, wantEmbed)
	}
}

func TestJournalApplyHandler_RejectsNonImageUpload(t *testing.T) {
	vaultRoot := "/vault"
	dbx := newFakeNotesDropbox(vaultRoot)
	h := NewJournalApplyHandler(dbx, &fakeLister{}, vaultRoot, fixedNow)

	req := newApplyRequest(t, map[string]string{
		"heading":           "## 📌 etc.",
		"insertionMarkdown": "- `11:12`: not actually a photo",
	}, []byte("just some plain text, not an image"))
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if len(dbx.uploadCalls) != 0 {
		t.Error("nothing should be uploaded when the attached file isn't a recognized image type")
	}
}

func TestJournalApplyHandler_UsesBrowserSuppliedNowIso(t *testing.T) {
	// Regression test — see the matching draft-side test's comment. Apply
	// must target the date the *browser* says it is, not the server
	// container's own clock, and specifically the nowIso the caller sends
	// (which the frontend carries over from the matching draft call) —
	// here that's a different calendar day than fixedNow's UTC value, to
	// prove it's actually driving which file gets written.
	vaultRoot := "/vault"
	bstPath := vaultRoot + "/periodic/daily/2026-08-15-W33-Sat.md"

	dbx := newFakeNotesDropbox(vaultRoot)
	dbx.files[bstPath] = []byte("## 📌 etc.\n\n- \n")

	h := NewJournalApplyHandler(dbx, &fakeLister{}, vaultRoot, fixedNow)

	req := newApplyRequest(t, map[string]string{
		"heading":           "## 📌 etc.",
		"insertionMarkdown": "- `00:20`: noted",
		"nowIso":            "2026-08-15T00:20:00+01:00", // just past midnight BST
	}, nil)
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if len(dbx.uploadCalls) != 1 || dbx.uploadCalls[0].path != bstPath {
		t.Fatalf("uploaded to %v, want exactly one upload to %q", dbx.uploadCalls, bstPath)
	}
}

func TestJournalApplyHandler_MissingFields(t *testing.T) {
	h := NewJournalApplyHandler(newFakeNotesDropbox("/vault"), &fakeLister{}, "/vault", fixedNow)

	req := newApplyRequest(t, map[string]string{"heading": "## 📌 etc."}, nil)
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestJournalApplyHandler_DownloadFails(t *testing.T) {
	dbx := newFakeNotesDropbox("/vault")
	dbx.downloadErr = errUploadFailedSentinel
	h := NewJournalApplyHandler(dbx, &fakeLister{}, "/vault", fixedNow)

	req := newApplyRequest(t, map[string]string{
		"heading": "## 📌 etc.", "insertionMarkdown": "- x",
	}, nil)
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

func TestJournalApplyHandler_HeadingNotFound(t *testing.T) {
	vaultRoot := "/vault"
	notePath := vaultRoot + "/periodic/daily/2026-08-14-W33-Fri.md"
	dbx := newFakeNotesDropbox(vaultRoot)
	dbx.files[notePath] = []byte("## 🥇Small Victories\n\n...\n")

	h := NewJournalApplyHandler(dbx, &fakeLister{}, vaultRoot, fixedNow)

	req := newApplyRequest(t, map[string]string{
		"heading": "## 📌 etc.", "insertionMarkdown": "- x",
	}, nil)
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
	if len(dbx.uploadCalls) != 0 {
		t.Error("Upload should not be called when the heading isn't found")
	}
}

func TestJournalApplyHandler_UploadFails(t *testing.T) {
	vaultRoot := "/vault"
	notePath := vaultRoot + "/periodic/daily/2026-08-14-W33-Fri.md"
	dbx := newFakeNotesDropbox(vaultRoot)
	dbx.files[notePath] = []byte("## 📌 etc.\n\n- \n")
	dbx.uploadErr = errUploadFailedSentinel

	h := NewJournalApplyHandler(dbx, &fakeLister{}, vaultRoot, fixedNow)

	req := newApplyRequest(t, map[string]string{
		"heading": "## 📌 etc.", "insertionMarkdown": "- x",
	}, nil)
	w := httptest.NewRecorder()
	h.HandleApply(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

var errUploadFailedSentinel = &journalApplyTestError{"simulated failure"}

type journalApplyTestError struct{ msg string }

func (e *journalApplyTestError) Error() string { return e.msg }
