package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jonwhittlestone/tools-randoread/internal/journal"
	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
	"github.com/jonwhittlestone/tools-randoread/internal/note"
)

// JournalDropbox is the subset of *dropbox.Client JournalApplyHandler needs
// — read the current note (avoids clobbering same-day edits made between
// draft and apply), then write the spliced result back, plus (for a photo
// attachment) an upload of the image itself.
type JournalDropbox interface {
	Download(path string) ([]byte, error)
	Upload(path string, data []byte) error
}

// journalImageMaxBytes bounds the upload — generous for a phone photo
// (even an uncompressed HEIC rarely exceeds a few MB) without accepting
// something absurd. Enforced via http.MaxBytesReader before multipart
// parsing even starts, so an oversized body is rejected before it's fully
// read into memory.
const journalImageMaxBytes = 20 << 20 // 20MB

// journalImageExtensions maps a sniffed content type (http.DetectContentType)
// to the file extension the upload is stored under — deliberately a
// small allowlist, not "whatever extension the browser's filename had":
// the actual bytes are what get trusted, not client-supplied metadata.
var journalImageExtensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"image/heic": ".heic",
	"image/heif": ".heic",
}

// JournalApplyHandler serves the floating "Send to oh-two" journal input's
// confirm step — POST /api/journal/apply. This is the only place in the
// journal-draft flow that touches Dropbox; NanoClaw (JournalDraftHandler's
// client) never gets vault credentials, and never sees an attached photo
// either — see main.md §05.05's architecture decision on why the write
// stays here, following HandleSaveRelated's existing precedent
// (watching_notes.go). A photo is uploaded here, at confirm time, not at
// draft time — so clicking "Don't update and close" after attaching one
// never leaves an orphaned file in the vault; nothing is written to
// Dropbox at all until OK is actually pressed.
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

// randomHex returns n random hex characters — collision-avoidance for the
// image filename, not a security token, so crypto/rand is used purely for
// its non-repeating output, not for any secrecy property.
func randomHex(n int) string {
	buf := make([]byte, (n+1)/2)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failing is effectively unheard of on any real
		// system; fall back to a fixed suffix rather than erroring the
		// whole request over a filename uniqueness nicety.
		return strings.Repeat("0", n)
	}
	return hex.EncodeToString(buf)[:n]
}

// journalImageFilename generates a unique, timestamp-sortable filename for
// a photo attached at now, matching the vault's existing timestamp-prefixed
// asset naming (see assets/ for precedent) — journal- prefixed and
// randoread/-scoped specifically so these are identifiable as coming from
// this feature, distinct from screenshots/scans dropped in assets/ by hand
// or by other tools.
func journalImageFilename(now time.Time, ext string) string {
	return fmt.Sprintf("journal-%s-%s%s", now.Format("20060102-150405"), randomHex(4), ext)
}

// sniffImageExtension reads (without exhausting) enough of the upload to
// identify its actual format via magic bytes — the file's real content,
// not its client-supplied filename/Content-Type, which is what's trusted
// to determine both validity and the stored extension. Returns ok=false
// for anything not in journalImageExtensions.
func sniffImageExtension(f io.Reader) (ext string, data []byte, ok bool) {
	data, err := io.ReadAll(f)
	if err != nil {
		return "", nil, false
	}
	sniffLen := 512
	if len(data) < sniffLen {
		sniffLen = len(data)
	}
	contentType := http.DetectContentType(data[:sniffLen])
	// DetectContentType can append "; charset=..." for text types — not
	// relevant here since none of journalImageExtensions' keys have one,
	// but split defensively rather than assume an exact match always.
	contentType = strings.SplitN(contentType, ";", 2)[0]
	ext, ok = journalImageExtensions[contentType]
	return ext, data, ok
}

// HandleApply serves POST /api/journal/apply (multipart/form-data — heading,
// insertionMarkdown, nowIso as fields, plus an optional "image" file part).
// Re-downloads today's note rather than trusting a client-supplied copy, so
// an edit made elsewhere between the draft and apply steps isn't silently
// overwritten.
func (h *JournalApplyHandler) HandleApply(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, journalImageMaxBytes)
	if err := r.ParseMultipartForm(journalImageMaxBytes); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid form data (or image too large)")
		return
	}

	heading := strings.TrimSpace(r.FormValue("heading"))
	insertionMarkdown := strings.TrimSpace(r.FormValue("insertionMarkdown"))
	if heading == "" || insertionMarkdown == "" {
		writeJSONError(w, http.StatusBadRequest, "missing heading or insertionMarkdown")
		return
	}

	now := h.resolveNow(r.FormValue("nowIso"))

	if file, _, err := r.FormFile("image"); err == nil {
		defer file.Close() //nolint:errcheck

		ext, data, ok := sniffImageExtension(file)
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "attached file is not a supported image type")
			return
		}

		imagePath := h.VaultRoot + "/assets/randoread/" + journalImageFilename(now, ext)
		if err := h.Dropbox.Upload(imagePath, data); err != nil {
			writeJSONError(w, http.StatusBadGateway, "failed to upload the photo")
			return
		}

		// Path-qualified (not a bare filename) so this resolves to exactly
		// the file just uploaded, not any other same-named file elsewhere
		// in the vault — see vaultFileResolver's doc comment on why a bare
		// "![[name]]" can be ambiguous.
		embedRef := strings.TrimPrefix(imagePath, h.VaultRoot+"/")
		insertionMarkdown += "\n\t![[" + embedRef + "]]"
	}

	path := h.dailyNotePath(now)
	raw, err := h.Dropbox.Download(path)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch today's daily note")
		return
	}

	updated, err := journal.InsertUnderHeading(string(raw), heading, insertionMarkdown)
	if err != nil {
		// Most likely cause: the note's headings didn't match what
		// NanoClaw was shown at draft time (edited concurrently, or a
		// stale draft response after midnight rolled the note over).
		// Note: any photo above has already been uploaded to Dropbox by
		// this point — orphaned, but not silently lost — rather than
		// re-reading the note before uploading, which would cost every
		// successful apply an extra round trip to save one unlikely-path
		// orphan.
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
