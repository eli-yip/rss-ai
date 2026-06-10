package telegram

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTMLToText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain passthrough", "just text", "just text"},
		{"strip anchor keeps text", `see <a href="https://t.me/x">the link</a> now`, "see the link now"},
		{"decode entities", "Tom &amp; Jerry &lt;3", "Tom & Jerry <3"},
		{"br becomes newline", "line one<br>line two", "line one\nline two"},
		{"paragraphs become newlines", "<p>first</p><p>second</p>", "first\nsecond"},
		{"collapse blank lines and spaces", "a<br><br><br>b   c", "a\nb c"},
		{"trim per line", "  <br>   padded   <br>  ", "padded"},
		{"empty input", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, htmlToText(tc.in))
		})
	}
}

// A realistic RSSHub telegram <description> fragment: forwarding line, a link,
// hard breaks, and trailing media caption. It should render as readable plain
// text with no tags or HTML entities left behind.
func TestHTMLToTextRealisticMessage(t *testing.T) {
	const desc = `Forwarded from <b>Some Channel</b><br><br>` +
		`Big news today: the &amp; project shipped.<br>` +
		`Read more at <a href="https://example.com">example.com</a><br><br>` +
		`<i>photo caption</i>`

	got := htmlToText(desc)

	require.NotContains(t, got, "<")
	require.NotContains(t, got, "&amp;")
	require.Contains(t, got, "Forwarded from Some Channel")
	require.Contains(t, got, "the & project shipped")
	require.Contains(t, got, "example.com")
	// No blank-line runs survive; lines are single-newline separated.
	require.NotContains(t, got, "\n\n")
	require.False(t, strings.HasPrefix(got, "\n"))
}
