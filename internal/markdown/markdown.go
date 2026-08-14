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
	anchorHrefPattern    = regexp.MustCompile(`<a href="`)
	// Matches both youtube.com/watch?v=ID and youtu.be/ID (the form mobile
	// "Share" sheets produce, typically with a trailing ?si=... tracking
	// param) — group 1 or 2 holds the 11-character video ID depending on
	// which form matched.
	youtubeURLPattern = regexp.MustCompile(`https?://(?:www\.)?(?:youtube\.com/watch\?v=([A-Za-z0-9_-]{11})\S*|youtu\.be/([A-Za-z0-9_-]{11})\S*)`)
)

// openLinksInNewTab adds target="_blank" (plus the standard rel guard
// against the new tab getting a handle back on window.opener) to every
// rendered link — both explicit "[text](url)" links and GFM's bare-URL
// autolinking. Without this, clicking a link inside a note (e.g. a "vault
// references" entry, or a plain pasted URL in a clipping) navigates the
// whole single-page app away in the same tab, losing all client-side state.
// Post-processed on the final HTML string rather than via a goldmark AST
// transform — goldmark's default link renderer doesn't emit arbitrary node
// attributes, and every <a> goldmark ever emits starts with `<a href="`
// (href always comes first), so this is exact rather than a heuristic.
func openLinksInNewTab(html string) string {
	return anchorHrefPattern.ReplaceAllString(html, `<a target="_blank" rel="noopener noreferrer" href="`)
}

var renderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithUnsafe()),
)

// xhtmlRenderer additionally self-closes void elements (<img … />,
// <hr/>, <br/>) — required by RenderXHTML, see there for why.
var xhtmlRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(html.WithUnsafe(), html.WithXHTML()),
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
	return openLinksInNewTab(render(renderer, source, resolveImage, false))
}

// RenderXHTML behaves like Render, with two differences needed for
// embedding directly into an EPUB (see internal/epub):
//
//  1. Void elements (<img>, <hr>, <br>) are self-closed. Render's plain
//     HTML5 output (no self-closing slash) is fine for a browser's lenient
//     HTML parser, but EPUB sections must be well-formed XML — a bare
//     "<img ...>" makes some readers' XML parsers (confirmed: the
//     reMarkable's own EPUB renderer) fail right there and silently drop
//     everything in the document after it.
//  2. Absolute (http/https) image URLs are also routed through
//     resolveImage, not left as-is. Render correctly leaves them alone —
//     the browser fetches them directly — but an EPUB has no live network
//     access once it's on the tablet, and real Clippings articles almost
//     always reference images by absolute URL (scraped from the source
//     site), so leaving those unresolved would mean Clippings images never
//     actually embed. data: URIs are still left untouched either way —
//     already self-contained, nothing to fetch.
func RenderXHTML(source []byte, resolveImage ImageResolver) string {
	return render(xhtmlRenderer, source, resolveImage, true)
}

func render(r goldmark.Markdown, source []byte, resolveImage ImageResolver, resolveAbsoluteImages bool) string {
	body := stripFrontmatter(string(source))
	preprocessed := preprocess(body, resolveImage, resolveAbsoluteImages)

	var buf bytes.Buffer
	if err := r.Convert([]byte(preprocessed), &buf); err != nil {
		return "<p>failed to render note</p>"
	}
	return buf.String()
}

func preprocess(source string, resolveImage ImageResolver, resolveAbsoluteImages bool) string {
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

		// Skipped for XHTML/EPUB rendering (resolveAbsoluteImages is also
		// RenderXHTML's signal, see its doc comment) — a live YouTube
		// iframe is dead weight in an EPUB with no network access on the
		// tablet; a plain autolinked URL (GFM's default) is strictly more
		// useful there, so this step is left to only Render's browser path.
		if !resolveAbsoluteImages {
			original := line
			line = youtubeURLPattern.ReplaceAllStringFunc(line, func(match string) string {
				// Respect an explicit [label](url) the author already wrote
				// around this URL rather than overriding it with an embed.
				if idx := strings.Index(original, match); idx >= 2 && original[idx-2:idx] == "](" {
					return match
				}
				id := youtubeVideoID(match)
				if id == "" {
					return match
				}
				return renderYouTubeEmbed(id, match)
			})
		}

		// Runs before embedPattern below, which produces its own
		// "![alt](url)" output — matching that here too would double-process
		// an already-resolved URL.
		line = standardImagePattern.ReplaceAllStringFunc(line, func(match string) string {
			groups := standardImagePattern.FindStringSubmatch(match)
			alt, ref := groups[1], groups[2]

			if isAbsoluteRef(ref) && !(resolveAbsoluteImages && isHTTPRef(ref)) {
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
// URL or a data URI) and so shouldn't be routed through resolveImage by
// default (Render never does; RenderXHTML makes an exception for http(s)
// URLs specifically — see isHTTPRef).
func isAbsoluteRef(ref string) bool {
	return strings.Contains(ref, "://") || strings.HasPrefix(ref, "data:")
}

// isHTTPRef reports whether ref is an http(s) URL — as opposed to a data:
// URI, which is already self-contained and never needs fetching.
func isHTTPRef(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
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
// source format. Wrapped with a −/+ toggle (see static/app.js) so a tall
// video doesn't force scrolling past it to reach the rest of the note —
// expanded by default, collapsible on click.
func renderVideoEmbed(url, display string) string {
	return fmt.Sprintf(
		`<div class="video-embed"><button type="button" class="video-toggle" aria-expanded="true" aria-label="Collapse video">−</button><video controls preload="metadata"><source src="%s">🎬 <a href="%s">%s</a></video></div>`,
		url, url, stdhtml.EscapeString(display),
	)
}

// youtubeVideoID extracts the 11-character video ID from a URL that
// already matched youtubeURLPattern (whichever of its two alternatives —
// youtube.com or youtu.be — actually matched), or "" if somehow neither
// capture group is populated.
func youtubeVideoID(url string) string {
	m := youtubeURLPattern.FindStringSubmatch(url)
	if m == nil {
		return ""
	}
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}

// renderYouTubeEmbed renders a bare YouTube URL as a click-to-load embed:
// a thumbnail (YouTube's public img.youtube.com CDN — no API key needed)
// with a play button, swapped for the real <iframe> only on click (see
// static/app.js's delegated .youtube-embed-play listener) rather than
// loading the iframe — and whatever tracking/cookies it sets — up front
// for every YouTube link a note happens to contain. Includes a plain link
// fallback alongside the thumbnail for accessibility/no-JS.
func renderYouTubeEmbed(id, url string) string {
	return fmt.Sprintf(
		`<div class="youtube-embed" data-video-id="%s"><button type="button" class="youtube-embed-play" aria-label="Play video">▶</button><img src="https://img.youtube.com/vi/%s/hqdefault.jpg" alt="YouTube video thumbnail" loading="lazy"><a class="youtube-embed-link" href="%s">Watch on YouTube</a></div>`,
		id, id, url,
	)
}
