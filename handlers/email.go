package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
)

// SendFunc delivers an already-composed email — bound to internal/mail.Send
// with SMTP config and the recipient baked in, so handlers don't need to
// know about SMTP at all.
type SendFunc func(subject, htmlBody string) error

// EmailHandler emails the currently displayed note as an HTML-embedded
// message. It re-downloads and re-renders the note server-side (rather than
// trusting client-supplied HTML) so the email always reflects the real
// vault content, and so a Rando/Clipped note can be emailed without
// re-picking a new one or consuming its cooldown.
type EmailHandler struct {
	Downloader    NoteDownloader
	VaultRoot     string
	PublicBaseURL string // e.g. "https://howapped.zapto.org/randoread/"
	AuthToken     string // embedded in asset URLs so the email client can fetch images
	Send          SendFunc
}

// NewEmailHandler builds an EmailHandler.
func NewEmailHandler(downloader NoteDownloader, vaultRoot, publicBaseURL, authToken string, send SendFunc) *EmailHandler {
	return &EmailHandler{Downloader: downloader, VaultRoot: vaultRoot, PublicBaseURL: publicBaseURL, AuthToken: authToken, Send: send}
}

type emailRequest struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// emailImageResolver builds absolute, token-bearing asset URLs so images
// resolve correctly in an email client (which has no session, base tag, or
// custom headers of its own — see handlers/auth.go's query-param fallback).
func (h *EmailHandler) emailImageResolver() markdown.ImageResolver {
	return func(filename string) (string, bool) {
		path := h.VaultRoot + "/assets/" + filename
		q := url.Values{"path": {path}, "token": {h.AuthToken}}
		return h.PublicBaseURL + "api/asset?" + q.Encode(), true
	}
}

func (h *EmailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Path == "" {
		writeJSONError(w, http.StatusBadRequest, "missing note path")
		return
	}

	raw, err := h.Downloader.Download(req.Path)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to fetch the note to email")
		return
	}

	html := markdown.Render(raw, h.emailImageResolver())
	subject := "randoread: " + req.Title

	if err := h.Send(subject, html); err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to send email")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "sent"}) //nolint:errcheck
}
