package epub

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// TestBuildRegressionRealClippingTruncation reproduces the exact bug
// reported against a live delivery: sending "Silicon Valley has a science
// fiction problem" (a Clippings/ article) to the reMarkable truncated right
// after the embedded image — everything past it silently vanished. Root
// cause: markdown.Render emits HTML5-style void elements (bare "<img ...>",
// no self-closing slash), which isn't well-formed XML; the tablet's EPUB
// reader uses a strict XHTML/XML parser that stops dead the moment it hits
// one and drops the rest of the document. Fixed by switching Build to
// markdown.RenderXHTML, which self-closes void elements.
//
// This test builds a real EPUB shaped like that article — an embedded
// image (fetched via fetchImage, same as a real Clippings image would be)
// followed by more paragraphs — then parses the actual XHTML section
// pulled back out of the zip with encoding/xml, exactly as a conformant
// EPUB reader's parser would. A truncating bug fails this the same way it
// failed on the real tablet: parsing stops at the image and a trailing
// marker string placed after it is never reached.
func TestBuildRegressionRealClippingTruncation(t *testing.T) {
	fakeJPEG := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	fetchImage := func(ref string) ([]byte, string, bool) {
		if ref == "photo.jpg" {
			return fakeJPEG, "image/jpeg", true
		}
		return nil, "", false
	}

	source := []byte(`In January 2026, Elon Musk stood before the US Secretary of Defense.

![Futuristic vehicle on a city street at night.](photo.jpg)

Photo by Nathan Rupert/Flickr

TRAILING_CONTENT_MARKER: this paragraph must survive the image.`)

	data, err := Build("Silicon Valley has a science fiction problem", source, SizeL, fetchImage)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	section := onlySection(t, data)

	// The regression: a strict XML parser must be able to walk the whole
	// section without erroring, and reach the trailing marker.
	dec := xml.NewDecoder(strings.NewReader("<root>" + section + "</root>"))
	sawTrailingMarker := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("section is not well-formed XML — this is exactly how it truncated on the real tablet: %v\nsection:\n%s", err, section)
		}
		if cd, ok := tok.(xml.CharData); ok && strings.Contains(string(cd), "TRAILING_CONTENT_MARKER") {
			sawTrailingMarker = true
		}
	}

	if !sawTrailingMarker {
		t.Fatalf("content after the image was truncated — trailing marker never reached.\nsection:\n%s", section)
	}

	if !strings.Contains(section, `<img src="`) || !strings.Contains(section, `" />`) {
		t.Errorf("expected a self-closed <img .../> tag, got:\n%s", section)
	}
}

// onlySection returns the single EPUB/xhtml/*.xhtml section Build produces.
func onlySection(t *testing.T, data []byte) string {
	t.Helper()
	for _, name := range zipNames(t, data) {
		if strings.HasPrefix(name, "EPUB/xhtml/") {
			return zipFile(t, data, name)
		}
	}
	t.Fatal("no EPUB/xhtml/ section found")
	return ""
}
