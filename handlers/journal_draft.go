package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/journaldraft"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// JournalDraftClient is the subset of *journaldraft.Client JournalDraftHandler
// needs — an interface so tests can fake it without a real NanoClaw instance.
type JournalDraftClient interface {
	Available() bool
	Draft(ctx context.Context, dailyNoteRaw, userText, nowISO string) (*journaldraft.Result, error)
}

// JournalDraftHandler serves the floating "Send to oh-two" journal input's
// propose step (main-randoread.md 05.05) — GET /api/journal/status and
// POST /api/journal/draft. It never writes to Dropbox; see
// JournalApplyHandler for the write side, kept separate so a draft can be
// discarded ("Don't update and close") with no side effect at all.
type JournalDraftHandler struct {
	Downloader NoteDownloader
	Client     JournalDraftClient
	VaultRoot  string

	// Now is overridable for tests; defaults to time.Now. Used both to
	// resolve today's daily-note filename and to timestamp the fragment
	// with the caller's wall-clock time (see journaldraft.Client.Draft's
	// doc comment on why it must be the caller's clock, not NanoClaw's).
	Now func() time.Time
}

// NewJournalDraftHandler builds a JournalDraftHandler. now defaults to
// time.Now if nil.
func NewJournalDraftHandler(downloader NoteDownloader, client JournalDraftClient, vaultRoot string, now func() time.Time) *JournalDraftHandler {
	if now == nil {
		now = time.Now
	}
	return &JournalDraftHandler{Downloader: downloader, Client: client, VaultRoot: vaultRoot, Now: now}
}

func (h *JournalDraftHandler) dailyNotePath() string {
	return h.VaultRoot + "/periodic/daily/" + note.DailyFilename(h.Now())
}

// HandleStatus serves GET /api/journal/status — backs the frontend's
// availability gate ("if doylestone02 isn't on tailscale, the feature
// should be unavailable," main.md §05.05). Always 200; availability is in
// the body, not the status code, since "NanoClaw is unreachable" is an
// expected, not exceptional, response for this endpoint.
func (h *JournalDraftHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]bool{"available": h.Client.Available()})
}

type journalDraftRequest struct {
	UserText string `json:"userText"`
}

// HandleDraft serves POST /api/journal/draft. Read-only against Dropbox
// (Download only) — nothing is written until HandleApply.
func (h *JournalDraftHandler) HandleDraft(w http.ResponseWriter, r *http.Request) {
	var req journalDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.UserText) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing userText")
		return
	}

	raw, err := h.Downloader.Download(h.dailyNotePath())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch today's daily note")
		return
	}

	result, err := h.Client.Draft(r.Context(), string(raw), req.UserText, h.Now().Format(time.RFC3339))
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to draft journal entry")
		return
	}

	writeJSON(w, result)
}
