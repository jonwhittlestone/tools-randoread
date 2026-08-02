package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
	"github.com/jonwhittlestone/tools-randoread/internal/videonotes"
	"github.com/jonwhittlestone/tools-randoread/internal/watchitlater"
)

// NotesDropbox is the subset of *dropbox.Client WatchingNotesHandler needs
// for the (small, dedicated) per-video notes folder: reading/writing a
// single note and listing that one folder to find it.
type NotesDropbox interface {
	Download(path string) ([]byte, error)
	Upload(path string, data []byte) error
	ListFolder(path string, recursive bool) ([]dropbox.Entry, error)
}

// WatchingNotesHandler serves the per-video notes pane under "Watching It
// Later" — see main-randoread.md section 05.02. It never talks to
// tools-watchitlater's database directly; a note's identity is derived
// purely from the currently-staged record (see videonotes.Suffix's doc
// comment for why StagedAt can't be used for this).
type WatchingNotesHandler struct {
	Client WatchitlaterClient

	// Dropbox is used uncached (unlike VaultLister) so a just-written note
	// is immediately visible to the next findExisting lookup — mirrors why
	// ClippingsListHandler (clippings.go) also bypasses the cache.
	Dropbox NotesDropbox

	// VaultLister is the shared, 24h-cached recursive vault listing
	// (dropbox.CachedLister in main.go) — reused for embed resolution
	// inside note bodies and for the related-note fuzzy search, exactly
	// like Rando/Clipped already do. No separate index is built.
	VaultLister NoteLister

	VaultRoot string
	AuthToken string

	// Now is overridable for tests; defaults to time.Now. Only used to mint
	// a fresh filename's date prefix on a note's first-ever save.
	Now func() time.Time
}

// NewWatchingNotesHandler builds a WatchingNotesHandler.
func NewWatchingNotesHandler(client WatchitlaterClient, dbx NotesDropbox, vaultLister NoteLister, vaultRoot string) *WatchingNotesHandler {
	return &WatchingNotesHandler{
		Client:      client,
		Dropbox:     dbx,
		VaultLister: vaultLister,
		VaultRoot:   vaultRoot,
		Now:         time.Now,
	}
}

// noteResponse is returned by every endpoint that has a note's full state to
// report (GET, save, add-related) — the same shape lets the frontend swap
// straight from an autosave response back to the rendered view without an
// extra round trip.
type noteResponse struct {
	HTML       string                 `json:"html"`
	Raw        string                 `json:"raw"`
	Path       string                 `json:"path"`
	Exists     bool                   `json:"exists"`
	References []videonotes.Reference `json:"references"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// findExisting looks for a previously-saved note for record in the (small,
// dedicated) notes folder, matching by videonotes.Suffix(record.Title) —
// deliberately ignoring the date prefix, see that function's doc comment.
//
// Any ListFolder error (including "folder doesn't exist yet," Dropbox's
// normal response before the very first note is ever saved anywhere) is
// treated as "nothing found" rather than a hard failure — a real,
// non-transient problem talking to Dropbox will still surface honestly the
// moment Download/Upload is attempted next.
func (h *WatchingNotesHandler) findExisting(record *watchitlater.Record) (path string, found bool) {
	dir := videonotes.Dir(h.VaultRoot)
	entries, err := h.Dropbox.ListFolder(dir, false)
	if err != nil {
		return "", false
	}
	suffix := videonotes.Suffix(record.Title)
	for _, e := range entries {
		if !e.IsFolder && strings.HasSuffix(e.Name, suffix) {
			return e.Path, true
		}
	}
	return "", false
}

// loadOrDraft returns the current state of record's note: the previously
// saved content if one exists, or a template-applied draft (never written)
// otherwise.
func (h *WatchingNotesHandler) loadOrDraft(record *watchitlater.Record) (noteResponse, error) {
	path, found := h.findExisting(record)

	var raw string
	if found {
		data, err := h.Dropbox.Download(path)
		if err != nil {
			return noteResponse{}, err
		}
		raw = string(data)
	} else {
		tmpl, err := h.Dropbox.Download(h.VaultRoot + "/templates/video-notes.md")
		if err != nil {
			return noteResponse{}, err
		}
		raw = videonotes.ApplyTemplate(string(tmpl), record.YoutubeURL)
		path = ""
	}

	return h.render(raw, path, found), nil
}

// save writes content for record — overwriting its existing note if one was
// found, or minting a brand-new filename on the first-ever save (see
// videonotes.Filename).
func (h *WatchingNotesHandler) save(record *watchitlater.Record, content string) (noteResponse, error) {
	path, found := h.findExisting(record)
	if !found {
		path = videonotes.Dir(h.VaultRoot) + "/" + videonotes.Filename(h.Now(), record.PlaylistRank, record.Title)
	}
	if err := h.Dropbox.Upload(path, []byte(content)); err != nil {
		return noteResponse{}, err
	}
	return h.render(content, path, true), nil
}

func (h *WatchingNotesHandler) render(raw, path string, exists bool) noteResponse {
	html := markdown.Render([]byte(raw), vaultFileResolver(h.VaultLister, h.VaultRoot, h.AuthToken))
	return noteResponse{
		HTML:       html,
		Raw:        raw,
		Path:       path,
		Exists:     exists,
		References: videonotes.ParseReferences(raw),
	}
}

// currentStagedRecord fetches the currently staged watchitlater record,
// writing a JSON error response and returning ok=false if there isn't one —
// every notes endpoint operates on whatever's currently staged, never a
// client-supplied video ID (see the handler's doc comment).
func (h *WatchingNotesHandler) currentStagedRecord(w http.ResponseWriter) (*watchitlater.Record, bool) {
	record, err := h.Client.Current()
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to reach watchitlater")
		return nil, false
	}
	if !record.Staged {
		writeJSONError(w, http.StatusBadRequest, "no video currently staged")
		return nil, false
	}
	return record, true
}

// HandleGet serves GET /api/watching/note.
func (h *WatchingNotesHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	record, ok := h.currentStagedRecord(w)
	if !ok {
		return
	}
	resp, err := h.loadOrDraft(record)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to load note")
		return
	}
	writeJSON(w, resp)
}

// HandleSave serves POST /api/watching/note — called by the frontend's
// debounced autosave, not a manual Save click (see the feature plan).
func (h *WatchingNotesHandler) HandleSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	record, ok := h.currentStagedRecord(w)
	if !ok {
		return
	}
	resp, err := h.save(record, body.Content)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to save note")
		return
	}
	writeJSON(w, resp)
}

// HandleAddRelated serves POST /api/watching/note/related — "Linking to
// existing note" (see main-randoread.md 05.02).
func (h *WatchingNotesHandler) HandleAddRelated(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Path == "" {
		writeJSONError(w, http.StatusBadRequest, "missing path")
		return
	}

	record, ok := h.currentStagedRecord(w)
	if !ok {
		return
	}

	current, err := h.loadOrDraft(record)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to load note")
		return
	}

	title := note.FormatVaultTitle(body.Path, h.VaultRoot)
	newRaw := videonotes.AppendReference(current.Raw, videonotes.Reference{Title: title, Path: body.Path})

	resp, err := h.save(record, newRaw)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to save note")
		return
	}
	writeJSON(w, resp)
}

// noteSearchResult is one hit from HandleSearch.
type noteSearchResult struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

const maxSearchResults = 15

// HandleSearch serves GET /api/watching/note/search?q= — the fuzzy-find
// backing "Linking to existing note". Scores VaultLister's already-cached
// recursive listing in memory; no index, no extra Dropbox call (see the
// feature plan's design decision on this).
func (h *WatchingNotesHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, map[string]any{"results": []noteSearchResult{}})
		return
	}

	entries, err := h.VaultLister.ListFolder(h.VaultRoot, true)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to search vault notes")
		return
	}

	results := searchNotes(candidateNotes(entries, h.VaultRoot), q, h.VaultRoot)
	writeJSON(w, map[string]any{"results": results})
}

// searchNotes scores candidates' titles against query by subsequence match
// (every character of query must appear, in order, somewhere in the title —
// not necessarily contiguously), ranking a literal substring match highest
// (rewarding an earlier match position) and otherwise rewarding a tighter
// subsequence span, capped to maxSearchResults.
func searchNotes(candidates []dropbox.Entry, query, vaultRoot string) []noteSearchResult {
	q := strings.ToLower(query)

	type scored struct {
		result noteSearchResult
		score  int
	}
	var matches []scored
	for _, e := range candidates {
		title := note.FormatVaultTitle(e.Path, vaultRoot)
		if score, ok := subsequenceScore(strings.ToLower(title), q); ok {
			matches = append(matches, scored{result: noteSearchResult{Title: title, Path: e.Path}, score: score})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	if len(matches) > maxSearchResults {
		matches = matches[:maxSearchResults]
	}

	out := make([]noteSearchResult, len(matches))
	for i, m := range matches {
		out[i] = m.result
	}
	return out
}

func subsequenceScore(title, q string) (int, bool) {
	if idx := strings.Index(title, q); idx != -1 {
		return 1000 - idx, true
	}

	ti, qi, start := 0, 0, -1
	for ti < len(title) && qi < len(q) {
		if title[ti] == q[qi] {
			if start == -1 {
				start = ti
			}
			qi++
		}
		ti++
	}
	if qi < len(q) {
		return 0, false
	}
	return 500 - (ti - start), true
}

// HandleRelatedPreview serves GET /api/watching/note/related-preview?path= —
// a read-only inline preview for a related note, so "view those related
// notes too" doesn't need a whole second navigation mode (see the feature
// plan). path must be an in-vault .md file.
func (h *WatchingNotesHandler) HandleRelatedPreview(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if !isValidVaultNotePath(path, h.VaultRoot) {
		writeJSONError(w, http.StatusBadRequest, "invalid path")
		return
	}

	data, err := h.Dropbox.Download(path)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to load note")
		return
	}

	html := markdown.Render(data, vaultFileResolver(h.VaultLister, h.VaultRoot, h.AuthToken))
	writeJSON(w, map[string]string{"html": html, "title": note.FormatVaultTitle(path, h.VaultRoot)})
}

func isValidVaultNotePath(path, vaultRoot string) bool {
	return path != "" &&
		strings.HasPrefix(path, vaultRoot+"/") &&
		strings.HasSuffix(path, ".md") &&
		!strings.Contains(path, "..")
}
