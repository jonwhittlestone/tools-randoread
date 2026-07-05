// Package note builds vault paths and titles for the notes randoread serves.
package note

import (
	"fmt"
	"time"
)

// DailyFilename returns the vault's daily-note filename for t, matching the
// existing periodic/daily/YYYY-MM-DD-[W]WW-ddd.md naming convention.
func DailyFilename(t time.Time) string {
	_, week := t.ISOWeek()
	return fmt.Sprintf("%s-W%02d-%s.md", t.Format("2006-01-02"), week, t.Format("Mon"))
}
