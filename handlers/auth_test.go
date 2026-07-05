package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestAuthValid(t *testing.T) {
	issuedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name  string
		token string
		now   time.Time
		want  bool
	}{
		{"correct token, fresh", "secret", issuedAt.Add(time.Hour), true},
		{"wrong token", "nope", issuedAt.Add(time.Hour), false},
		{"empty token", "", issuedAt.Add(time.Hour), false},
		{"just under 90 days", "secret", issuedAt.Add(89 * 24 * time.Hour), true},
		{"expired after 90 days", "secret", issuedAt.Add(91 * 24 * time.Hour), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := NewAuth("secret", issuedAt, fixedClock(c.now))
			if got := a.Valid(c.token); got != c.want {
				t.Errorf("Valid(%q) at %v = %v, want %v", c.token, c.now, got, c.want)
			}
		})
	}
}

func TestHandleValidate(t *testing.T) {
	issuedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := NewAuth("secret", issuedAt, fixedClock(issuedAt.Add(time.Hour)))

	t.Run("valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth?token=secret", nil)
		rec := httptest.NewRecorder()
		a.HandleValidate(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/auth?token=wrong", nil)
		rec := httptest.NewRecorder()
		a.HandleValidate(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestRequireToken(t *testing.T) {
	issuedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := NewAuth("secret", issuedAt, fixedClock(issuedAt.Add(time.Hour)))

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	protected := a.RequireToken(ok)

	t.Run("missing header on protected route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/daily", nil)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("valid header on protected route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/daily", nil)
		req.Header.Set(AuthTokenHeader, "secret")
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("public path bypasses auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("query param token accepted as fallback", func(t *testing.T) {
		// Browser-native navigations (OAuth redirects, <img> tags) can't set
		// custom headers, so protected routes also accept ?token=.
		req := httptest.NewRequest(http.MethodGet, "/api/asset?token=secret&path=/x.png", nil)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("dropbox oauth callback is public", func(t *testing.T) {
		// Dropbox's redirect back to us carries no auth token; the PKCE
		// state itself is what gates this endpoint.
		req := httptest.NewRequest(http.MethodGet, "/api/dropbox/callback?code=abc", nil)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
	})
}
