package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJournalApplyHandler_Success(t *testing.T) {
	vaultRoot := "/vault"
	notePath := vaultRoot + "/periodic/daily/2026-08-14-W33-Fri.md"

	dbx := newFakeNotesDropbox(vaultRoot)
	dbx.files[notePath] = []byte("## 📌 etc.\n\n- \n")

	h := NewJournalApplyHandler(dbx, &fakeLister{}, vaultRoot, fixedNow)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"heading":           "## 📌 etc.",
		"insertionMarkdown": "- `11:12`: noted",
	})
	w := httptest.NewRecorder()
	h.HandleApply(w, httptest.NewRequest(http.MethodPost, "/api/journal/apply", bytes.NewReader(body)))

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

func TestJournalApplyHandler_MissingFields(t *testing.T) {
	h := NewJournalApplyHandler(newFakeNotesDropbox("/vault"), &fakeLister{}, "/vault", fixedNow)

	body, _ := json.Marshal(map[string]string{"heading": "## 📌 etc."}) //nolint:errcheck
	w := httptest.NewRecorder()
	h.HandleApply(w, httptest.NewRequest(http.MethodPost, "/api/journal/apply", bytes.NewReader(body)))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestJournalApplyHandler_DownloadFails(t *testing.T) {
	dbx := newFakeNotesDropbox("/vault")
	dbx.downloadErr = errUploadFailedSentinel
	h := NewJournalApplyHandler(dbx, &fakeLister{}, "/vault", fixedNow)

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"heading": "## 📌 etc.", "insertionMarkdown": "- x",
	})
	w := httptest.NewRecorder()
	h.HandleApply(w, httptest.NewRequest(http.MethodPost, "/api/journal/apply", bytes.NewReader(body)))

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

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"heading": "## 📌 etc.", "insertionMarkdown": "- x",
	})
	w := httptest.NewRecorder()
	h.HandleApply(w, httptest.NewRequest(http.MethodPost, "/api/journal/apply", bytes.NewReader(body)))

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

	body, _ := json.Marshal(map[string]string{ //nolint:errcheck
		"heading": "## 📌 etc.", "insertionMarkdown": "- x",
	})
	w := httptest.NewRecorder()
	h.HandleApply(w, httptest.NewRequest(http.MethodPost, "/api/journal/apply", bytes.NewReader(body)))

	if w.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", w.Code)
	}
}

var errUploadFailedSentinel = &journalApplyTestError{"simulated failure"}

type journalApplyTestError struct{ msg string }

func (e *journalApplyTestError) Error() string { return e.msg }
