package tw

import (
	"strings"
	"testing"
)

func TestUDANamePattern(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"estimate", true},
		{"client", true},
		{"points", true},
		{"story_points", true},
		{"X", true},
		{"a1", true},
		{"FooBar99", true},
		// Rejected: leading digit, leading underscore, dots, dashes, spaces,
		// shell metas, taskwarrior parser tokens, empty.
		{"", false},
		{"1estimate", false},
		{"_underscore", false},
		{"with-dash", false},
		{"with.dot", false},
		{"with space", false},
		{"name;ls", false},
		{"+tag", false},
		{"due:tomorrow", false},
		{"rc.uda.foo", false},
		// Length cap (1 + 63 = 64).
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := UDANamePattern.MatchString(c.name)
			if got != c.ok {
				t.Errorf("name %q: got %v want %v", c.name, got, c.ok)
			}
		})
	}
}

func TestQuoteArg(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", `"plain"`},
		{`with "quote"`, `"with \"quote\""`},
		{`back\slash`, `"back\\slash"`},
		{`combo \ and "q"`, `"combo \\ and \"q\""`},
		{"", `""`},
	}
	for _, c := range cases {
		got := QuoteArg(c.in)
		if got != c.want {
			t.Errorf("in=%q\n got=%q\nwant=%q", c.in, got, c.want)
		}
	}
}
