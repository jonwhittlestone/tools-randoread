// Package handlers implements randoread's HTTP handlers.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// TokenTTL is how long a login token remains valid after it was issued.
const TokenTTL = 90 * 24 * time.Hour

// AuthTokenHeader is the header the frontend sends the stored token back in
// on every API request, once logged in.
const AuthTokenHeader = "X-Auth-Token"

// Auth validates the single shared login token and its expiry.
type Auth struct {
	Token    string
	IssuedAt time.Time
	Now      func() time.Time // overridable for tests; defaults to time.Now
}

// NewAuth builds an Auth validator. now defaults to time.Now if nil.
func NewAuth(token string, issuedAt time.Time, now func() time.Time) *Auth {
	if now == nil {
		now = time.Now
	}
	return &Auth{Token: token, IssuedAt: issuedAt, Now: now}
}

// ExpiresAt returns when the current token stops being valid.
func (a *Auth) ExpiresAt() time.Time {
	return a.IssuedAt.Add(TokenTTL)
}

// Valid reports whether the supplied token matches and hasn't expired.
func (a *Auth) Valid(token string) bool {
	if token == "" || token != a.Token {
		return false
	}
	return a.Now().Before(a.ExpiresAt())
}

// HandleValidate serves GET /api/auth?token=... — used by the deep-link login
// flow so the token is checked server-side before the frontend stores it.
func (a *Auth) HandleValidate(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	w.Header().Set("Content-Type", "application/json")

	if !a.Valid(token) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"valid": false}) //nolint:errcheck
		return
	}

	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"valid":     true,
		"expiresAt": a.ExpiresAt().Format(time.RFC3339),
	})
}

// publicPaths bypass token auth entirely: static assets and the auth check
// endpoint itself (which performs its own validation).
func isPublicPath(path string) bool {
	if path == "/health" || path == "/api/auth" {
		return true
	}
	return !strings.HasPrefix(path, "/api/")
}

// RequireToken is middleware that enforces AuthTokenHeader on every /api/
// route except the public ones above.
func (a *Auth) RequireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get(AuthTokenHeader)
		if !a.Valid(token) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"error": "missing or invalid auth token"}) //nolint:errcheck
			return
		}

		next.ServeHTTP(w, r)
	})
}
