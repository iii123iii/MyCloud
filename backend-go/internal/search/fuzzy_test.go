package search

import "testing"

func TestNormaliseNameLowercases(t *testing.T) {
	if got := NormaliseName("Report.PDF"); got != "report.pdf" {
		t.Errorf("got %q", got)
	}
}

func TestDamerauLevenshteinBasic(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"kitten", "sitting", 3},
		{"book", "back", 2},
		{"ab", "ba", 1}, // transposition counts as 1
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abdc", 1},
	}
	for _, c := range cases {
		if got := DamerauLevenshtein(c.a, c.b); got != c.want {
			t.Errorf("DamerauLevenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
