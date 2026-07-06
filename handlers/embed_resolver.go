package handlers

import (
	"net/url"
	"strings"
	"sync"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
)

// vaultFileResolver builds the /api/asset proxy URL for an Obsidian embed
// by looking it up in a full recursive vault listing — matching how
// Obsidian itself resolves "![[file]]" embeds vault-wide, rather than
// assuming a fixed folder like assets/. That assumption breaks for files
// that live elsewhere, e.g. tools-browsernotes' reMarkable sync drops
// handwritten-note PDFs in _remarkable-emails-via-browsernotes/.
//
// Obsidian usually writes a bare filename ("![[photo.jpg]]"), but qualifies
// it with a vault-relative path ("![[folder/note.pdf]]") when the bare name
// alone is ambiguous — e.g. reMarkable's daily "focus" PDF exports reuse the
// same filename pattern across different date subfolders. A path-qualified
// reference is matched against the exact vault-relative path first; a bare
// filename falls back to a by-name lookup (first match wins if it happens
// to be ambiguous, since there's no path to disambiguate with).
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
		byPath map[string]bool
		byName map[string]string
	)

	load := func() {
		byPath = map[string]bool{}
		byName = map[string]string{}
		entries, err := lister.ListFolder(vaultRoot, true)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsFolder || isConflictedCopy(e.Name) {
				continue
			}
			byPath[e.Path] = true
			if _, exists := byName[e.Name]; !exists {
				byName[e.Name] = e.Path
			}
		}
	}

	buildURL := func(path string) string {
		q := url.Values{"path": {path}, "token": {authToken}}
		return "api/asset?" + q.Encode()
	}

	return func(ref string) (string, bool) {
		once.Do(load)

		if strings.Contains(ref, "/") {
			fullPath := vaultRoot + "/" + ref
			if byPath[fullPath] {
				return buildURL(fullPath), true
			}
			// Fall through to a bare-filename lookup on the last path
			// segment, in case the given path doesn't line up exactly
			// with Dropbox's path_display for some reason.
			ref = ref[strings.LastIndex(ref, "/")+1:]
		}

		path, ok := byName[ref]
		if !ok {
			return "", false
		}
		return buildURL(path), true
	}
}
