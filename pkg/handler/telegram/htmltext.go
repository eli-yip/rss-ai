package telegram

import (
	"strings"

	"golang.org/x/net/html"
)

// blockTags are elements whose boundary should become a line break, so a
// Telegram message's structure survives as newlines once tags are stripped.
var blockTags = map[string]bool{
	"br": true, "p": true, "div": true, "li": true, "tr": true,
	"blockquote": true,
	"h1":         true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// htmlToText flattens a Telegram <description> HTML fragment to plain text: tags
// dropped, entities decoded, block boundaries turned into newlines, then every
// line trimmed and its internal whitespace collapsed, with blank lines removed.
// Malformed HTML degrades gracefully — whatever text was tokenized is returned.
func htmlToText(s string) string {
	if s == "" {
		return ""
	}

	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(s))
	for {
		switch z.Next() {
		case html.ErrorToken:
			return normalizeLines(b.String())
		case html.TextToken:
			b.Write(z.Text()) // Text() returns entity-decoded bytes, copied here
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			if name, _ := z.TagName(); blockTags[string(name)] {
				b.WriteByte('\n')
			}
		}
	}
}

// normalizeLines collapses each line's internal whitespace, trims it, drops
// empty lines, and joins with single newlines.
func normalizeLines(s string) string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, ln := range lines {
		if ln = strings.Join(strings.Fields(ln), " "); ln != "" {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}
