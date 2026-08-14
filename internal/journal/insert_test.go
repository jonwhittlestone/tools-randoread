package journal

import "testing"

func TestInsertUnderHeading_LastSectionEOF(t *testing.T) {
	// "## 📌 etc." is the last section in templates/Daily.md — no heading
	// or thematic break follows it, only end of file.
	raw := "## 📌 etc.\n> photos, handwritten notes, misc\n\n- \n\n\n"

	got, err := InsertUnderHeading(raw, "## 📌 etc.", "- `11:12`: noted")
	if err != nil {
		t.Fatalf("InsertUnderHeading() error = %v", err)
	}

	want := "## 📌 etc.\n> photos, handwritten notes, misc\n\n- \n- `11:12`: noted\n\n\n"
	if got != want {
		t.Errorf("InsertUnderHeading() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertUnderHeading_BoundedByThematicBreak(t *testing.T) {
	raw := "## 🥇Small Victories\n\n...\n\n---\n## 🗨 Log\n"

	got, err := InsertUnderHeading(raw, "## 🥇Small Victories", "- Shipped phase 2")
	if err != nil {
		t.Fatalf("InsertUnderHeading() error = %v", err)
	}

	want := "## 🥇Small Victories\n\n...\n- Shipped phase 2\n\n---\n## 🗨 Log\n"
	if got != want {
		t.Errorf("InsertUnderHeading() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertUnderHeading_MotivesSubsectionBoundedByNextSubsection(t *testing.T) {
	// ### headings are bounded by the next ### (same level) — must not
	// bleed into Evaluate.
	raw := "### Vent\n...\n\n### Evaluate\n...\n"

	got, err := InsertUnderHeading(raw, "### Vent", "- Frustrated with the deploy pipeline")
	if err != nil {
		t.Fatalf("InsertUnderHeading() error = %v", err)
	}

	want := "### Vent\n...\n- Frustrated with the deploy pipeline\n\n### Evaluate\n...\n"
	if got != want {
		t.Errorf("InsertUnderHeading() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertUnderHeading_StopsAtShallowerHeadingNotDeeperOne(t *testing.T) {
	// A ## section containing ### subsections: inserting under the ##
	// heading itself must not stop at the first ### it meets (deeper, not
	// same-or-shallower) — only a same-or-shallower heading or thematic
	// break ends the section.
	raw := "## 🗨 Log\n\n### Mindset\n...\n\n### Obligations\n...\n\n---\n## ❎ Todo.\n"

	got, err := InsertUnderHeading(raw, "## 🗨 Log", "- misfiled note")
	if err != nil {
		t.Fatalf("InsertUnderHeading() error = %v", err)
	}

	want := "## 🗨 Log\n\n### Mindset\n...\n\n### Obligations\n...\n- misfiled note\n\n---\n## ❎ Todo.\n"
	if got != want {
		t.Errorf("InsertUnderHeading() =\n%q\nwant\n%q", got, want)
	}
}

func TestInsertUnderHeading_HeadingNotFound(t *testing.T) {
	raw := "## 🥇Small Victories\n\n...\n"

	_, err := InsertUnderHeading(raw, "## 📌 etc.", "- x")
	if err == nil {
		t.Fatal("InsertUnderHeading() error = nil, want an error for a missing heading")
	}
}

func TestInsertUnderHeading_ExactStringMatchRequired(t *testing.T) {
	// A heading differing only in whitespace/case must not match — NanoClaw
	// is instructed to copy headings verbatim specifically so this stays a
	// strict match, not a fuzzy one.
	raw := "##  📌 etc.\n\n...\n" // note: two spaces after ##

	_, err := InsertUnderHeading(raw, "## 📌 etc.", "- x")
	if err == nil {
		t.Fatal("InsertUnderHeading() error = nil, want an error for a near-match heading")
	}
}
