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

	// Now is a fallback only — used when the request omits nowIso (older
	// client, or a direct API call) or sends something unparseable. The
	// normal path uses the browser-supplied nowIso for both the timestamp
	// shown in the drafted line and for resolving "today's" filename: the
	// server container's own clock isn't reliably in the user's timezone
	// (this is exactly the bug that shipped first — server-side time.Now()
	// produced a UTC timestamp a real BST user saw as an hour off), and
	// only the browser actually knows the user's real local time.
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

// resolveNow parses the browser-supplied nowIso (RFC3339, with the
// browser's own local UTC offset — see static/journal.js's localIso()),
// falling back to h.Now() if it's missing or fails to parse.
func (h *JournalDraftHandler) resolveNow(nowIso string) time.Time {
	if nowIso != "" {
		if t, err := time.Parse(time.RFC3339, nowIso); err == nil {
			return t
		}
	}
	return h.Now()
}

func (h *JournalDraftHandler) dailyNotePath(now time.Time) string {
	return h.VaultRoot + "/periodic/daily/" + note.DailyFilename(now)
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
	// NowISO is the browser's own local time (RFC3339, with its real UTC
	// offset) at the moment "Send to oh-two" was clicked — see
	// resolveNow's doc comment.
	NowISO string `json:"nowIso"`
}

// HandleDraft serves POST /api/journal/draft. Read-only against Dropbox
// (Download only) — nothing is written until HandleApply.
func (h *JournalDraftHandler) HandleDraft(w http.ResponseWriter, r *http.Request) {
	var req journalDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.UserText) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing userText")
		return
	}

	now := h.resolveNow(req.NowISO)

	raw, err := h.Downloader.Download(h.dailyNotePath(now))
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch today's daily note")
		return
	}

	result, err := h.Client.Draft(r.Context(), string(raw), req.UserText, now.Format(time.RFC3339))
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to draft journal entry")
		return
	}

	writeJSON(w, result)
}
