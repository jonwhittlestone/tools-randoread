package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
)

// DropboxConnect implements the self-service "Connect Dropbox" flow: no
// token files are ever placed on the server by hand, the user authorizes
// via Dropbox's OAuth+PKCE consent screen and we persist the resulting
// tokens ourselves.
type DropboxConnect struct {
	client      *dropbox.Client
	redirectURI string

	// appURL is randoread's full external URL, including the Traefik
	// /randoread path prefix (e.g. "https://howapped.zapto.org/randoread/").
	// Needed because our server never sees that prefix (Traefik strips it),
	// but a Location-header redirect is resolved by the browser, which does
	// see it — a bare "/" would send the browser to the domain root instead
	// of back into the app.
	appURL string

	// pendingVerifier holds the PKCE code verifier between /auth and
	// /callback. This is a single-user, single-instance app (like
	// tools-browsernotes' equivalent in-memory state), so a single field is
	// sufficient — no need for a keyed session store.
	pendingVerifier string
}

// NewDropboxConnect builds a DropboxConnect using client for Dropbox API
// calls, redirecting back to redirectURI after the user authorizes, and
// bouncing the browser back to appURL once tokens are saved.
func NewDropboxConnect(client *dropbox.Client, redirectURI, appURL string) *DropboxConnect {
	return &DropboxConnect{client: client, redirectURI: redirectURI, appURL: appURL}
}

// HandleAuth serves GET /api/dropbox/auth — redirects the browser to
// Dropbox's PKCE consent screen.
func (dc *DropboxConnect) HandleAuth(w http.ResponseWriter, r *http.Request) {
	verifier, err := dropbox.GenerateCodeVerifier()
	if err != nil {
		http.Error(w, "failed to start Dropbox connect flow", http.StatusInternalServerError)
		return
	}
	dc.pendingVerifier = verifier

	challenge := dropbox.CodeChallenge(verifier)
	http.Redirect(w, r, dc.client.AuthorizeURL(challenge, dc.redirectURI), http.StatusFound)
}

// HandleCallback serves GET /api/dropbox/callback — Dropbox redirects here
// with an authorization code after the user consents. We exchange it for
// tokens and persist them, then send the browser back to the app.
func (dc *DropboxConnect) HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}
	if dc.pendingVerifier == "" {
		http.Error(w, "no Dropbox connect flow in progress", http.StatusBadRequest)
		return
	}

	tokens, err := dc.client.ExchangeCode(code, dc.pendingVerifier, dc.redirectURI)
	dc.pendingVerifier = ""
	if err != nil {
		http.Error(w, "failed to connect to Dropbox: "+err.Error(), http.StatusBadGateway)
		return
	}

	if err := dc.client.Store.Save(tokens); err != nil {
		http.Error(w, "failed to save Dropbox connection", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, dc.appURL, http.StatusFound)
}

// HandleStatus serves GET /api/dropbox/status.
func (dc *DropboxConnect) HandleStatus(w http.ResponseWriter, r *http.Request) {
	tokens, err := dc.client.Store.Load()
	if err != nil {
		http.Error(w, "failed to read Dropbox connection state", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"connected": tokens.AccessToken != ""}) //nolint:errcheck
}

// HandleDisconnect serves POST /api/dropbox/disconnect.
func (dc *DropboxConnect) HandleDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := dc.client.Store.Delete(); err != nil {
		http.Error(w, "failed to disconnect Dropbox", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "disconnected"}) //nolint:errcheck
}
