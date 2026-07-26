package markdown

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// assertWellFormedXML fails t if wrapping html in a root element doesn't
// parse as well-formed XML — mirrors what a strict EPUB reader's XHTML
// parser does. A bare HTML5 void element like <img src="..."> (no
// self-closing slash) is exactly the kind of thing that fails this and
// silently truncates everything parsed after it.
func assertWellFormedXML(t *testing.T, html string) {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader("<root>" + html + "</root>"))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v\nhtml: %s", err, html)
		}
	}
}

func TestRenderXHTMLSelfClosesImageTags(t *testing.T) {
	// Regression: this exact shape (absolute URL image, plain markdown
	// syntax, content after it) truncated a real delivered EPUB — the
	// tablet's XHTML parser hit the non-self-closed <img> and stopped
	// rendering everything past it. See internal/epub's integration test
	// for the full pipeline.
	resolve := func(ref string) (string, bool) { return "../images/cat.jpg", true }
	html := RenderXHTML([]byte("![a cat](https://example.com/cat.png?width=3840&quality=75)\n\nTrailing text."), resolve)

	if !strings.Contains(html, `<img src="../images/cat.jpg" alt="a cat" />`) {
		t.Fatalf("expected a self-closed <img />, got: %s", html)
	}
	if !strings.Contains(html, "Trailing text.") {
		t.Fatalf("expected content after the image to survive, got: %s", html)
	}
	assertWellFormedXML(t, html)
}

func TestRenderXHTMLSelfClosesThematicBreak(t *testing.T) {
	html := RenderXHTML([]byte("above\n\n---\n\nbelow"), resolveNone)
	if !strings.Contains(html, "<hr />") {
		t.Fatalf("expected a self-closed <hr />, got: %s", html)
	}
	assertWellFormedXML(t, html)
}

func TestRenderXHTMLResolvesEmbedsLikeRender(t *testing.T) {
	resolve := func(ref string) (string, bool) {
		if ref == "photo.jpg" {
			return "images/photo.jpg", true
		}
		return "", false
	}
	html := RenderXHTML([]byte("![[photo.jpg]]"), resolve)
	if !strings.Contains(html, `src="images/photo.jpg"`) {
		t.Fatalf("expected the embed to resolve, got: %s", html)
	}
	assertWellFormedXML(t, html)
}

func TestRenderXHTMLResolvesAbsoluteImageURLs(t *testing.T) {
	// Regression: real Clippings articles almost always reference images by
	// absolute remote URL (scraped from the source website), not a vault-
	// relative path. Render (web) correctly leaves those alone — the
	// browser fetches them directly. But an EPUB has no live network access
	// once it's on the tablet, so RenderXHTML must route absolute URLs
	// through the resolver too, or every Clippings image silently fails to
	// embed (shows as a broken image, even though nothing truncates).
	const remoteURL = "https://images.aeonmedia.co/photo.jpg?width=3840&quality=75"
	resolve := func(ref string) (string, bool) {
		if ref == remoteURL {
			return "../images/photo.jpg", true
		}
		return "", false
	}

	html := RenderXHTML([]byte("![a photo]("+remoteURL+")"), resolve)
	if !strings.Contains(html, `src="../images/photo.jpg"`) {
		t.Fatalf("expected the absolute URL to be routed through the resolver, got: %s", html)
	}
}

func TestRenderStandardMarkdownImageAbsoluteURLStillNotRerouted(t *testing.T) {
	// Pin the existing web-facing behavior: Render must NOT call the
	// resolver for absolute URLs — introducing RenderXHTML's opposite
	// behavior must not leak into Render.
	called := false
	resolve := func(ref string) (string, bool) {
		called = true
		return "should-not-be-used", true
	}

	Render([]byte("![a photo](https://example.com/cat.png)"), resolve)
	if called {
		t.Fatal("Render must not call the resolver for absolute image URLs")
	}
}

func TestRenderXHTMLDoesNotResolveDataURIImages(t *testing.T) {
	// A data: URI is already self-contained — nothing to fetch/embed.
	called := false
	resolve := func(ref string) (string, bool) {
		called = true
		return "", false
	}

	html := RenderXHTML([]byte("![x](data:image/png;base64,AAAA)"), resolve)
	if called {
		t.Fatal("RenderXHTML must not call the resolver for data: URIs")
	}
	if !strings.Contains(html, `src="data:image/png;base64,AAAA"`) {
		t.Fatalf("expected the data URI to pass through untouched, got: %s", html)
	}
}

func TestRenderPlainHTML5DoesNotSelfClose(t *testing.T) {
	// Pin the existing web-facing behavior: Render (used by the browser UI)
	// must stay HTML5-style, unaffected by RenderXHTML's introduction.
	html := Render([]byte("![a cat](https://example.com/cat.png)"), resolveNone)
	if strings.Contains(html, "/>") {
		t.Fatalf("Render should not self-close void elements (HTML5, browser-facing), got: %s", html)
	}
}
