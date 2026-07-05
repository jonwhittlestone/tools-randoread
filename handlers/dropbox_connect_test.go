package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
)

func newTestDropboxConnect(t *testing.T) *DropboxConnect {
	t.Helper()
	store := dropbox.NewStore(filepath.Join(t.TempDir(), "tokens.json"))
	client := dropbox.NewClient("app-key", store)
	return NewDropboxConnect(client, "https://example.com/api/dropbox/callback", "https://example.com/randoread/")
}

func TestHandleAuthRedirectsToDropbox(t *testing.T) {
	dc := newTestDropboxConnect(t)

	req := httptest.NewRequest(http.MethodGet, "/api/dropbox/auth", nil)
	rec := httptest.NewRecorder()
	dc.HandleAuth(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("invalid Location header: %v", err)
	}
	if loc.Host != "www.dropbox.com" {
		t.Fatalf("expected redirect to dropbox.com, got %s", loc.Host)
	}
	if loc.Query().Get("client_id") != "app-key" {
		t.Fatalf("expected client_id=app-key, got %s", loc.Query().Get("client_id"))
	}
}

func TestHandleCallbackWithoutPendingAuthFails(t *testing.T) {
	dc := newTestDropboxConnect(t)

	req := httptest.NewRequest(http.MethodGet, "/api/dropbox/callback?code=abc", nil)
	rec := httptest.NewRecorder()
	dc.HandleCallback(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when no auth was initiated, got %d", rec.Code)
	}
}

func TestHandleCallbackExchangesCodeAndSavesTokens(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"access_token":"access-1","refresh_token":"refresh-1"}`)) //nolint:errcheck
	}))
	defer tokenSrv.Close()

	dc := newTestDropboxConnect(t)
	dc.client.APIBaseURL = tokenSrv.URL

	// Simulate having initiated the flow first, which stores a PKCE verifier.
	authReq := httptest.NewRequest(http.MethodGet, "/api/dropbox/auth", nil)
	dc.HandleAuth(httptest.NewRecorder(), authReq)

	req := httptest.NewRequest(http.MethodGet, "/api/dropbox/callback?code=auth-code", nil)
	rec := httptest.NewRecorder()
	dc.HandleCallback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect after connecting, got %d: %s", rec.Code, rec.Body.String())
	}
	// Regression: this must be the app's full external URL (including the
	// Traefik /randoread path prefix), not a bare "/" — our server has no
	// visibility into that prefix (Traefik strips it), but the browser's
	// Location-header navigation does, so a bare "/" 404s at the domain root.
	if loc := rec.Header().Get("Location"); loc != "https://example.com/randoread/" {
		t.Fatalf("expected redirect to the app's public URL, got %q", loc)
	}

	tokens, err := dc.client.Store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tokens.AccessToken != "access-1" || tokens.RefreshToken != "refresh-1" {
		t.Fatalf("unexpected tokens saved: %+v", tokens)
	}
}

func TestHandleStatus(t *testing.T) {
	dc := newTestDropboxConnect(t)

	req := httptest.NewRequest(http.MethodGet, "/api/dropbox/status", nil)
	rec := httptest.NewRecorder()
	dc.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"connected":false}`+"\n" {
		t.Fatalf("expected disconnected status, got %q", got)
	}
}

func TestHandleDisconnect(t *testing.T) {
	dc := newTestDropboxConnect(t)
	if err := dc.client.Store.Save(dropbox.Tokens{AccessToken: "a"}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/dropbox/disconnect", nil)
	rec := httptest.NewRecorder()
	dc.HandleDisconnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	tokens, err := dc.client.Store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tokens.AccessToken != "" {
		t.Fatalf("expected tokens cleared after disconnect, got %+v", tokens)
	}
}
