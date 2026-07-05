// Package note builds vault paths and titles for the notes randoread serves.
package note

import (
	"fmt"
	"strings"
	"time"
)

// DailyFilename returns the vault's daily-note filename for t, matching the
// existing periodic/daily/YYYY-MM-DD-[W]WW-ddd.md naming convention.
func DailyFilename(t time.Time) string {
	_, week := t.ISOWeek()
	return fmt.Sprintf("%s-W%02d-%s.md", t.Format("2006-01-02"), week, t.Format("Mon"))
}

// FormatVaultTitle turns a full Dropbox path into the vault-relative,
// slash-separated title shown for Rando/Clipped notes, e.g.
// "/DropsyncFiles/jw-mind/books/2026/main.md" -> "books / 2026 / main".
func FormatVaultTitle(path, vaultRoot string) string {
	rel := strings.TrimPrefix(path, vaultRoot)
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimSuffix(rel, ".md")
	return strings.Join(strings.Split(rel, "/"), " / ")
}
