// Package epub renders Obsidian-flavored markdown (via internal/markdown)
// into a self-contained, offline-readable EPUB — referenced images are
// embedded directly rather than left as remote URLs, since a reMarkable
// tablet reading an EPUB has no access to randoread's auth-gated
// /api/asset proxy.
package epub

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"path/filepath"

	goepub "github.com/bmaupin/go-epub"

	"github.com/jonwhittlestone/tools-randoread/internal/markdown"
)

// Size is a font-size/line-height preset for the generated EPUB's body
// text. SizeL is the default: comfortable line height, generous font size,
// per Jon (motivated less by squinting at Clippings on the tablet).
type Size string

const (
	SizeS  Size = "S"
	SizeM  Size = "M"
	SizeL  Size = "L"
	SizeXL Size = "XL"
)

type sizeStyle struct {
	FontSize   string
	LineHeight string
}

var sizeStyles = map[Size]sizeStyle{
	SizeS:  {FontSize: "110%", LineHeight: "1.5"},
	SizeM:  {FontSize: "130%", LineHeight: "1.7"},
	SizeL:  {FontSize: "150%", LineHeight: "1.9"},
	SizeXL: {FontSize: "180%", LineHeight: "2.1"},
}

func styleFor(size Size) sizeStyle {
	if s, ok := sizeStyles[size]; ok {
		return s
	}
	return sizeStyles[SizeL]
}

// ImageFetcher resolves an Obsidian embed ref (e.g. "photo.jpg", as passed
// to markdown.ImageResolver) to raw image bytes and a MIME content type.
// ok=false renders the same unresolved-embed placeholder markdown.Render
// uses elsewhere, rather than failing the whole conversion.
type ImageFetcher func(ref string) (data []byte, contentType string, ok bool)

// Build renders markdown source into a single-section EPUB titled title,
// with body text styled per size and referenced images embedded via
// fetchImage.
func Build(title string, source []byte, size Size, fetchImage ImageFetcher) ([]byte, error) {
	e := goepub.NewEpub(title)
	e.SetTitle(title)

	style := styleFor(size)
	css := fmt.Sprintf("body { font-size: %s; line-height: %s; }", style.FontSize, style.LineHeight)
	cssPath, err := e.AddCSS("data:text/css,"+url.PathEscape(css), "style.css")
	if err != nil {
		return nil, fmt.Errorf("add css: %w", err)
	}

	html := markdown.Render(source, imageResolver(e, fetchImage))

	if _, err := e.AddSection(html, title, "", cssPath); err != nil {
		return nil, fmt.Errorf("add section: %w", err)
	}

	var buf bytes.Buffer
	if _, err := e.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("write epub: %w", err)
	}
	return buf.Bytes(), nil
}

// imageResolver adapts an ImageFetcher into a markdown.ImageResolver:
// fetched bytes are embedded into e as a data URL (AddImage accepts a URL,
// a local file path, or an embedded data URL) and the returned internal
// EPUB path becomes the rendered <img src>. Each ref is only added once —
// a clipping referencing the same image twice would otherwise trip
// go-epub's FilenameAlreadyUsedError on the second AddImage call.
func imageResolver(e *goepub.Epub, fetchImage ImageFetcher) markdown.ImageResolver {
	added := map[string]string{}

	return func(ref string) (string, bool) {
		if internalPath, ok := added[ref]; ok {
			return internalPath, true
		}

		data, contentType, ok := fetchImage(ref)
		if !ok {
			return "", false
		}

		dataURL := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
		internalPath, err := e.AddImage(dataURL, filepath.Base(ref))
		if err != nil {
			return "", false
		}

		added[ref] = internalPath
		return internalPath, true
	}
}
