package handlers

import (
	"net/url"
	"strings"
	"testing"
	"time"

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

// vaultPathResolver is the plain path-lookup vaultFileResolver builds its
// proxy URLs on top of — also used directly by the send-to-remarkable epub
// builder, which needs the real vault path to download bytes, not a URL.
func TestVaultPathResolverResolvesBareFilename(t *testing.T) {
	lister := &fakeLister{entries: []dropbox.Entry{
		{Path: "/vault/assets/photo.jpg", Name: "photo.jpg"},
	}}
	resolve := vaultPathResolver(lister, "/vault")

	got, ok := resolve("photo.jpg")
	if !ok {
		t.Fatal("expected photo.jpg to resolve")
	}
	if got != "/vault/assets/photo.jpg" {
		t.Fatalf("resolve(%q) = %q, want %q", "photo.jpg", got, "/vault/assets/photo.jpg")
	}
}

func TestVaultPathResolverReturnsFalseForUnknownFile(t *testing.T) {
	lister := &fakeLister{entries: []dropbox.Entry{
		{Path: "/vault/assets/photo.jpg", Name: "photo.jpg"},
	}}
	resolve := vaultPathResolver(lister, "/vault")

	if _, ok := resolve("missing.jpg"); ok {
		t.Fatal("expected an unknown file to be unresolved")
	}
}

// sequenceLister lets successive ListFolder calls return different
// snapshots — standing in for a vault where a file finishes syncing to
// Dropbox in between two listings.
type sequenceLister struct {
	snapshots [][]dropbox.Entry
	calls     int
}

func (s *sequenceLister) ListFolder(path string, recursive bool) ([]dropbox.Entry, error) {
	i := s.calls
	if i >= len(s.snapshots) {
		i = len(s.snapshots) - 1
	}
	s.calls++
	return s.snapshots[i], nil
}

func TestVaultPathResolverRetriesOnMissAfterCacheInvalidation(t *testing.T) {
	// First snapshot (what's cached going in) doesn't have the file yet;
	// the second does — mimicking a reMarkable PDF that finished syncing
	// to Dropbox after the 24h vault listing cache was last built.
	underlying := &sequenceLister{snapshots: [][]dropbox.Entry{
		{{Path: "/vault/assets/other.jpg", Name: "other.jpg"}},
		{
			{Path: "/vault/assets/other.jpg", Name: "other.jpg"},
			{Path: "/vault/_remarkable/focus/note.pdf", Name: "note.pdf"},
		},
	}}
	cached := dropbox.NewCachedLister(underlying, time.Hour)
	resolve := vaultPathResolver(cached, "/vault")

	got, ok := resolve("_remarkable/focus/note.pdf")
	if !ok {
		t.Fatal("expected the miss to trigger an invalidate-and-retry that finds the newly synced file")
	}
	if want := "/vault/_remarkable/focus/note.pdf"; got != want {
		t.Fatalf("resolve(...) = %q, want %q", got, want)
	}
	if underlying.calls != 2 {
		t.Fatalf("expected exactly one retry reload (2 underlying calls total), got %d", underlying.calls)
	}
}

func TestVaultPathResolverStillMissingAfterRetryReturnsFalse(t *testing.T) {
	underlying := &sequenceLister{snapshots: [][]dropbox.Entry{
		{{Path: "/vault/assets/other.jpg", Name: "other.jpg"}},
	}}
	cached := dropbox.NewCachedLister(underlying, time.Hour)
	resolve := vaultPathResolver(cached, "/vault")

	if _, ok := resolve("missing.pdf"); ok {
		t.Fatal("expected a genuinely absent file to still be unresolved after the retry")
	}
	if underlying.calls != 2 {
		t.Fatalf("expected the retry to still happen (2 underlying calls), got %d", underlying.calls)
	}
}
