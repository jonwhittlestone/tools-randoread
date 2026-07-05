package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{
		AuthToken:         "secret",
		AuthTokenIssuedAt: time.Now().Add(-time.Hour),
	}
}

func TestHealthEndpoint(t *testing.T) {
	mux := newMux(testConfig())

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
	mux := newMux(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestAuthEndpointWired(t *testing.T) {
	mux := newMux(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/auth?token=secret", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestUnauthenticatedAPIRequestRejected(t *testing.T) {
	mux := newMux(testConfig())

	req := httptest.NewRequest(http.MethodGet, "/api/does-not-exist-yet", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}
