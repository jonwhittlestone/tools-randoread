package handlers

import (
	"net/url"
	"sync"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
)

// vaultFileResolver builds the /api/asset proxy URL for an Obsidian embed
// by looking up its bare filename in a full recursive vault listing —
// matching how Obsidian itself resolves "![[file]]" embeds vault-wide,
// rather than assuming a fixed folder like assets/. That assumption breaks
// for files that live elsewhere, e.g. tools-browsernotes' reMarkable sync
// drops handwritten-note PDFs in _remarkable-emails-via-browsernotes/.
//
// The listing is fetched lazily (only if the note actually contains an
// embed) and only once per call, backed by the same cached lister
// Rando/Clipped use — so this doesn't add a Dropbox round trip on every
// request in steady state.
//
// authToken is embedded as a query param because the browser requests this
// URL directly via <img src>/<object data>, which can't carry the
// X-Auth-Token header — RequireToken accepts either (see handlers/auth.go).
func vaultFileResolver(lister NoteLister, vaultRoot, authToken string) markdown.ImageResolver {
	var (
		once   sync.Once
		byName map[string]string
	)

	load := func() {
		byName = map[string]string{}
		entries, err := lister.ListFolder(vaultRoot, true)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsFolder || isConflictedCopy(e.Name) {
				continue
			}
			if _, exists := byName[e.Name]; !exists {
				byName[e.Name] = e.Path
			}
		}
	}

	return func(filename string) (string, bool) {
		once.Do(load)

		path, ok := byName[filename]
		if !ok {
			return "", false
		}
		q := url.Values{"path": {path}, "token": {authToken}}
		return "api/asset?" + q.Encode(), true
	}
}
