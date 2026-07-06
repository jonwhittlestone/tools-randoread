package markdown

import (
	"html"
	"net/url"
	"strings"
)

// stripFrontmatter removes a leading YAML frontmatter block (as produced by
// Obsidian Web Clipper: "---\ntitle: ...\nsource: \"https://...\"\n---\n").
// goldmark has no notion of frontmatter, so left alone it renders as a
// garbled, unclickable paragraph. If present, "source" becomes a clickable
// link followed by its base domain (so it's clear which site it leads to
// before clicking) and "title" becomes a heading below that — everything
// else in the block (tags, author, etc.) is just dropped as noise.
func stripFrontmatter(source string) string {
	lines := strings.Split(source, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return source
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return source // no closing delimiter — not actually frontmatter
	}

	fmLines := lines[1:end]
	body := strings.TrimLeft(strings.Join(lines[end+1:], "\n"), "\n")

	var header strings.Builder
	if src := frontmatterField(fmLines, "source"); src != "" {
		header.WriteString("[🔗 View original](" + src + ")" + sourceDomainSuffix(src) + "\n\n")
	}
	if title := frontmatterField(fmLines, "title"); title != "" {
		// Raw HTML (not markdown "# "+title) so the title can't be broken by
		// markdown special characters it happens to contain.
		header.WriteString("<h1>" + html.EscapeString(title) + "</h1>\n\n")
	}

	return header.String() + body
}

// sourceDomainSuffix returns " | [example.com](src)", or "" if src isn't a
// parseable URL with a host.
func sourceDomainSuffix(src string) string {
	u, err := url.Parse(src)
	if err != nil || u.Host == "" {
		return ""
	}
	return " | [" + u.Host + "](" + src + ")"
}

func frontmatterField(lines []string, key string) string {
	prefix := key + ":"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, prefix); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}
