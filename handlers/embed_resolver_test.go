package handlers

import (
	"net/url"
	"strings"
	"testing"

	"github.com/jonwhittlestone/tools-randoread/internal/dropbox"
)

func TestVaultFileResolverResolvesBareFilename(t *testing.T) {
	lister := &fakeLister{entries: []dropbox.Entry{
		{Path: "/vault/assets/photo.jpg", Name: "photo.jpg"},
	}}
	resolve := vaultFileResolver(lister, "/vault", "tok")

	got, ok := resolve("photo.jpg")
	if !ok {
		t.Fatal("expected photo.jpg to resolve")
	}
	if !strings.Contains(got, url.QueryEscape("/vault/assets/photo.jpg")) {
		t.Fatalf("expected the resolved URL to reference the real path, got %q", got)
	}
}

func TestVaultFileResolverResolvesVaultRelativePathEmbed(t *testing.T) {
	// Obsidian qualifies an embed with a partial path (rather than a bare
	// filename) when the bare name alone is ambiguous — e.g. reMarkable's
	// daily "focus" PDF exports reuse the same filename pattern across
	// different date subfolders. A resolver keyed only on bare filenames
	// would silently pick the wrong file (or none at all).
	lister := &fakeLister{entries: []dropbox.Entry{
		{Path: "/vault/_remarkable/day-one/note.pdf", Name: "note.pdf"},
		{Path: "/vault/_remarkable/day-two/note.pdf", Name: "note.pdf"},
	}}
	resolve := vaultFileResolver(lister, "/vault", "tok")

	got, ok := resolve("_remarkable/day-two/note.pdf")
	if !ok {
		t.Fatal("expected the path-qualified embed to resolve")
	}
	wantPath := url.QueryEscape("/vault/_remarkable/day-two/note.pdf")
	if !strings.Contains(got, wantPath) {
		t.Fatalf("expected the disambiguated path, got %q", got)
	}
}

func TestVaultFileResolverReturnsFalseForUnknownFile(t *testing.T) {
	lister := &fakeLister{entries: []dropbox.Entry{
		{Path: "/vault/assets/photo.jpg", Name: "photo.jpg"},
	}}
	resolve := vaultFileResolver(lister, "/vault", "tok")

	if _, ok := resolve("missing.jpg"); ok {
		t.Fatal("expected an unknown file to be unresolved")
	}
}
