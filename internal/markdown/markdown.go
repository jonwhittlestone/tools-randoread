// Package markdown renders Obsidian-flavored markdown (wikilink images,
// wikilinks, GFM tables/tasklists/autolinks) to HTML for the Dark Moss theme.
package markdown

import (
	"bytes"
	"fmt"
	stdhtml "html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// ImageResolver maps a bare filename from an Obsidian embed (e.g.
// "![[photo.png]]" or "![[note.pdf]]") to a servable URL. It returns
// ok=false if the file can't be located, in which case a text placeholder
// is rendered instead.
type ImageResolver func(filename string) (url string, ok bool)

var (
	embedPattern         = regexp.MustCompile(`!\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	wikilinkPattern      = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	standardImagePattern = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)
)

var renderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// Render converts Obsidian-flavored markdown source to HTML.
//
// Obsidian syntax not natively understood by goldmark is preprocessed first:
//   - ![[file.png]] / ![[file.png|alt]] embeds resolve via resolveImage to a
//     standard markdown image, or a plain-text placeholder if unresolved.
//   - [[Note]] / [[Note|alias]] wikilinks become plain text (not navigable —
//     there's no in-app vault browser).
//
// Both are skipped inside fenced code blocks so example markdown in a note
// doesn't get rewritten.
func Render(source []byte, resolveImage ImageResolver) string {
	body := stripFrontmatter(string(source))
	preprocessed := preprocess(body, resolveImage)

	var buf bytes.Buffer
	if err := renderer.Convert([]byte(preprocessed), &buf); err != nil {
		return "<p>failed to render note</p>"
	}
	return buf.String()
}

func preprocess(source string, resolveImage ImageResolver) string {
	lines := strings.Split(source, "\n")
	inFence := false

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		// Runs before embedPattern below, which produces its own
		// "![alt](url)" output — matching that here too would double-process
		// an already-resolved URL.
		line = standardImagePattern.ReplaceAllStringFunc(line, func(match string) string {
			groups := standardImagePattern.FindStringSubmatch(match)
			alt, ref := groups[1], groups[2]

			if isAbsoluteRef(ref) {
				return match
			}

			url, ok := resolveImage(ref)
			if !ok {
				return "*[missing image: " + ref + "]*"
			}
			if isPDF(ref) {
				return renderPDFEmbed(url, alt)
			}
			if isVideo(ref) {
				return renderVideoEmbed(url, alt)
			}
			return "![" + alt + "](" + url + ")"
		})

		line = embedPattern.ReplaceAllStringFunc(line, func(match string) string {
			groups := embedPattern.FindStringSubmatch(match)
			filename, alias := groups[1], groups[2]
			display := alias
			if display == "" {
				display = filename
			}

			url, ok := resolveImage(filename)
			if !ok {
				return "*[missing embed: " + filename + "]*"
			}

			if isPDF(filename) {
				return renderPDFEmbed(url, display)
			}
			if isVideo(filename) {
				return renderVideoEmbed(url, display)
			}
			return "![" + display + "](" + url + ")"
		})

		line = wikilinkPattern.ReplaceAllStringFunc(line, func(match string) string {
			groups := wikilinkPattern.FindStringSubmatch(match)
			name, alias := groups[1], groups[2]
			if alias != "" {
				return alias
			}
			return name
		})

		lines[i] = line
	}

	return strings.Join(lines, "\n")
}

// isAbsoluteRef reports whether ref is already directly servable (a full
// URL or a data URI) and so shouldn't be routed through resolveImage.
func isAbsoluteRef(ref string) bool {
	return strings.Contains(ref, "://") || strings.HasPrefix(ref, "data:")
}

func isPDF(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".pdf")
}

// isVideo reports whether filename is a video format the vault embeds
// (see videos/ folder) — these need a <video> tag rather than <img>, which
// can't play video at all.
func isVideo(filename string) bool {
	lower := strings.ToLower(filename)
	for _, ext := range []string{".mp4", ".webm", ".mov", ".m4v"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// renderPDFEmbed renders a PDF as an inline <object> (most desktop and
// mobile browsers show their native PDF viewer for this), with a plain link
// as fallback content for the rare viewer that renders neither — e.g. a
// handwritten note synced in from tools-browsernotes' reMarkable pipeline.
func renderPDFEmbed(url, display string) string {
	return fmt.Sprintf(
		`<object data="%s" type="application/pdf" width="100%%" height="600"><p>📄 <a href="%s">%s</a></p></object>`,
		url, url, stdhtml.EscapeString(display),
	)
}

// renderVideoEmbed renders a video as an inline <video controls> element,
// with a plain link as fallback content for a browser that can't play the
// source format.
func renderVideoEmbed(url, display string) string {
	return fmt.Sprintf(
		`<video controls preload="metadata" style="max-width:100%%"><source src="%s">🎬 <a href="%s">%s</a></video>`,
		url, url, stdhtml.EscapeString(display),
	)
}
