// Package markdown renders Obsidian-flavored markdown (wikilink images,
// wikilinks, GFM tables/tasklists/autolinks) to HTML for the Dark Moss theme.
package markdown

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// ImageResolver maps a bare filename from an Obsidian embed (e.g.
// "![[photo.png]]") to a servable URL. It returns ok=false if the file can't
// be located, in which case a text placeholder is rendered instead.
type ImageResolver func(filename string) (url string, ok bool)

var (
	embedPattern    = regexp.MustCompile(`!\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	wikilinkPattern = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
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

		line = embedPattern.ReplaceAllStringFunc(line, func(match string) string {
			groups := embedPattern.FindStringSubmatch(match)
			filename, alias := groups[1], groups[2]
			display := alias
			if display == "" {
				display = filename
			}

			url, ok := resolveImage(filename)
			if !ok {
				return "*[missing image: " + filename + "]*"
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
