// Package journaldraft is a thin HTTP client for NanoClaw's one-shot
// journal-draft endpoint (src/journal-draft.ts in the nanoclaw repo) —
// used by randoread's floating "Send to oh-two" journal input (see
// main-randoread.md / the 26-nanoclaw vault project's main.md §05.05).
//
// NanoClaw runs on a different host (doylestone02) from randoread
// (doylestonex), bridged over Tailscale — see main.md §05.05.02
// "Security". This client talks to NanoClaw server-to-server, using its
// own bearer token; the browser only ever talks to randoread, mirroring
// how tools-watchitlater/Dropbox/reMarkable/SMTP credentials are already
// brokered server-side (see internal/watchitlater, internal/dropbox).
package journaldraft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// healthCheckTimeout bounds Available()'s liveness probe — short, since it
// backs a frontend "is the feature usable right now" check (main.md
// §05.05: "if doylestone02 isn't on tailscale, the feature should be
// unavailable") that shouldn't make the page hang waiting on a dead host.
const healthCheckTimeout = 2 * time.Second

// Client talks to a single NanoClaw journal-draft endpoint.
type Client struct {
	BaseURL    string // e.g. "http://100.111.143.116:3021"
	APIKey     string
	HTTPClient *http.Client // overridable for tests; defaults to http.DefaultClient
}

// NewClient builds a Client for baseURL, authenticating with apiKey.
// baseURL == "" is valid — it just means the feature isn't configured;
// Available() reports false and Draft() errors immediately without making
// a network call.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{BaseURL: baseURL, APIKey: apiKey}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// Configured reports whether a base URL was set at all — distinct from
// Available(), which additionally checks the host is actually reachable
// right now.
func (c *Client) Configured() bool {
	return c.BaseURL != ""
}

// Available reports whether NanoClaw's journal-draft endpoint is reachable
// right now, via its unauthenticated /internal/journal/health liveness
// check (no bearer token needed — see that handler's doc comment). Any
// failure (unconfigured, unreachable, non-200, timeout) reports false
// rather than an error — this backs a simple frontend on/off switch, not a
// diagnostic.
func (c *Client) Available() bool {
	if !c.Configured() {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	// Deliberately no Authorization header — the health check is
	// unauthenticated server-side (see the endpoint's doc comment in
	// src/journal-draft.ts), and availability should reflect reachability
	// alone, independent of whether the two hosts' API keys currently
	// agree.
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, strings.TrimSuffix(c.BaseURL, "/")+"/internal/journal/health", nil,
	)
	if err != nil {
		return false
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Result mirrors NanoClaw's JournalDraftResult — see src/journal-draft.ts.
type Result struct {
	Heading           string `json:"heading"`
	InsertionMarkdown string `json:"insertionMarkdown"`
	Reply             string `json:"reply"`
}

// Draft asks NanoClaw to classify userText against dailyNoteRaw's headings
// and draft the line to insert. nowISO is the current local time
// (RFC3339) — NanoClaw uses it to timestamp the drafted bullet, so it must
// be the caller's wall-clock time, not NanoClaw's (the two hosts' clocks
// aren't assumed to agree, and it's the user's local time that belongs in
// their journal regardless).
func (c *Client) Draft(ctx context.Context, dailyNoteRaw, userText, nowISO string) (*Result, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("journal draft endpoint not configured")
	}

	body, err := json.Marshal(map[string]string{
		"dailyNoteRaw": dailyNoteRaw,
		"userText":     userText,
		"nowIso":       nowISO,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal draft request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, "internal/journal/draft", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("draft journal entry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body) //nolint:errcheck
		return nil, fmt.Errorf("draft journal entry: unexpected status %d: %s", resp.StatusCode, msg)
	}

	var result Result
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode draft response: %w", err)
	}
	return &result, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := strings.TrimSuffix(c.BaseURL, "/") + "/" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", path, err)
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return req, nil
}
