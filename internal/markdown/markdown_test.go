package markdown

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resolveNone(string) (string, bool) { return "", false }

func TestRenderBasicHeadingAndParagraph(t *testing.T) {
	html := Render([]byte("## Hello\n\nworld"), resolveNone)
	if !strings.Contains(html, "<h2>Hello</h2>") {
		t.Fatalf("expected an <h2>, got: %s", html)
	}
	if !strings.Contains(html, "<p>world</p>") {
		t.Fatalf("expected a <p>, got: %s", html)
	}
}

func TestRenderStandardMarkdownLinkIsClickable(t *testing.T) {
	html := Render([]byte("[Sunweb](https://www.sunweb.co.uk/ski)"), resolveNone)
	if !strings.Contains(html, `<a href="https://www.sunweb.co.uk/ski">Sunweb</a>`) {
		t.Fatalf("expected a rendered anchor, got: %s", html)
	}
}

func TestRenderLinkifiesBareURL(t *testing.T) {
	html := Render([]byte("https://www.sunweb.co.uk/ski/france"), resolveNone)
	if !strings.Contains(html, `<a href="https://www.sunweb.co.uk/ski/france">`) {
		t.Fatalf("expected the bare URL to be autolinked, got: %s", html)
	}
}

func TestRenderStandardMarkdownImageIsAbsolute(t *testing.T) {
	html := Render([]byte("![a cat](https://example.com/cat.png)"), resolveNone)
	if !strings.Contains(html, `<img src="https://example.com/cat.png" alt="a cat">`) {
		t.Fatalf("expected the absolute image to render as-is, got: %s", html)
	}
}

func TestRenderResolvesRelativeObsidianImageEmbed(t *testing.T) {
	resolve := func(filename string) (string, bool) {
		if filename == "Pasted image 20260122173316.png" {
			return "api/asset?path=/assets/Pasted%20image%2020260122173316.png", true
		}
		return "", false
	}

	html := Render([]byte("![[Pasted image 20260122173316.png]]"), resolve)
	if !strings.Contains(html, `<img src="api/asset?path=/assets/Pasted%20image%2020260122173316.png"`) {
		t.Fatalf("expected the wikilink embed to resolve to an <img>, got: %s", html)
	}
}

func TestRenderResolvesRelativePDFEmbedAsObjectTag(t *testing.T) {
	resolve := func(filename string) (string, bool) {
		if filename == "handwritten-note.pdf" {
			return "api/asset?path=/assets/handwritten-note.pdf", true
		}
		return "", false
	}

	html := Render([]byte("![[handwritten-note.pdf]]"), resolve)
	if !strings.Contains(html, `<object data="api/asset?path=/assets/handwritten-note.pdf" type="application/pdf"`) {
		t.Fatalf("expected the PDF embed to resolve to an <object> tag, got: %s", html)
	}
	if !strings.Contains(html, `<a href="api/asset?path=/assets/handwritten-note.pdf">`) {
		t.Fatalf("expected a fallback link for viewers that can't render the <object>, got: %s", html)
	}
	if strings.Contains(html, "<img") {
		t.Fatalf("a PDF embed should never render as <img>, got: %s", html)
	}
}

func TestRenderShowsPlaceholderForUnresolvedPDFEmbed(t *testing.T) {
	html := Render([]byte("![[missing.pdf]]"), resolveNone)
	if strings.Contains(html, "<object") {
		t.Fatalf("expected no <object> for an unresolved embed, got: %s", html)
	}
	if !strings.Contains(html, "missing.pdf") {
		t.Fatalf("expected the filename to still be visible as a placeholder, got: %s", html)
	}
}

func TestRenderShowsPlaceholderForUnresolvedImageEmbed(t *testing.T) {
	html := Render([]byte("![[missing.png]]"), resolveNone)
	if strings.Contains(html, "<img") {
		t.Fatalf("expected no <img> for an unresolved embed, got: %s", html)
	}
	if !strings.Contains(html, "missing.png") {
		t.Fatalf("expected the filename to still be visible as a placeholder, got: %s", html)
	}
}

func TestRenderWikilinkIsPlainTextNotNavigable(t *testing.T) {
	html := Render([]byte("[[Some Note]] and [[Some Note|an alias]]"), resolveNone)
	if strings.Contains(html, "<a ") {
		t.Fatalf("expected wikilinks to not become anchors, got: %s", html)
	}
	if !strings.Contains(html, "Some Note") || !strings.Contains(html, "an alias") {
		t.Fatalf("expected wikilink text to still be visible, got: %s", html)
	}
}

func TestRenderLeavesFencedCodeBlockLiteral(t *testing.T) {
	src := "```\n### not a real heading\n**not real bold**\n```"
	html := Render([]byte(src), resolveNone)
	if strings.Contains(html, "<h3>") || strings.Contains(html, "<strong>") {
		t.Fatalf("expected code fence contents to stay literal, got: %s", html)
	}
	if !strings.Contains(html, "<pre><code>") {
		t.Fatalf("expected a <pre><code> block, got: %s", html)
	}
}

func TestRenderTable(t *testing.T) {
	src := "| A | B |\n| - | - |\n| 1 | 2 |"
	html := Render([]byte(src), resolveNone)
	if !strings.Contains(html, "<table>") || !strings.Contains(html, "<td>1</td>") {
		t.Fatalf("expected a rendered table, got: %s", html)
	}
}

func TestRenderTaskList(t *testing.T) {
	src := "- [ ] todo\n- [x] done"
	html := Render([]byte(src), resolveNone)
	if !strings.Contains(html, `type="checkbox"`) {
		t.Fatalf("expected checkboxes for task list items, got: %s", html)
	}
	if !strings.Contains(html, "checked") {
		t.Fatalf("expected the completed item to be marked checked, got: %s", html)
	}
}

func TestRenderStripsFrontmatterAndLinkifiesSource(t *testing.T) {
	src := "---\n" +
		`title: "How to ask for help"` + "\n" +
		`source: "https://example.com/article?utm_source=x&utm_medium=y"` + "\n" +
		"author:\n" +
		"tags:\n" +
		"  - \"clippings\"\n" +
		"---\n" +
		"Body content here."

	html := Render([]byte(src), resolveNone)

	if strings.Contains(html, "title:") || strings.Contains(html, "tags:") {
		t.Fatalf("expected raw frontmatter fields to be stripped, got: %s", html)
	}
	if !strings.Contains(html, `<a href="https://example.com/article?utm_source=x&amp;utm_medium=y">`) {
		t.Fatalf("expected the source URL to be a clickable link, got: %s", html)
	}
	if !strings.Contains(html, `View original</a> | <a href="https://example.com/article?utm_source=x&amp;utm_medium=y">example.com</a>`) {
		t.Fatalf("expected the source's base URL to be shown after a pipe, got: %s", html)
	}
	if !strings.Contains(html, "<h1>How to ask for help</h1>") {
		t.Fatalf("expected the article's title as a heading, got: %s", html)
	}
	if !strings.Contains(html, "Body content here.") {
		t.Fatalf("expected the body to still render, got: %s", html)
	}
	if strings.Index(html, "View original") > strings.Index(html, "<h1>") {
		t.Fatalf("expected the title heading to come after the View original line, got: %s", html)
	}
}

func TestRenderFrontmatterTitleShownWithoutSource(t *testing.T) {
	src := "---\ntitle: \"No source here\"\n---\nBody content."
	html := Render([]byte(src), resolveNone)

	if strings.Contains(html, "title:") {
		t.Fatalf("expected the raw frontmatter field to be stripped, got: %s", html)
	}
	if !strings.Contains(html, "<h1>No source here</h1>") {
		t.Fatalf("expected the title heading even without a source link, got: %s", html)
	}
	if !strings.Contains(html, "Body content.") {
		t.Fatalf("expected the body to still render, got: %s", html)
	}
}

func TestRenderFrontmatterWithNeitherTitleNorSourceIsJustStripped(t *testing.T) {
	src := "---\nauthor: \"Someone\"\n---\nBody content."
	html := Render([]byte(src), resolveNone)

	if strings.Contains(html, "<h1>") || strings.Contains(html, "View original") {
		t.Fatalf("expected no additions when neither title nor source is present, got: %s", html)
	}
	if !strings.Contains(html, "Body content.") {
		t.Fatalf("expected the body to still render, got: %s", html)
	}
}

func TestRenderWithoutFrontmatterIsUnaffected(t *testing.T) {
	html := Render([]byte("## Hello\n\nworld"), resolveNone)
	if !strings.Contains(html, "<h2>Hello</h2>") {
		t.Fatalf("expected normal rendering when there's no frontmatter, got: %s", html)
	}
}

// TestRenderFixtureDoesNotPanic exercises everything above together against
// a real trimmed note (see internal/markdown/testdata/ski-trip.md).
func TestRenderFixtureDoesNotPanic(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "ski-trip.md"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	resolve := func(filename string) (string, bool) {
		return "api/asset?path=/assets/" + url.QueryEscape(filename), true
	}

	html := Render(src, resolve)

	for _, want := range []string{"<img", "<table>", `type="checkbox"`, "<pre><code>", "sunweb.co.uk"} {
		if !strings.Contains(html, want) {
			t.Errorf("expected fixture render to contain %q, got: %s", want, html)
		}
	}
}
