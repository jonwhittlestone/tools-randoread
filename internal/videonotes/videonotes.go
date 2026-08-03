// Package videonotes builds and edits the per-video markdown notes for the
// "Watching It Later" feature — see main-randoread.md section 05.02. Pure
// string/path logic only; Dropbox I/O and the tools-watchitlater lookup live
// in handlers/watching_notes.go.
package videonotes

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// notesDirSuffix is the vault-relative folder every per-video note lives in.
const notesDirSuffix = "/Clippings/randoread-watching-it-later"

// vaultReferencesHeading is the exact heading line templates/video-notes.md
// (and every note derived from it) uses to mark the references section.
const vaultReferencesHeading = "## vault references"

// nothingPlaceholder is what an empty vault references list reads as —
// applied by ApplyTemplate and replaced by the first AppendReference call.
const nothingPlaceholder = "- nothing"

// onlineLinkPlaceholder/templateReferencePlaceholder are the exact lines
// templates/video-notes.md uses as fill-in-the-blank markers.
const onlineLinkPlaceholder = "> online link"
const templateReferencePlaceholder = "- [directory1-from-vault-relative / directory 2 / note-title](template/...)"

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// Slug lowercases s and collapses every run of non-alphanumeric characters
// into a single hyphen, trimming leading/trailing hyphens.
func Slug(s string) string {
	lower := strings.ToLower(s)
	slug := slugPattern.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}

// Suffix is the stable lookup key for a video's note: every filename minted
// by Filename ends with this, regardless of the date prefix it was created
// with. See the "why not derive from StagedAt" design decision in the
// feature plan — StagedAt changes every time a video is (re-)staged, so it
// can't be part of a stable identity key, but the title doesn't.
func Suffix(title string) string {
	return "-" + Slug(title) + ".md"
}

// Filename mints a brand-new note filename — used only the first time a
// video's note is ever saved. Later saves overwrite whatever path was found
// via a Suffix match instead of re-minting this.
func Filename(today time.Time, playlistRank int, title string) string {
	return fmt.Sprintf("%s-%d%s", today.Format("2006-01-02"), playlistRank, Suffix(title))
}

// Dir returns the vault-relative per-video notes folder under vaultRoot.
func Dir(vaultRoot string) string {
	return vaultRoot + notesDirSuffix
}

// ApplyTemplate fills in templates/video-notes.md's two placeholders: the
// YouTube link and (since a fresh note has no references yet) the vault
// references list. Everything else in the template — including the "..."
// body placeholder — is left untouched; it's immediately editable anyway.
func ApplyTemplate(templateRaw, youtubeURL string) string {
	result := strings.Replace(templateRaw, onlineLinkPlaceholder, "> "+youtubeURL, 1)
	result = strings.Replace(result, templateReferencePlaceholder, nothingPlaceholder, 1)
	return result
}

// Reference is one entry in a note's "## vault references" section. Tagged
// lowercase — without this, encoding/json emits "Title"/"Path" verbatim,
// which silently doesn't match watching-notes.js's ref.title/ref.path
// (found live: every reference-link click fell through to a real page
// navigation instead of the intended inline preview, since the frontend's
// path lookup was always undefined).
type Reference struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

var referenceLinePattern = regexp.MustCompile(`^- \[([^\]]*)\]\(([^)]*)\)$`)

// ParseReferences extracts the "## vault references" section's `- [title](path)`
// list items, skipping the "- nothing" placeholder.
func ParseReferences(content string) []Reference {
	lines := strings.Split(content, "\n")
	start, end := referencesSectionBounds(lines)
	if start == -1 {
		return nil
	}

	var refs []Reference
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		m := referenceLinePattern.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		refs = append(refs, Reference{Title: m[1], Path: m[2]})
	}
	return refs
}

// AppendReference adds ref to content's "## vault references" section:
// replacing the "- nothing" placeholder if that's still the only entry, or
// appending a new list item after the last existing one. Returns content
// unchanged if it has no vault references section at all (shouldn't happen
// for a note built via ApplyTemplate).
func AppendReference(content string, ref Reference) string {
	lines := strings.Split(content, "\n")
	start, end := referencesSectionBounds(lines)
	if start == -1 {
		return content
	}

	newLine := fmt.Sprintf("- [%s](%s)", ref.Title, ref.Path)

	nothingIdx := -1
	lastListIdx := -1
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == nothingPlaceholder {
			nothingIdx = i
		}
		if strings.HasPrefix(trimmed, "- ") {
			lastListIdx = i
		}
	}

	if nothingIdx != -1 {
		lines[nothingIdx] = newLine
		return strings.Join(lines, "\n")
	}

	insertAt := start
	if lastListIdx != -1 {
		insertAt = lastListIdx + 1
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:insertAt]...)
	out = append(out, newLine)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n")
}

// referencesSectionBounds returns the [start,end) line-index range of the
// vault references section's body (the lines after the heading, up to but
// excluding the next "---" or "## " heading). start is -1 if the heading
// isn't present.
func referencesSectionBounds(lines []string) (start, end int) {
	start = -1
	for i, line := range lines {
		if strings.TrimSpace(line) == vaultReferencesHeading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return -1, -1
	}

	end = len(lines)
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "---" || strings.HasPrefix(trimmed, "## ") {
			end = i
			break
		}
	}
	return start, end
}
