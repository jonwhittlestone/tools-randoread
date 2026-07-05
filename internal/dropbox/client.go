package dropbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultAPIBaseURL     = "https://api.dropboxapi.com"
	defaultContentBaseURL = "https://content.dropboxapi.com"
	authorizeURL          = "https://www.dropbox.com/oauth2/authorize"
)

// Entry is a file or folder returned by ListFolder.
type Entry struct {
	Path       string
	Name       string
	IsFolder   bool
	ModifiedAt time.Time // zero value for folders, which have no modified time
}

// Client is a minimal Dropbox API client: OAuth2+PKCE token exchange and
// refresh, files/download, and files/list_folder (paginated). All Dropbox
// API calls transparently refresh the access token on a 401 and persist the
// new one via Store, so callers never have to think about token lifetime.
type Client struct {
	AppKey         string
	Store          *Store
	HTTPClient     *http.Client
	APIBaseURL     string
	ContentBaseURL string
}

// NewClient builds a Client for the given app key, persisting tokens via store.
func NewClient(appKey string, store *Store) *Client {
	return &Client{
		AppKey:         appKey,
		Store:          store,
		APIBaseURL:     defaultAPIBaseURL,
		ContentBaseURL: defaultContentBaseURL,
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// AuthorizeURL builds the Dropbox PKCE authorization URL a user should be
// redirected to in order to connect their Dropbox account.
func (c *Client) AuthorizeURL(codeChallenge, redirectURI string) string {
	q := url.Values{
		"client_id":             {c.AppKey},
		"response_type":         {"code"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"token_access_type":     {"offline"},
		"redirect_uri":          {redirectURI},
	}
	return authorizeURL + "?" + q.Encode()
}

// ExchangeCode exchanges an OAuth authorization code (plus its PKCE
// verifier) for an access+refresh token pair.
func (c *Client) ExchangeCode(code, verifier, redirectURI string) (Tokens, error) {
	form := url.Values{
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"client_id":     {c.AppKey},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}
	return c.postTokenForm(form)
}

// RefreshAccessToken exchanges a refresh token for a new access token.
func (c *Client) RefreshAccessToken(refreshToken string) (string, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.AppKey},
	}
	tokens, err := c.postTokenForm(form)
	if err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

func (c *Client) postTokenForm(form url.Values) (Tokens, error) {
	req, err := http.NewRequest(http.MethodPost, c.APIBaseURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Tokens{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Tokens{}, fmt.Errorf("dropbox token request failed: status %d: %s", resp.StatusCode, body)
	}

	var tokens Tokens
	if err := json.Unmarshal(body, &tokens); err != nil {
		return Tokens{}, err
	}
	return tokens, nil
}

// Download fetches the raw content of the file at path (files/download).
func (c *Client) Download(path string) ([]byte, error) {
	arg, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return nil, err
	}

	return c.authorizedRequest(http.MethodPost, c.ContentBaseURL+"/2/files/download", nil, map[string]string{
		"Dropbox-API-Arg": string(arg),
	})
}

type listFolderEntry struct {
	Type           string `json:".tag"`
	Name           string `json:"name"`
	PathDisplay    string `json:"path_display"`
	ServerModified string `json:"server_modified"`
}

type listFolderResponse struct {
	Entries []listFolderEntry `json:"entries"`
	Cursor  string            `json:"cursor"`
	HasMore bool              `json:"has_more"`
}

// ListFolder lists the contents of path, following pagination cursors until
// has_more is false. Folder entries have a zero ModifiedAt.
func (c *Client) ListFolder(path string, recursive bool) ([]Entry, error) {
	body, err := json.Marshal(map[string]any{
		"path":                                path,
		"recursive":                           recursive,
		"include_media_info":                  false,
		"include_deleted":                     false,
		"include_has_explicit_shared_members": false,
	})
	if err != nil {
		return nil, err
	}

	data, err := c.authorizedRequest(http.MethodPost, c.APIBaseURL+"/2/files/list_folder", bytes.NewReader(body), map[string]string{
		"Content-Type": "application/json",
	})
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for {
		var page listFolderResponse
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		entries = append(entries, toEntries(page.Entries)...)

		if !page.HasMore {
			break
		}

		continueBody, err := json.Marshal(map[string]string{"cursor": page.Cursor})
		if err != nil {
			return nil, err
		}
		data, err = c.authorizedRequest(http.MethodPost, c.APIBaseURL+"/2/files/list_folder/continue", bytes.NewReader(continueBody), map[string]string{
			"Content-Type": "application/json",
		})
		if err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func toEntries(raw []listFolderEntry) []Entry {
	entries := make([]Entry, 0, len(raw))
	for _, e := range raw {
		entry := Entry{
			Path:     e.PathDisplay,
			Name:     e.Name,
			IsFolder: e.Type == "folder",
		}
		if e.ServerModified != "" {
			if t, err := time.Parse(time.RFC3339, e.ServerModified); err == nil {
				entry.ModifiedAt = t
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// authorizedRequest performs an authenticated Dropbox API call, refreshing
// the access token and retrying once if the server responds 401.
func (c *Client) authorizedRequest(method, targetURL string, body io.Reader, headers map[string]string) ([]byte, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}

	tokens, err := c.Store.Load()
	if err != nil {
		return nil, err
	}

	do := func(accessToken string) (*http.Response, error) {
		req, err := http.NewRequest(method, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return c.httpClient().Do(req)
	}

	resp, err := do(tokens.AccessToken)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && tokens.RefreshToken != "" {
		resp.Body.Close() //nolint:errcheck

		newAccessToken, err := c.RefreshAccessToken(tokens.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("dropbox token refresh failed: %w", err)
		}
		tokens.AccessToken = newAccessToken
		if err := c.Store.Save(tokens); err != nil {
			return nil, err
		}

		resp, err = do(newAccessToken)
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close() //nolint:errcheck

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dropbox request to %s failed: status %d: %s", targetURL, resp.StatusCode, data)
	}
	return data, nil
}
