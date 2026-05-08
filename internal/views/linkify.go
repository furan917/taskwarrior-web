package views

import (
	"context"
	"io"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

// urlPat matches http:// or https:// URLs in a conservative, "good enough"
// way for body text rendered out of user-supplied descriptions and
// annotations. The character class deliberately excludes whitespace and the
// HTML metacharacters < > " ' so the match can never run past a tag
// boundary or quote boundary if the surrounding text contains markup-like
// content. Trailing punctuation that's almost always sentence punctuation
// rather than part of the URL (".,)]};!?") is trimmed in Linkify, not here -
// excluding it from the regex outright would break URLs that legitimately
// contain those characters mid-path (e.g. paren-style Wikipedia URLs).
var urlPat = regexp.MustCompile(`https?://[^\s<>"']+`)

// linkifyTrailingPunct is the set of trailing characters stripped from a
// matched URL after the regex match. These almost always belong to the
// surrounding sentence, not the URL itself, so "see http://example.com." links
// "http://example.com" and leaves the period as text.
const linkifyTrailingPunct = ".,)]};!?"

// linkClass mirrors the project / tag / blocked-by anchor styling in
// row.templ so URLs in body text adopt the same visual language as our
// other in-row links. Keep this in sync with that convention.
const linkClass = "text-blue-700 hover:underline dark:text-blue-400"

// Linkify wraps bare http(s) URLs in a string with anchor tags while
// HTML-escaping every non-URL segment. The returned templ.Component is safe
// to drop in anywhere a description or annotation is rendered as body text -
// it never emits raw user input as HTML, so a description containing
// "<script>" still renders as the literal escaped text.
//
// Implementation notes:
//   - We walk the regex matches in order, writing the (escaped) gap before
//     each match, then the trimmed URL as an anchor (escaped both in the
//     href and as visible text), then the trimmed-off trailing punctuation
//     (also escaped) so it stays as plain text after the link.
//   - Anchors use target="_blank" rel="noopener noreferrer" so opening a
//     task-description link can never gain access to window.opener.
//   - Server-rendered by design - the link must work without JS, both for
//     accessibility and so the row renders correctly during the brief
//     pre-hydration window on a fresh page load.
func Linkify(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		matches := urlPat.FindAllStringIndex(s, -1)
		cursor := 0
		for _, m := range matches {
			start, end := m[0], m[1]
			// Plain text segment between the previous match (or start) and
			// this match.
			if start > cursor {
				if _, err := io.WriteString(w, templ.EscapeString(s[cursor:start])); err != nil {
					return err
				}
			}
			rawURL := s[start:end]
			url, trailing := trimTrailingPunct(rawURL)
			// Defensive: if punctuation-trimming somehow left an empty URL
			// (e.g. a string that's only "http://."), emit the original
			// match as escaped text rather than an empty anchor.
			if url == "" {
				if _, err := io.WriteString(w, templ.EscapeString(rawURL)); err != nil {
					return err
				}
				cursor = end
				continue
			}
			escaped := templ.EscapeString(url)
			if _, err := io.WriteString(w, `<a href="`+escaped+`" target="_blank" rel="noopener noreferrer" class="`+linkClass+`">`+escaped+`</a>`); err != nil {
				return err
			}
			if trailing != "" {
				if _, err := io.WriteString(w, templ.EscapeString(trailing)); err != nil {
					return err
				}
			}
			cursor = end
		}
		// Tail segment after the last match (or the full string if no
		// matches).
		if cursor < len(s) {
			if _, err := io.WriteString(w, templ.EscapeString(s[cursor:])); err != nil {
				return err
			}
		}
		return nil
	})
}

// trimTrailingPunct splits a regex-matched URL into the URL itself and the
// trailing punctuation that should stay outside the anchor. We strip from
// the right repeatedly so "http://example.com).," loses all three trailing
// characters in one pass.
func trimTrailingPunct(raw string) (url, trailing string) {
	cut := strings.TrimRight(raw, linkifyTrailingPunct)
	return cut, raw[len(cut):]
}
