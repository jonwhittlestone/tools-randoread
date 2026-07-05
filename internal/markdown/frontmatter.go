package markdown

import "strings"

// stripFrontmatter removes a leading YAML frontmatter block (as produced by
// Obsidian Web Clipper: "---\ntitle: ...\nsource: \"https://...\"\n---\n").
// goldmark has no notion of frontmatter, so left alone it renders as a
// garbled, unclickable paragraph. If a "source" field is present, its URL is
// kept as a clickable link at the top of the body; everything else in the
// block (title, tags, author, etc.) is just dropped as noise — the app
// already shows a title of its own.
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

	body := strings.TrimLeft(strings.Join(lines[end+1:], "\n"), "\n")

	if src := frontmatterField(lines[1:end], "source"); src != "" {
		return "[🔗 View original](" + src + ")\n\n" + body
	}
	return body
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
