package main

import (
	"net/http"
	"net/http/httptest"
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
