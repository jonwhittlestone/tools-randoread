package markdown

import (
	"strings"
	"testing"
)

const checklistTable = `| ✓ | Piece |
|---|---|
| [ ] | A Side walls |
| [x] | B Base |
|  [X]  | C Upper back |
`

func TestRenderTableCellCheckboxes(t *testing.T) {
	html := Render([]byte(checklistTable), resolveNone)

	for _, want := range []string{
		`<input type="checkbox" class="md-table-checkbox" data-checkbox-index="0" disabled>`,
		`<input type="checkbox" class="md-table-checkbox" data-checkbox-index="1" disabled checked>`,
		`<input type="checkbox" class="md-table-checkbox" data-checkbox-index="2" disabled checked>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("expected %s in:\n%s", want, html)
		}
	}
	if strings.Contains(html, "[ ]") || strings.Contains(html, "[x]") {
		t.Errorf("raw checkbox markers should not survive rendering:\n%s", html)
	}
}

// Brackets that aren't a whole table cell, or live inside a fenced code
// block, are ordinary text — and mustn't consume an index either, or the
// renderer and SetTableCheckbox would disagree about which box is which.
func TestRenderTableCheckboxesIgnoresNonCells(t *testing.T) {
	src := "not a table [ ] here\n\n```\n| [ ] | in a fence |\n```\n\n| a | b |\n|---|---|\n| [ ] partly | [x] |\n"
	html := Render([]byte(src), resolveNone)

	if strings.Count(html, "md-table-checkbox") != 1 {
		t.Fatalf("expected exactly one checkbox, got:\n%s", html)
	}
	if !strings.Contains(html, `data-checkbox-index="0" disabled checked>`) {
		t.Errorf("expected the lone whole-cell [x] to be index 0:\n%s", html)
	}
}

// EPUBs are static XHTML read on a tablet, so a (non-self-closed, and
// pointless) form control is swapped for a ballot-box character.
func TestRenderXHTMLTableCheckboxesAsSymbols(t *testing.T) {
	html := RenderXHTML([]byte(checklistTable), resolveNone)
	if strings.Contains(html, "<input") {
		t.Errorf("XHTML must not contain <input>:\n%s", html)
	}
	if strings.Count(html, "☐") != 1 || strings.Count(html, "☑") != 2 {
		t.Errorf("expected 1 ☐ and 2 ☑, got:\n%s", html)
	}
}

func TestSetTableCheckbox(t *testing.T) {
	got, err := SetTableCheckbox(checklistTable, 0, true)
	if err != nil {
		t.Fatalf("SetTableCheckbox() error = %v", err)
	}
	want := strings.Replace(checklistTable, "| [ ] | A Side walls |", "| [x] | A Side walls |", 1)
	if got != want {
		t.Errorf("checking index 0:\ngot:\n%s\nwant:\n%s", got, want)
	}

	got, err = SetTableCheckbox(checklistTable, 2, false)
	if err != nil {
		t.Fatalf("SetTableCheckbox() error = %v", err)
	}
	// Surrounding cell padding is preserved byte-for-byte.
	want = strings.Replace(checklistTable, "|  [X]  |", "|  [ ]  |", 1)
	if got != want {
		t.Errorf("unchecking index 2:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetTableCheckbox_OutOfRange(t *testing.T) {
	if _, err := SetTableCheckbox(checklistTable, 3, true); err == nil {
		t.Error("expected an error for index 3 of 3")
	}
	if _, err := SetTableCheckbox(checklistTable, -1, true); err == nil {
		t.Error("expected an error for a negative index")
	}
}

// Frontmatter is stripped before rendering, so it must not count toward
// indices when toggling either.
func TestSetTableCheckbox_SkipsFrontmatterAndFences(t *testing.T) {
	src := "---\ntitle: x\n---\n```\n| [ ] |\n```\n| a |\n|---|\n| [ ] |\n"
	got, err := SetTableCheckbox(src, 0, true)
	if err != nil {
		t.Fatalf("SetTableCheckbox() error = %v", err)
	}
	want := "---\ntitle: x\n---\n```\n| [ ] |\n```\n| a |\n|---|\n| [x] |\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Round trip: whatever index the renderer stamps on a box is the one
// SetTableCheckbox flips.
func TestSetTableCheckbox_AgreesWithRenderIndices(t *testing.T) {
	src := "# T\n\n```\n| [ ] |\n```\n\n| ✓ | p |\n|---|---|\n| [ ] | one |\n| [ ] | two |\n| [ ] | three |\n"
	for i := 0; i < 3; i++ {
		toggled, err := SetTableCheckbox(src, i, true)
		if err != nil {
			t.Fatalf("SetTableCheckbox(%d) error = %v", i, err)
		}
		html := Render([]byte(toggled), resolveNone)
		if strings.Count(html, " checked>") != 1 {
			t.Fatalf("index %d: expected exactly one checked box:\n%s", i, html)
		}
		want := `data-checkbox-index="` + string(rune('0'+i)) + `" disabled checked>`
		if !strings.Contains(html, want) {
			t.Errorf("index %d: expected %s in:\n%s", i, want, html)
		}
	}
}
