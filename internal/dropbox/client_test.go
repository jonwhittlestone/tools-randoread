package dropbox

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, apiSrv, contentSrv *httptest.Server) *Client {
	t.Helper()
	store := NewStore(filepath.Join(t.TempDir(), "tokens.json"))
	if err := store.Save(Tokens{AccessToken: "valid-token", RefreshToken: "refresh-token"}); err != nil {
		t.Fatalf("seed store: %v", err)
	}

	c := NewClient("app-key", store)
	if apiSrv != nil {
		c.APIBaseURL = apiSrv.URL
	}
	if contentSrv != nil {
		c.ContentBaseURL = contentSrv.URL
	}
	return c
}

func TestDownloadSuccess(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer valid-token" {
			t.Errorf("unexpected Authorization header: %q", got)
		}
		var arg struct {
			Path string `json:"path"`
		}
		json.Unmarshal([]byte(r.Header.Get("Dropbox-API-Arg")), &arg) //nolint:errcheck
		if arg.Path != "/notes/hello.md" {
			t.Errorf("unexpected path in Dropbox-API-Arg: %q", arg.Path)
		}
		w.Header().Set("dropbox-api-result", `{"rev":"abc"}`)
		w.Write([]byte("# hello")) //nolint:errcheck
	}))
	defer content.Close()

	c := newTestClient(t, nil, content)

	got, err := c.Download("/notes/hello.md")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(got) != "# hello" {
		t.Fatalf("Download body = %q, want %q", got, "# hello")
	}
}

func TestDownloadRefreshesExpiredTokenAndRetries(t *testing.T) {
	attempt := 0
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt++
		if attempt == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer refreshed-token" {
			t.Errorf("expected refreshed token on retry, got %q", got)
		}
		w.Header().Set("dropbox-api-result", `{"rev":"abc"}`)
		w.Write([]byte("body-after-refresh")) //nolint:errcheck
	}))
	defer content.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("unexpected refresh path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "grant_type=refresh_token") {
			t.Errorf("expected refresh_token grant, got %s", body)
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "refreshed-token"}) //nolint:errcheck
	}))
	defer api.Close()

	c := newTestClient(t, api, content)

	got, err := c.Download("/notes/hello.md")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(got) != "body-after-refresh" {
		t.Fatalf("Download body = %q, want body-after-refresh", got)
	}

	stored, err := c.Store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if stored.AccessToken != "refreshed-token" {
		t.Fatalf("expected stored access token to be updated, got %q", stored.AccessToken)
	}
}

func TestDownloadFailsWhenRefreshFails(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer content.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer api.Close()

	c := newTestClient(t, api, content)

	_, err := c.Download("/notes/hello.md")
	if err == nil {
		t.Fatal("expected error when refresh fails, got nil")
	}
}

func TestListFolderPaginates(t *testing.T) {
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/2/files/list_folder":
			fmt.Fprint(w, `{
				"entries": [{"path_display": "/a.md", "name": "a.md", ".tag": "file", "server_modified": "2026-01-01T00:00:00Z"}],
				"has_more": true,
				"cursor": "cursor-1"
			}`)
		case "/2/files/list_folder/continue":
			var body struct {
				Cursor string `json:"cursor"`
			}
			json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck
			if body.Cursor != "cursor-1" {
				t.Errorf("expected cursor-1, got %q", body.Cursor)
			}
			fmt.Fprint(w, `{
				"entries": [{"path_display": "/sub", "name": "sub", ".tag": "folder"}],
				"has_more": false,
				"cursor": ""
			}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer api.Close()

	c := newTestClient(t, api, nil)

	entries, err := c.ListFolder("/vault", true)
	if err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (list_folder + continue), got %d", calls)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}
	if entries[0].Path != "/a.md" || entries[0].IsFolder {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Path != "/sub" || !entries[1].IsFolder {
		t.Errorf("unexpected second entry: %+v", entries[1])
	}
}

func TestExchangeCode(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form := string(body)
		for _, want := range []string{"grant_type=authorization_code", "code=auth-code", "code_verifier=verifier-1"} {
			if !strings.Contains(form, want) {
				t.Errorf("expected form to contain %q, got %s", want, form)
			}
		}
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"access_token":  "new-access",
			"refresh_token": "new-refresh",
		})
	}))
	defer api.Close()

	c := newTestClient(t, api, nil)

	tokens, err := c.ExchangeCode("auth-code", "verifier-1", "https://example.com/callback")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tokens.AccessToken != "new-access" || tokens.RefreshToken != "new-refresh" {
		t.Fatalf("unexpected tokens: %+v", tokens)
	}
}

func TestAuthorizeURL(t *testing.T) {
	c := NewClient("my-app-key", NewStore("unused"))
	url := c.AuthorizeURL("my-challenge", "https://example.com/callback")

	for _, want := range []string{
		"client_id=my-app-key",
		"code_challenge=my-challenge",
		"code_challenge_method=S256",
		"token_access_type=offline",
		"redirect_uri=https%3A%2F%2Fexample.com%2Fcallback",
	} {
		if !strings.Contains(url, want) {
			t.Errorf("expected URL to contain %q, got %s", want, url)
		}
	}
}
