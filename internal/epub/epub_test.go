package epub

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// zipFile returns the decompressed contents of name within an EPUB (which
// is itself a zip archive), or "" if not found — used to inspect Build's
// output without needing a full EPUB-validating dependency.
func zipFile(t *testing.T, data []byte, name string) string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	for _, f := range r.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			var buf bytes.Buffer
			buf.ReadFrom(rc) //nolint:errcheck
			return buf.String()
		}
	}
	return ""
}

func zipNames(t *testing.T, data []byte) []string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("output is not a valid zip: %v", err)
	}
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	return names
}

func noImages(ref string) ([]byte, string, bool) {
	return nil, "", false
}

func TestBuildProducesValidEpubContainer(t *testing.T) {
	data, err := Build("Golden Son", []byte("# Hello\n\nSome text."), SizeL, noImages)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if got := zipFile(t, data, "mimetype"); got != "application/epub+zip" {
		t.Errorf("mimetype entry = %q, want %q", got, "application/epub+zip")
	}
}

func TestBuildEmbedsSectionContent(t *testing.T) {
	data, err := Build("Golden Son", []byte("# Hello\n\nSome text."), SizeL, noImages)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	names := zipNames(t, data)
	var sectionFile string
	for _, n := range names {
		if strings.HasPrefix(n, "EPUB/xhtml/") {
			sectionFile = n
			break
		}
	}
	if sectionFile == "" {
		t.Fatalf("no EPUB/xhtml/ section found in %v", names)
	}

	body := zipFile(t, data, sectionFile)
	if !strings.Contains(body, "Some text.") {
		t.Errorf("section body doesn't contain rendered markdown text: %q", body)
	}
}

func TestBuildSizePresetsProduceDifferentCSS(t *testing.T) {
	small, err := Build("t", []byte("body"), SizeS, noImages)
	if err != nil {
		t.Fatalf("Build(SizeS) error: %v", err)
	}
	large, err := Build("t", []byte("body"), SizeXL, noImages)
	if err != nil {
		t.Fatalf("Build(SizeXL) error: %v", err)
	}

	smallCSS := zipFile(t, small, "EPUB/css/style.css")
	largeCSS := zipFile(t, large, "EPUB/css/style.css")

	if smallCSS == "" || largeCSS == "" {
		t.Fatalf("expected non-empty CSS, got smallCSS=%q largeCSS=%q", smallCSS, largeCSS)
	}
	if smallCSS == largeCSS {
		t.Errorf("SizeS and SizeXL produced identical CSS: %q", smallCSS)
	}
}

func TestBuildUnknownSizeFallsBackToL(t *testing.T) {
	unknown, err := Build("t", []byte("body"), Size("bogus"), noImages)
	if err != nil {
		t.Fatalf("Build(bogus size) error: %v", err)
	}
	large, err := Build("t", []byte("body"), SizeL, noImages)
	if err != nil {
		t.Fatalf("Build(SizeL) error: %v", err)
	}

	if zipFile(t, unknown, "EPUB/css/style.css") != zipFile(t, large, "EPUB/css/style.css") {
		t.Errorf("unknown size should fall back to SizeL's CSS")
	}
}

func TestBuildEmbedsResolvedImage(t *testing.T) {
	fakeImage := []byte{0xFF, 0xD8, 0xFF, 0xE0} // fake JPEG-ish bytes, content doesn't matter
	fetch := func(ref string) ([]byte, string, bool) {
		if ref == "photo.jpg" {
			return fakeImage, "image/jpeg", true
		}
		return nil, "", false
	}

	data, err := Build("t", []byte("![[photo.jpg]]"), SizeL, fetch)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	names := zipNames(t, data)
	var imageFile string
	for _, n := range names {
		if strings.HasPrefix(n, "EPUB/images/") {
			imageFile = n
			break
		}
	}
	if imageFile == "" {
		t.Fatalf("no embedded image found in %v", names)
	}

	got := zipFile(t, data, imageFile)
	if got != string(fakeImage) {
		t.Errorf("embedded image bytes = %q, want %q", got, string(fakeImage))
	}
}

func TestBuildStripsQueryStringFromEmbeddedImageFilename(t *testing.T) {
	// Real Clippings images come from URLs like
	// "https://images.aeonmedia.co/photo.jpg?width=3840&quality=75" — the
	// internal EPUB filename shouldn't carry that query string along
	// (ugly, and "?"/"&" in a zip entry name is asking for trouble with
	// some tools even where it technically works).
	fetch := func(ref string) ([]byte, string, bool) {
		return []byte{0xFF, 0xD8}, "image/jpeg", true
	}

	data, err := Build("t", []byte("![x](https://example.com/path/photo.jpg?width=3840&quality=75)"), SizeL, fetch)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	for _, n := range zipNames(t, data) {
		if strings.HasPrefix(n, "EPUB/images/") && strings.ContainsAny(n, "?&") {
			t.Errorf("embedded image filename still contains the query string: %q", n)
		}
	}
}

func TestBuildUnresolvedImageDoesNotError(t *testing.T) {
	// Mirrors markdown.Render's existing placeholder behavior for
	// unresolved embeds — Build shouldn't fail the whole conversion over
	// one missing image.
	_, err := Build("t", []byte("![[missing.jpg]]"), SizeL, noImages)
	if err != nil {
		t.Fatalf("Build() with unresolved image should not error, got: %v", err)
	}
}
