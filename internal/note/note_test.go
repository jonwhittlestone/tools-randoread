package note

import (
	"testing"
	"time"
)

func TestDailyFilename(t *testing.T) {
	cases := []struct {
		date time.Time
		want string
	}{
		// Real filenames already in the vault (periodic/daily/), used to
		// pin the exact format: zero-padded ISO week, 3-letter weekday.
		{time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), "2026-07-05-W27-Sun.md"},
		{time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), "2022-01-01-W52-Sat.md"},
		{time.Date(2022, 1, 3, 0, 0, 0, 0, time.UTC), "2022-01-03-W01-Mon.md"},
	}

	for _, c := range cases {
		if got := DailyFilename(c.date); got != c.want {
			t.Errorf("DailyFilename(%s) = %q, want %q", c.date.Format("2006-01-02"), got, c.want)
		}
	}
}

func TestFormatRandoTitle(t *testing.T) {
	cases := []struct {
		path      string
		vaultRoot string
		want      string
	}{
		{
			"/DropsyncFiles/jw-mind/books/2026/happier-child-with-pda/main.md",
			"/DropsyncFiles/jw-mind",
			"books / 2026 / happier-child-with-pda / main",
		},
		{
			"/DropsyncFiles/jw-mind/projects/25-handyman.md",
			"/DropsyncFiles/jw-mind",
			"projects / 25-handyman",
		},
	}

	for _, c := range cases {
		if got := FormatRandoTitle(c.path, c.vaultRoot); got != c.want {
			t.Errorf("FormatRandoTitle(%q, %q) = %q, want %q", c.path, c.vaultRoot, got, c.want)
		}
	}
}
