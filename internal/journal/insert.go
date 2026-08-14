// Package journal splices a drafted line into today's daily note under a
// given heading — the write side of the "Send to oh-two" journal input
// (see main-randoread.md / the 26-nanoclaw vault project's main.md §05.05).
// NanoClaw (internal/journaldraft) only proposes heading + line; this
// package does the actual insertion, and handlers/journal_apply.go does
// the actual Dropbox write — the two are kept separate so this logic is
// testable without any network dependency.
package journal

import (
	"fmt"
	"regexp"
	"strings"
)

var headingPattern = regexp.MustCompile(`^(#{1,6})\s`)

// headingLevel returns the number of leading '#'s if line is a heading
// (1-6), or 0 if it isn't.
func headingLevel(line string) int {
	m := headingPattern.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return 0
	}
	return len(m[1])
}

// isThematicBreak reports whether line is a Markdown horizontal rule —
// this vault's daily note template uses both `---` and `___` to separate
// top-level sections (see templates/Daily.md), so both count as a section
// boundary the same way a same-or-shallower heading does.
func isThematicBreak(line string) bool {
	t := strings.TrimSpace(line)
	return t == "---" || t == "___"
}

// InsertUnderHeading returns raw with line inserted as a new final entry
// of the section introduced by heading — heading must match a line in raw
// exactly after trimming (NanoClaw returns headings copied verbatim from
// groups/journal-draft/CLAUDE.md's fixed list specifically so this match
// succeeds).
//
// A section runs from its heading to the next heading of the same or
// shallower level, the next thematic break (`---`/`___`), or end of file —
// whichever comes first. line is inserted immediately after the section's
// last non-blank line, preserving whatever blank-line spacing already
// separates the section from what follows, rather than appended right
// after the heading (which would put new entries in reverse-chronological
// order instead of alongside existing content).
//
// Known limitation (documented, not fixed here — see main.md §05.05):
// placeholder content the template ships with (a bare "..." or "- ") is
// left in place rather than replaced, so the first entry filed under a
// section sits alongside its placeholder instead of overwriting it.
func InsertUnderHeading(raw, heading, line string) (string, error) {
	lines := strings.Split(raw, "\n")

	headingIdx := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == heading {
			headingIdx = i
			break
		}
	}
	if headingIdx == -1 {
		return "", fmt.Errorf("heading %q not found in note", heading)
	}
	level := headingLevel(lines[headingIdx])

	boundary := len(lines)
	for i := headingIdx + 1; i < len(lines); i++ {
		if isThematicBreak(lines[i]) {
			boundary = i
			break
		}
		if lvl := headingLevel(lines[i]); lvl > 0 && lvl <= level {
			boundary = i
			break
		}
	}

	// Walk the boundary back past trailing blank lines so line lands right
	// after the section's actual last content line, not after a run of
	// blank lines that exist purely to space the section from the next one.
	insertAt := boundary
	for insertAt > headingIdx+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:insertAt]...)
	out = append(out, line)
	out = append(out, lines[insertAt:]...)
	return strings.Join(out, "\n"), nil
}
