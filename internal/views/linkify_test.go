package views

import (
	"context"
	"strings"
	"testing"
)

// renderLinkify is a tiny test-only helper that renders the component to a
// string so the test cases can substring-match against the output. Mirrors
// the way callers will use it (writing into the templ buffer); we just
// capture into a strings.Builder instead.
func renderLinkify(t *testing.T, in string) string {
	t.Helper()
	var sb strings.Builder
	if err := Linkify(in).Render(context.Background(), &sb); err != nil {
		t.Fatalf("Linkify(%q).Render: %v", in, err)
	}
	return sb.String()
}

func TestLinkify_PlainTextEscapesAndRoundTrips(t *testing.T) {
	got := renderLinkify(t, "no urls here")
	if got != "no urls here" {
		t.Errorf("got %q want %q", got, "no urls here")
	}
}

func TestLinkify_BareURLBecomesOneAnchor(t *testing.T) {
	got := renderLinkify(t, "https://example.com")
	want := `<a href="https://example.com" target="_blank" rel="noopener noreferrer" class="text-blue-700 hover:underline dark:text-blue-400">https://example.com</a>`
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestLinkify_HTTPSchemeAlsoLinks(t *testing.T) {
	got := renderLinkify(t, "see http://example.com today")
	if !strings.Contains(got, `href="http://example.com"`) {
		t.Errorf("expected http URL to be linked: %q", got)
	}
	if !strings.Contains(got, "see ") || !strings.Contains(got, " today") {
		t.Errorf("surrounding text lost: %q", got)
	}
}

func TestLinkify_TrailingPeriodStaysOutside(t *testing.T) {
	got := renderLinkify(t, "see http://example.com.")
	if !strings.Contains(got, `href="http://example.com"`) {
		t.Errorf("href should not include trailing period: %q", got)
	}
	if !strings.HasSuffix(got, "</a>.") {
		t.Errorf("period should follow closing anchor: %q", got)
	}
}

func TestLinkify_TrailingPunctVariants(t *testing.T) {
	// Each input has the URL followed by a single trailing punct character;
	// the href must end at "example.com" and the punct must follow </a>.
	cases := []struct{ in, suffix string }{
		{"see https://example.com.", "."},
		{"see https://example.com,", ","},
		{"see https://example.com)", ")"},
		{"see https://example.com]", "]"},
		{"see https://example.com}", "}"},
		{"see https://example.com;", ";"},
		{"see https://example.com!", "!"},
		{"see https://example.com?", "?"},
	}
	for _, c := range cases {
		t.Run(c.suffix, func(t *testing.T) {
			got := renderLinkify(t, c.in)
			if !strings.Contains(got, `href="https://example.com"`) {
				t.Errorf("href should not include %q: %q", c.suffix, got)
			}
			if !strings.HasSuffix(got, "</a>"+c.suffix) {
				t.Errorf("trailing %q should follow </a>: %q", c.suffix, got)
			}
		})
	}
}

func TestLinkify_TrailingPunctParenWrapped(t *testing.T) {
	// Parenthesised reference: "(see https://example.com)" - the trailing
	// ")" should stay outside the anchor.
	got := renderLinkify(t, "(see https://example.com)")
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("href trimmed wrong: %q", got)
	}
	if !strings.HasSuffix(got, "</a>)") {
		t.Errorf("closing paren should be outside anchor: %q", got)
	}
}

func TestLinkify_HTMLMetacharsInSurroundingTextAreEscaped(t *testing.T) {
	got := renderLinkify(t, "<script>alert(1)</script> https://example.com <img>")
	// The script tag must be rendered as escaped literal, never as raw HTML.
	if strings.Contains(got, "<script>") || strings.Contains(got, "<img>") {
		t.Errorf("HTML metachars must be escaped: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected &lt;script&gt; escape: %q", got)
	}
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("URL should still be linked: %q", got)
	}
}

func TestLinkify_HTMLMetacharsAlone(t *testing.T) {
	// No URL in the input, just markup-shaped text - everything must come
	// out escaped, and no anchor tag must appear.
	got := renderLinkify(t, `<script>alert("xss")</script>`)
	if strings.Contains(got, "<a ") {
		t.Errorf("no URL means no anchor: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") || !strings.Contains(got, "&lt;/script&gt;") {
		t.Errorf("expected fully escaped output: %q", got)
	}
}

func TestLinkify_MultipleURLs(t *testing.T) {
	got := renderLinkify(t, "first https://a.example second https://b.example end")
	if strings.Count(got, "<a ") != 2 {
		t.Errorf("expected 2 anchors: %q", got)
	}
	if !strings.Contains(got, `href="https://a.example"`) || !strings.Contains(got, `href="https://b.example"`) {
		t.Errorf("both URLs must be linked: %q", got)
	}
	if !strings.Contains(got, "first ") || !strings.Contains(got, " second ") || !strings.Contains(got, " end") {
		t.Errorf("interleaved text must survive: %q", got)
	}
}

func TestLinkify_AnchorAttributesAndClass(t *testing.T) {
	got := renderLinkify(t, "https://example.com")
	for _, want := range []string{
		`target="_blank"`,
		`rel="noopener noreferrer"`,
		`class="text-blue-700 hover:underline dark:text-blue-400"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in output: %q", want, got)
		}
	}
}

func TestLinkify_QueryStringAndFragmentSurvive(t *testing.T) {
	in := "https://example.com/path?x=1&y=2#frag"
	got := renderLinkify(t, in)
	if !strings.Contains(got, `href="https://example.com/path?x=1&amp;y=2#frag"`) {
		t.Errorf("href should escape '&' as &amp; but keep query+fragment: %q", got)
	}
}

func TestLinkify_DoesNotMatchInsideQuotes(t *testing.T) {
	// The regex deliberately stops at quote characters so the match can
	// never extend across an HTML quote boundary; verify by feeding it a
	// quoted URL and checking nothing past the quote ends up in the href.
	got := renderLinkify(t, `prefix "https://example.com" suffix`)
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("URL between quotes should still match cleanly: %q", got)
	}
	// The surrounding double-quote characters must be HTML-escaped (&#34;).
	if !strings.Contains(got, "&#34;") {
		t.Errorf("surrounding quotes should be escaped: %q", got)
	}
}

func TestTrimTrailingPunct(t *testing.T) {
	cases := []struct {
		in, wantURL, wantTail string
	}{
		{"https://example.com", "https://example.com", ""},
		{"https://example.com.", "https://example.com", "."},
		{"https://example.com).", "https://example.com", ")."},
		{"https://example.com!?)", "https://example.com", "!?)"},
	}
	for _, c := range cases {
		gotURL, gotTail := trimTrailingPunct(c.in)
		if gotURL != c.wantURL || gotTail != c.wantTail {
			t.Errorf("trimTrailingPunct(%q)=(%q,%q) want (%q,%q)", c.in, gotURL, gotTail, c.wantURL, c.wantTail)
		}
	}
}
