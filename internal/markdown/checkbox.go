package markdown

import (
	"fmt"
	"strings"
)

// Table-cell checkboxes: a GFM table cell whose whole content is "[ ]" or
// "[x]"/"[X]" (e.g. a "✓" column in a parts list). GFM only has task-list
// checkboxes on list items, and Obsidian can't tick those inside tables, so
// the markdown file itself holds the state as plain "[ ]"/"[x]" text — which
// keeps it visible and hand-editable in Obsidian, and shared across every
// device reading the same Dropbox file (see main-randoread.md 06.02).
//
// Boxes are numbered 0, 1, 2… in document order. Render stamps that number
// on each <input>, and SetTableCheckbox flips the box with the same number,
// so both must walk the source identically: table rows only (lines starting
// with "|"), skipping fenced code blocks by the same "```" rule preprocess
// uses. Frontmatter is already stripped before Render sees the source, so
// SetTableCheckbox skips it too.

// isTableRow reports whether line is a GFM table row with leading pipes.
func isTableRow(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "|")
}

// checkboxCellState reports whether cell (the text between two pipes) is a
// whole-cell checkbox, and if so whether it's checked.
func checkboxCellState(cell string) (isCheckbox, checked bool) {
	switch strings.TrimSpace(cell) {
	case "[ ]":
		return true, false
	case "[x]", "[X]":
		return true, true
	}
	return false, false
}

// replaceTableCheckboxes rewrites every whole-cell checkbox in a table row
// line via replace, which receives the running index, checked state and
// the cell's original marker text ("[ ]", "[x]" or "[X]").
// Splitting and re-joining on "|" is lossless, so every other byte of the
// line is untouched.
func replaceTableCheckboxes(line string, next *int, replace func(index int, checked bool, marker string) string) string {
	cells := strings.Split(line, "|")
	for i, cell := range cells {
		isBox, checked := checkboxCellState(cell)
		if !isBox {
			continue
		}
		trimmed := strings.TrimSpace(cell)
		start := strings.Index(cell, trimmed)
		cells[i] = cell[:start] + replace(*next, checked, trimmed) + cell[start+len(trimmed):]
		*next++
	}
	return strings.Join(cells, "|")
}

// renderTableCheckbox is the preprocess replacement for one box: a
// disabled <input> for the browser (watching-notes.js enables the ones it
// can persist), or a ballot-box character for XHTML/EPUB output.
func renderTableCheckbox(index int, checked, xhtml bool) string {
	if xhtml {
		if checked {
			return "☑"
		}
		return "☐"
	}
	attrs := " disabled"
	if checked {
		attrs += " checked"
	}
	return fmt.Sprintf(`<input type="checkbox" class="md-table-checkbox" data-checkbox-index="%d"%s>`, index, attrs)
}

// SetTableCheckbox returns source with its index-th table-cell checkbox set
// to checked, preserving every other byte. It errors if there's no such box.
func SetTableCheckbox(source string, index int, checked bool) (string, error) {
	if index < 0 {
		return "", fmt.Errorf("checkbox index %d out of range", index)
	}
	lines := strings.Split(source, "\n")
	start := frontmatterEnd(lines)
	inFence := false
	next := 0
	found := false

	for i := start; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence || !isTableRow(line) {
			continue
		}
		lines[i] = replaceTableCheckboxes(line, &next, func(n int, _ bool, marker string) string {
			if n != index {
				return marker
			}
			found = true
			if checked {
				return "[x]"
			}
			return "[ ]"
		})
	}
	if !found {
		return "", fmt.Errorf("checkbox index %d out of range (%d boxes)", index, next)
	}
	return strings.Join(lines, "\n"), nil
}
