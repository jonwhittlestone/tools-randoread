package journaldraft

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return NewClient(srv.URL, "test-key")
}

func TestAvailable_HealthOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/journal/health" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Health check must not require auth — see client.go's doc comment.
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty (health check is unauthenticated)", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if !c.Available() {
		t.Error("Available() = false, want true")
	}
}

func TestAvailable_Unreachable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "test-key") // port 0/1 refuses immediately
	if c.Available() {
		t.Error("Available() = true, want false for an unreachable host")
	}
}

func TestAvailable_Unconfigured(t *testing.T) {
	c := NewClient("", "")
	if c.Available() {
		t.Error("Available() = true, want false when BaseURL is empty")
	}
}

func TestAvailable_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if c.Available() {
		t.Error("Available() = true, want false for a non-200 status")
	}
}

func TestDraft_Success(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody) //nolint:errcheck

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"heading":           "## 📌 etc.",
			"insertionMarkdown": "- `11:12`: Jon commented that he got a lot of value out of the sprint retrospective today",
			"reply":             "It's good for closure that you have these endings to a sprint. I'll make a note of this in the etc section.",
		})
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	result, err := c.Draft(context.Background(), "## 📌 etc.\n\n- ", "Great sprint retro today.", "2026-08-14T11:12:00+01:00")
	if err != nil {
		t.Fatalf("Draft() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotPath != "/internal/journal/draft" {
		t.Errorf("path = %q, want /internal/journal/draft", gotPath)
	}
	if gotBody["userText"] != "Great sprint retro today." {
		t.Errorf("userText = %q", gotBody["userText"])
	}
	if gotBody["nowIso"] != "2026-08-14T11:12:00+01:00" {
		t.Errorf("nowIso = %q", gotBody["nowIso"])
	}
	if result.Heading != "## 📌 etc." || result.InsertionMarkdown == "" || result.Reply == "" {
		t.Errorf("Draft() = %+v", result)
	}
}

func TestDraft_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("agent error")) //nolint:errcheck
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.Draft(context.Background(), "note", "text", "2026-08-14T11:12:00+01:00")
	if err == nil {
		t.Fatal("Draft() error = nil, want an error for a non-200 response")
	}
}

func TestDraft_Unconfigured(t *testing.T) {
	c := NewClient("", "")
	_, err := c.Draft(context.Background(), "note", "text", "2026-08-14T11:12:00+01:00")
	if err == nil {
		t.Fatal("Draft() error = nil, want an error when unconfigured")
	}
}

func TestConfigured(t *testing.T) {
	if (&Client{}).Configured() {
		t.Error("Configured() = true, want false for an empty Client")
	}
	if !(&Client{BaseURL: "http://x"}).Configured() {
		t.Error("Configured() = false, want true when BaseURL is set")
	}
}
