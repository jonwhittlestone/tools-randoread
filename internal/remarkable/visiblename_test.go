package remarkable

import "testing"

func TestVisibleNameFromFilename(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/home/root/Golden Son.epub", "Golden Son"},
		{"book.epub", "book"},
		{"/a/b/My Book (2020) - libgen.li.epub", "My Book (2020) - libgen.li"},
	}
	for _, c := range cases {
		if got := VisibleNameFromFilename(c.in); got != c.want {
			t.Errorf("VisibleNameFromFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
