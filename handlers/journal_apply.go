package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/journal"
	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// JournalDropbox is the subset of *dropbox.Client JournalApplyHandler needs
// — read the current note (avoids clobbering same-day edits made between
// draft and apply), then write the spliced result back.
type JournalDropbox interface {
	Download(path string) ([]byte, error)
	Upload(path string, data []byte) error
}

// JournalApplyHandler serves the floating "Send to oh-two" journal input's
// confirm step — POST /api/journal/apply. This is the only place in the
// journal-draft flow that touches Dropbox; NanoClaw (JournalDraftHandler's
// client) never gets vault credentials — see main.md §05.05's architecture
// decision on why the write stays here, following HandleSaveRelated's
// existing precedent (watching_notes.go).
type JournalApplyHandler struct {
	Dropbox     JournalDropbox
	VaultLister NoteLister // embed resolution for the re-rendered note, same as DailyHandler
	VaultRoot   string
	AuthToken   string

	// Now is a fallback only, for the same reason as
	// JournalDraftHandler.Now — see that doc comment. The normal path uses
	// the browser-supplied nowIso, and critically the *same* nowIso the
	// matching draft call used (the frontend carries it from draft's
	// request straight into apply's), so a modal left open across midnight
	// can't make apply write to a different day's file than the one draft
	// actually read the heading from.
	Now func() time.Time
}

// NewJournalApplyHandler builds a JournalApplyHandler. now defaults to
// time.Now if nil.
func NewJournalApplyHandler(dbx JournalDropbox, vaultLister NoteLister, vaultRoot string, now func() time.Time) *JournalApplyHandler {
	if now == nil {
		now = time.Now
	}
	return &JournalApplyHandler{Dropbox: dbx, VaultLister: vaultLister, VaultRoot: vaultRoot, Now: now}
}

// resolveNow — see JournalDraftHandler.resolveNow's doc comment.
func (h *JournalApplyHandler) resolveNow(nowIso string) time.Time {
	if nowIso != "" {
		if t, err := time.Parse(time.RFC3339, nowIso); err == nil {
			return t
		}
	}
	return h.Now()
}

func (h *JournalApplyHandler) dailyNotePath(now time.Time) string {
	return h.VaultRoot + "/periodic/daily/" + note.DailyFilename(now)
}

type journalApplyRequest struct {
	Heading           string `json:"heading"`
	InsertionMarkdown string `json:"insertionMarkdown"`
	// NowISO should be the exact value the matching draft request sent —
	// see the Now field's doc comment.
	NowISO string `json:"nowIso"`
}

// HandleApply serves POST /api/journal/apply. Re-downloads today's note
// rather than trusting a client-supplied copy, so an edit made elsewhere
// between the draft and apply steps isn't silently overwritten.
func (h *JournalApplyHandler) HandleApply(w http.ResponseWriter, r *http.Request) {
	var req journalApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		strings.TrimSpace(req.Heading) == "" ||
		strings.TrimSpace(req.InsertionMarkdown) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing heading or insertionMarkdown")
		return
	}

	path := h.dailyNotePath(h.resolveNow(req.NowISO))
	raw, err := h.Dropbox.Download(path)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch today's daily note")
		return
	}

	updated, err := journal.InsertUnderHeading(string(raw), req.Heading, req.InsertionMarkdown)
	if err != nil {
		// Most likely cause: the note's headings didn't match what
		// NanoClaw was shown at draft time (edited concurrently, or a
		// stale draft response after midnight rolled the note over).
		writeJSONError(w, http.StatusUnprocessableEntity, "heading no longer found in today's note — try again")
		return
	}

	if err := h.Dropbox.Upload(path, []byte(updated)); err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to save today's daily note")
		return
	}

	html := markdown.Render([]byte(updated), vaultFileResolver(h.VaultLister, h.VaultRoot, h.AuthToken))
	writeJSON(w, map[string]string{
		"html": html,
		"raw":  updated,
		"path": path,
	})
}
