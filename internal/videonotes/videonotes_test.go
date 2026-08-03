package videonotes

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReferenceJSONKeysAreLowercase(t *testing.T) {
	// Regression: without json tags, encoding/json emits "Title"/"Path"
	// verbatim, which doesn't match watching-notes.js's ref.title/ref.path —
	// every reference-link click silently fell through to a real page
	// navigation instead of the intended inline preview (found live).
	data, err := json.Marshal(Reference{Title: "a title", Path: "/a/path.md"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["title"] != "a title" || got["path"] != "/a/path.md" {
		t.Fatalf(`expected lowercase "title"/"path" JSON keys, got: %s`, data)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Malcolm Gladwell: Talking to Strangers": "malcolm-gladwell-talking-to-strangers",
		"  Leading/Trailing  Spaces  ":           "leading-trailing-spaces",
		"Multiple---Hyphens!!":                   "multiple-hyphens",
		"":                                       "",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSuffix(t *testing.T) {
	got := Suffix("Malcolm Gladwell: Talking to Strangers")
	want := "-malcolm-gladwell-talking-to-strangers.md"
	if got != want {
		t.Fatalf("Suffix() = %q, want %q", got, want)
	}
}

func TestFilename(t *testing.T) {
	today := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	got := Filename(today, 80, "Malcolm Gladwell: Talking to Strangers")
	want := "2026-08-01-80-malcolm-gladwell-talking-to-strangers.md"
	if got != want {
		t.Fatalf("Filename() = %q, want %q", got, want)
	}
}

// TestFilenameSuffixMatchesSuffix documents the invariant findExisting relies
// on: a filename minted by Filename always ends with Suffix(title), so a
// suffix scan finds it regardless of which date prefix it was created with —
// see the "why not derive from StagedAt" design decision.
func TestFilenameSuffixMatchesSuffix(t *testing.T) {
	title := "Malcolm Gladwell: Talking to Strangers"
	name := Filename(time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), 999, title)
	suffix := Suffix(title)
	if len(name) < len(suffix) || name[len(name)-len(suffix):] != suffix {
		t.Fatalf("Filename() = %q does not end with Suffix() = %q", name, suffix)
	}
}

func TestDir(t *testing.T) {
	got := Dir("/DropsyncFiles/jw-mind")
	want := "/DropsyncFiles/jw-mind/Clippings/randoread-watching-it-later"
	if got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
}

// realTemplate mirrors the actual templates/video-notes.md fixture content
// in the vault (see main-randoread.md 05.02) — kept as a literal here rather
// than read from the live vault, since tests shouldn't depend on the user's
// personal Dropbox-synced filesystem.
const realTemplate = `> online link
## vault references
- [directory1-from-vault-relative / directory 2 / note-title](template/...)


---

...
`

func TestApplyTemplate(t *testing.T) {
	got := ApplyTemplate(realTemplate, "https://www.youtube.com/watch?v=Hgr1Wv8mwh8")

	want := `> https://www.youtube.com/watch?v=Hgr1Wv8mwh8
## vault references
- nothing


---

...
`
	if got != want {
		t.Fatalf("ApplyTemplate() =\n%q\nwant\n%q", got, want)
	}
}

func TestParseReferences_Empty(t *testing.T) {
	content := ApplyTemplate(realTemplate, "https://example.com")
	refs := ParseReferences(content)
	if len(refs) != 0 {
		t.Fatalf("expected no references for a fresh draft, got %+v", refs)
	}
}

func TestAppendReference_ReplacesNothingPlaceholder(t *testing.T) {
	content := ApplyTemplate(realTemplate, "https://example.com")

	got := AppendReference(content, Reference{Title: "music / piano / jazz-standards", Path: "/music/piano/jazz-standards.md"})

	refs := ParseReferences(got)
	if len(refs) != 1 {
		t.Fatalf("expected 1 reference, got %+v", refs)
	}
	if refs[0].Title != "music / piano / jazz-standards" || refs[0].Path != "/music/piano/jazz-standards.md" {
		t.Fatalf("unexpected reference: %+v", refs[0])
	}
	if got == content {
		t.Fatalf("expected content to change")
	}
}

func TestAppendReference_AppendsToExistingList(t *testing.T) {
	content := ApplyTemplate(realTemplate, "https://example.com")
	content = AppendReference(content, Reference{Title: "first", Path: "/first.md"})
	content = AppendReference(content, Reference{Title: "second", Path: "/second.md"})

	refs := ParseReferences(content)
	if len(refs) != 2 {
		t.Fatalf("expected 2 references, got %+v", refs)
	}
	if refs[0].Title != "first" || refs[1].Title != "second" {
		t.Fatalf("unexpected reference order: %+v", refs)
	}
}

func TestAppendReference_PreservesRestOfNote(t *testing.T) {
	content := ApplyTemplate(realTemplate, "https://example.com") + "some real notes here"
	got := AppendReference(content, Reference{Title: "x", Path: "/x.md"})

	if !containsLine(got, "some real notes here") {
		t.Fatalf("expected body content to survive AppendReference, got:\n%s", got)
	}
}

func containsLine(content, want string) bool {
	for _, line := range splitLines(content) {
		if line == want {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}
