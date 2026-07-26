package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		AuthToken:          "secret",
		AuthTokenIssuedAt:  time.Now().Add(-time.Hour),
		DataDir:            t.TempDir(),
		DropboxAppKey:      "app-key",
		DropboxRedirectURI: "https://example.com/api/dropbox/callback",
		VaultRoot:          "/DropsyncFiles/jw-mind",
	}
}

func TestHealthEndpoint(t *testing.T) {
	mux := newMux(testConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	got := rec.Body.String()
	want := `{"status":"ok"}`
	if got != want {
		t.Fatalf("expected body %q, got %q", want, got)
	}
}

func TestHealthEndpointIncludesCommitHashWhenSet(t *testing.T) {
	// CommitHash is set on every real deploy (see deploy/deploy.sh), so
	// /health can be used to confirm which commit is actually live —
	// unset (the zero value, as in every other test's testConfig) omits
	// the field entirely rather than showing a misleading empty string.
	cfg := testConfig(t)
	cfg.CommitHash = "b2b5173"
	mux := newMux(cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body struct {
		Status string `json:"status"`
		Commit string `json:"commit"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
	if body.Commit != "b2b5173" {
		t.Errorf("commit = %q, want %q", body.Commit, "b2b5173")
	}
}

func TestServesStaticIndex(t *testing.T) {
	mux := newMux(testConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestAuthEndpointWired(t *testing.T) {
	mux := newMux(testConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/api/auth?token=secret", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestUnauthenticatedAPIRequestRejected(t *testing.T) {
	mux := newMux(testConfig(t))

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist-yet", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestClippingsRoutesWired(t *testing.T) {
	mux := newMux(testConfig(t))

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/clippings?token=secret", ""},
		{http.MethodPost, "/api/clippings/send-to-remarkable?token=secret", `{"path":"x","title":"y"}`},
	}

	for _, c := range cases {
		var body io.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		}
		req := httptest.NewRequest(c.method, c.path, body)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Neither Dropbox nor the tablet are configured in tests, so these
		// fail at the network call (502) — this just pins that the route
		// exists and passes auth, not a 404/401.
		if rec.Code == http.StatusNotFound || rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s: expected the route to be wired and authorized, got %d: %s", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
}

func TestDropboxRoutesWired(t *testing.T) {
	mux := newMux(testConfig(t))

	cases := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/api/dropbox/auth?token=secret", http.StatusFound},
		{http.MethodGet, "/api/dropbox/status?token=secret", http.StatusOK},
		{http.MethodPost, "/api/dropbox/disconnect?token=secret", http.StatusOK},
	}

	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != c.want {
			t.Errorf("%s %s: expected status %d, got %d", c.method, c.path, c.want, rec.Code)
		}
	}
}
