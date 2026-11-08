package main

// Installable-app (PWA) contract — see main-randoread.md §05.03. These go
// through newMux(), the same path Traefik proxies to, so they cover the
// embed wiring and the served Content-Type, not just the files on disk.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func getPath(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type webManifest struct {
	Name            string `json:"name"`
	ShortName       string `json:"short_name"`
	StartURL        string `json:"start_url"`
	Scope           string `json:"scope"`
	Display         string `json:"display"`
	BackgroundColor string `json:"background_color"`
	ThemeColor      string `json:"theme_color"`
	Icons           []struct {
		Src     string `json:"src"`
		Sizes   string `json:"sizes"`
		Type    string `json:"type"`
		Purpose string `json:"purpose"`
	} `json:"icons"`
}

func fetchManifest(t *testing.T, h http.Handler) webManifest {
	t.Helper()
	rec := getPath(t, h, "/manifest.webmanifest")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /manifest.webmanifest: status %d, want 200", rec.Code)
	}
	var m webManifest
	if err := json.NewDecoder(rec.Body).Decode(&m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	return m
}

func TestWebManifestContentType(t *testing.T) {
	// Go's mime table and the debian:bookworm-slim runtime image both lack
	// .webmanifest, so without explicit registration FileServer sniffs it as
	// text/plain — which Chrome may refuse to treat as a manifest.
	rec := getPath(t, newMux(testConfig(t)), "/manifest.webmanifest")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/manifest+json") {
		t.Errorf("Content-Type = %q, want application/manifest+json", ct)
	}
}

func TestWebManifestIsInstallableStandalone(t *testing.T) {
	m := fetchManifest(t, newMux(testConfig(t)))

	if m.Name == "" || m.ShortName == "" {
		t.Errorf("name/short_name must both be set, got %q / %q", m.Name, m.ShortName)
	}
	if m.Display != "standalone" {
		t.Errorf("display = %q, want standalone (hides the browser bar)", m.Display)
	}
	// Relative, so they resolve against the manifest's own URL
	// (/randoread/manifest.webmanifest) — Traefik strips /randoread before
	// it reaches us, so an absolute "/" would escape the app's path.
	if m.StartURL != "./" || m.Scope != "./" {
		t.Errorf("start_url/scope = %q/%q, want ./ and ./", m.StartURL, m.Scope)
	}
	if m.ThemeColor == "" || m.BackgroundColor == "" {
		t.Errorf("theme_color/background_color must be set for the splash screen")
	}
}

func TestWebManifestIconsResolve(t *testing.T) {
	mux := newMux(testConfig(t))
	m := fetchManifest(t, mux)

	sizes := map[string]bool{}
	maskable := false
	for _, icon := range m.Icons {
		sizes[icon.Sizes] = true
		if strings.Contains(icon.Purpose, "maskable") {
			maskable = true
		}
		if strings.HasPrefix(icon.Src, "/") {
			t.Errorf("icon src %q must be relative (see start_url comment)", icon.Src)
			continue
		}
		rec := getPath(t, mux, "/"+icon.Src)
		if rec.Code != http.StatusOK {
			t.Errorf("icon %s: status %d, want 200", icon.Src, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Errorf("icon %s: Content-Type %q, want image/png", icon.Src, ct)
		}
	}
	for _, want := range []string{"192x192", "512x512"} {
		if !sizes[want] {
			t.Errorf("missing %s icon (Chrome requires 192 and 512)", want)
		}
	}
	if !maskable {
		t.Errorf("want a maskable icon so Android's adaptive crop doesn't clip it")
	}
}

func TestIndexLinksManifest(t *testing.T) {
	rec := getPath(t, newMux(testConfig(t)), "/")
	body := rec.Body.String()
	if !strings.Contains(body, `rel="manifest" href="manifest.webmanifest"`) {
		t.Errorf(`index.html must link the manifest: <link rel="manifest" href="manifest.webmanifest" />`)
	}
	if !strings.Contains(body, `name="theme-color"`) {
		t.Errorf("index.html should set a theme-color meta for the status bar")
	}
}
